package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
)

// Per-project resources (ADR-083): CPU, memory and Docker disk.
//
// A project's record holds what its owner asked for; 0 CPUs or 0 GiB means
// the whole host. The container is created at that size, so a change applies
// by rebuilding it — right away for a running project whose live size
// differs, at the next start for a stopped one (the orchestrator rebuilds a
// stopped container whose size disagrees with its record). Home and
// workspace are bind mounts and survive; the ADR-028 apt manifest replays
// installed packages; anything else in the container filesystem does not.

// Docker disk bounds, the same as the operator's default (config.Load).
const (
	minDiskGB = 5
	maxDiskGB = 500
)

type resourceLimits struct {
	MaxCPUs      int `json:"maxCpus"`
	MaxMemoryGiB int `json:"maxMemoryGiB"` // 0: the host is too small to set any
	MinDiskGB    int `json:"minDiskGB"`    // the current size: the disk only grows
	MaxDiskGB    int `json:"maxDiskGB"`
}

// resourcesView is GET /api/projects/{id}/resources's body.
type resourcesView struct {
	CPUs      int `json:"cpus"`      // 0 = unlimited
	MemoryGiB int `json:"memoryGiB"` // 0 = unlimited
	DiskGB    int `json:"diskGB"`    // the effective size, never 0
	// Pending: the project is RUNNING at another size than its record asks
	// for (every project still running from before ADR-083). A PUT of the
	// same values rebuilds it.
	Pending bool           `json:"pending"`
	Limits  resourceLimits `json:"limits"`
}

// resourcesReply is PUT's reply: the view as saved, and whether this request
// rebuilt the project.
type resourcesReply struct {
	resourcesView
	Rebuilt bool `json:"rebuilt"`
}

func recordResources(p project.Project) orchestrator.Resources {
	return orchestrator.Resources{CPUs: p.CPUs, MemoryGiB: p.MemoryGiB, DiskGB: p.DiskGB}
}

func limitsFor(h orchestrator.Host, diskGB int) resourceLimits {
	return resourceLimits{MaxCPUs: h.CPUs, MaxMemoryGiB: h.MaxMemoryGiB(),
		MinDiskGB: max(minDiskGB, diskGB), MaxDiskGB: maxDiskGB}
}

func (s *Server) projectResources(w http.ResponseWriter, r *http.Request) {
	_, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	h, err := s.size.Host(ctx)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	// No project lock: this is a read, and the dialog must not wait behind a
	// boot (plan D27). An inspect failure costs only the `pending` hint.
	var live *orchestrator.Live
	l, found, err := s.size.LiveSizing(ctx, p.ID)
	if err != nil {
		s.logger.Warn("resources: cannot inspect the project; reporting no pending change", "project", p.ID, "error", err)
	} else if found {
		live = &l
	}
	want := s.size.WantSizing(h, recordResources(p), live)
	writeJSON(w, http.StatusOK, resourcesView{
		CPUs: p.CPUs, MemoryGiB: p.MemoryGiB, DiskGB: want.DiskGB,
		Pending: live != nil && live.Running && live.Sizing != want,
		Limits:  limitsFor(h, want.DiskGB),
	})
}

// setProjectResources saves a project's size and applies it. Like
// setProjectNetwork, whether anything is rebuilt is decided against the LIVE
// container, under the project's lock (security review 2026-09-09 round 3:
// a record-based short-circuit answered 200 over a container still in the
// old state):
//
//	running at another size than requested → stop, persist, start (rebuilt)
//	running at the requested size           → persist only, no restart
//	stopped or absent                       → persist only; the next start
//	                                          rebuilds it at the new size
//
// The record is written while nothing is running at the old size, or when
// what runs already matches, so it never claims a size a live container
// does not have.
func (s *Server) setProjectResources(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CPUs      *int `json:"cpus"`
		MemoryGiB *int `json:"memoryGiB"`
		DiskGB    *int `json:"diskGB"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) && te.Field != "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("%s must be an integer", te.Field))
			return
		}
		writeErr(w, http.StatusBadRequest, fmt.Errorf("body must be {cpus, memoryGiB, diskGB} as integers: %w", err))
		return
	}
	for name, v := range map[string]*int{"cpus": body.CPUs, "memoryGiB": body.MemoryGiB, "diskGB": body.DiskGB} {
		if v == nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("%s (integer) required", name))
			return
		}
	}
	// 404 before anything else: another user's project must look exactly like
	// a project that does not exist.
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
		writeErr(w, http.StatusConflict, errors.New("project is being deleted"))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()
	h, err := s.size.Host(ctx)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	req := orchestrator.Resources{CPUs: *body.CPUs, MemoryGiB: *body.MemoryGiB, DiskGB: *body.DiskGB}
	if req.CPUs != 0 && (req.CPUs < 1 || req.CPUs > h.CPUs) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("cpus must be 0 (unlimited) or 1-%d", h.CPUs))
		return
	}
	if maxGiB := h.MaxMemoryGiB(); req.MemoryGiB != 0 && (req.MemoryGiB < 1 || req.MemoryGiB > maxGiB) {
		if maxGiB == 0 {
			writeErr(w, http.StatusBadRequest, errors.New("memoryGiB must be 0 (unlimited): this host is too small to set a memory size"))
			return
		}
		writeErr(w, http.StatusBadRequest, fmt.Errorf("memoryGiB must be 0 (unlimited) or 1-%d", maxGiB))
		return
	}
	persist := func() error { // netMu: same inner lock, same reason as network.go
		s.netMu.Lock()
		defer s.netMu.Unlock()
		return sp.projects.SetResources(p.ID, req.CPUs, req.MemoryGiB, req.DiskGB)
	}
	type reply struct {
		status int
		body   any
		err    error
	}
	fail := func(status int, err error) reply { return reply{status: status, err: err} }
	res := fail(http.StatusInternalServerError, errors.New("the size was NOT changed: no decision was made"))
	s.mode.WithProject(u.Username, p.ID, func(pr orchestrator.Project) {
		rec, err := sp.projects.Get(p.ID) // as of NOW, under the lock
		if err != nil {
			res = fail(http.StatusNotFound, err)
			return
		}
		var live *orchestrator.Live
		l, found, err := s.size.LiveSizing(ctx, p.ID)
		if err != nil {
			res = fail(http.StatusInternalServerError,
				fmt.Errorf("the project could not be inspected, so its size was NOT changed: %w", err))
			return
		}
		if found {
			live = &l
		}
		// The disk only grows: the floor is what the project has now.
		cur := s.size.WantSizing(h, recordResources(rec), live)
		if floor := max(minDiskGB, cur.DiskGB); req.DiskGB < floor || req.DiskGB > maxDiskGB {
			res = fail(http.StatusBadRequest, fmt.Errorf("diskGB must be %d-%d (the Docker disk can only grow)", floor, maxDiskGB))
			return
		}
		want := s.size.WantSizing(h, req, live)
		view := resourcesView{CPUs: req.CPUs, MemoryGiB: req.MemoryGiB, DiskGB: req.DiskGB,
			Limits: limitsFor(h, req.DiskGB)}
		logChange := func(rebuilt bool) {
			s.logger.Info("project resources changed", "by", u.Email, "project", p.ID, "projectName", p.Name,
				"from", recordResources(rec), "to", req, "rebuilt", rebuilt)
		}
		if live == nil || !live.Running || live.Sizing == want {
			if recordResources(rec) == req {
				res = reply{status: http.StatusOK, body: resourcesReply{resourcesView: view}}
				return
			}
			if err := persist(); err != nil {
				if errors.Is(err, project.ErrNotFound) {
					res = fail(http.StatusNotFound, err)
					return
				}
				res = fail(http.StatusInternalServerError, fmt.Errorf("the size could not be saved, so it was NOT changed: %w", err))
				return
			}
			logChange(false)
			res = reply{status: http.StatusOK, body: resourcesReply{resourcesView: view}}
			return
		}
		if err := pr.Stop(ctx); err != nil {
			res = fail(http.StatusInternalServerError,
				fmt.Errorf("the project could not be stopped, so it is still running at its old size: %w", err))
			return
		}
		if err := persist(); err != nil {
			if errors.Is(err, project.ErrNotFound) {
				res = fail(http.StatusNotFound, err)
				return
			}
			res = fail(http.StatusInternalServerError,
				fmt.Errorf("the project was stopped but the size could not be saved — start it to continue at its old size: %w", err))
			return
		}
		if _, err := pr.EnsureRunning(ctx, runOptsFor(sp, rec, sp.profile.GuestUser())); err != nil {
			res = fail(http.StatusInternalServerError,
				fmt.Errorf("the new size was saved, but the project could not be restarted: %w — it applies on the next start", err))
			return
		}
		logChange(true)
		go func() { // background pre-attach, as network.go does
			time.Sleep(time.Second)
			s.orch.EnsureSelfAttached(u.Username, p.ID)
		}()
		res = reply{status: http.StatusOK, body: resourcesReply{resourcesView: view, Rebuilt: true}}
	})
	if res.err != nil {
		writeErr(w, res.status, res.err)
		return
	}
	writeJSON(w, res.status, res.body)
}
