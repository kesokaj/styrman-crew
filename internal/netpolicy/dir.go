package netpolicy

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
)

const (
	// StaleAfter: a status file older than this means the timer is not
	// running; nothing can be confirmed from it any more.
	StaleAfter = 3 * time.Minute
	// PendingGrace: how long a record/rule mismatch reads as "pending" before
	// it is reported as an error.
	PendingGrace = 90 * time.Second
	// StatusCacheTTL bounds status-file reads under the 5 s project poll.
	StatusCacheTTL = 2 * time.Second
	// statusMaxBytes: a status file larger than this is treated as absent —
	// root writes a few hundred bytes; anything bigger is not root's file.
	statusMaxBytes = 64 << 10
)

var (
	uuidRe    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	kickTagRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
)

// Dir is the server's view of the marker directory and the root run dir.
//
// mu serializes SetMarker with the WHOLE of Sync — the want() snapshot is
// taken inside the lock — so a PUT can never create a marker between Sync's
// snapshot and its delete pass and lose the toggle (cold-reader finding on
// the plan). Kick takes no lock: the spool is append-only from the server's
// side and root consumes it.
type Dir struct {
	markers    string // <StateDir>/netpolicy/lan
	spool      string // <FirewallRunDir>/spool; "" = feature disabled
	statusPath string // <FirewallRunDir>/status
	logger     *slog.Logger
	now        func() time.Time

	mu sync.Mutex

	cacheMu  sync.Mutex
	cached   *Status
	cachedAt time.Time

	kickMu       sync.Mutex
	lastKick     map[string]time.Time
	lastKickWarn time.Time
}

// New wires the paths from the config. An empty FirewallRunDir disables the
// feature: Status is always nil, Kick always false.
func New(cfg *config.Config, logger *slog.Logger) *Dir {
	d := &Dir{
		markers:  filepath.Join(cfg.NetPolicyDir(), "lan"),
		logger:   logger,
		now:      time.Now,
		lastKick: map[string]time.Time{},
	}
	if cfg.FirewallRunDir != "" {
		d.spool = filepath.Join(cfg.FirewallRunDir, "spool")
		d.statusPath = filepath.Join(cfg.FirewallRunDir, "status")
	}
	return d
}

// Enabled reports whether a firewall run dir is CONFIGURED at all. Note that
// STYRMAN_FIREWALL_DIR carries a production default, so in practice this is
// true on every deployment; it is the kill switch for an operator who sets
// the variable to empty deliberately, not a test for "is the host half
// present".
func (d *Dir) Enabled() bool { return d.statusPath != "" }

// Installed reports whether the host firewall half is actually PRESENT — the
// root-owned run dir exists. Three states get confused easily and mean
// different things:
//
//   - Enabled()  — a dir is configured. Always true in practice (see above).
//   - Installed() — the run dir exists, so root's half has been installed.
//   - Status() != nil — a status file is readable RIGHT NOW.
//
// The credential broker needs the middle one. "Installed but the status is
// unreadable" means a rule may be live and a key must be withheld (BUG-014);
// "not installed at all" means no rule can exist, and treating that as
// "cannot confirm" would strip every project's SSH key on a local dev run or
// a host whose firewall half was never installed. The release review of
// 2026-09-08 caught exactly that: the guard had been written against
// Enabled(), which can never be false.
func (d *Dir) Installed() bool {
	if d.statusPath == "" {
		return false
	}
	// The STATUS FILE, not its directory. deploy/docker-compose.yml bind-mounts
	// /run/styrman-firewall into the server, and Docker creates a missing bind
	// source as an empty directory — so "the dir exists" is true on every
	// compose host whether or not root's half was ever installed, and the
	// first version of this check therefore fixed nothing in production
	// (security review round 2, 2026-09-08).
	//
	// Only root can write in that dir, so only root can remove the file; and
	// /run is a tmpfs, so a reboot clears it at the same moment it clears the
	// iptables rules the file describes. Absent really does mean "root has
	// applied nothing".
	fi, err := os.Stat(d.statusPath)
	return err == nil && fi.Mode().IsRegular()
}

// checkMarkersDir creates the marker dir if absent and refuses to use one
// that is not a real directory — the server's own uid is the only writer
// there, but a symlink would send markers somewhere root does not read.
func (d *Dir) checkMarkersDir() error {
	fi, err := os.Lstat(d.markers)
	if errors.Is(err, fs.ErrNotExist) {
		// 0750: the names inside are the set of LAN-enabled projects — the
		// server (owner) writes, root reads, other local uids do not.
		if err := os.MkdirAll(d.markers, 0o750); err != nil {
			return fmt.Errorf("netpolicy: create marker dir: %w", err)
		}
		fi, err = os.Lstat(d.markers)
	}
	if err != nil {
		return fmt.Errorf("netpolicy: marker dir: %w", err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("netpolicy: marker dir %s is not a real directory", d.markers)
	}
	return nil
}

// SetMarker projects one project's intent: an empty file named by the uuid
// while on, no file while off. Both directions are idempotent.
func (d *Dir) SetMarker(uuid string, on bool) error {
	if !uuidRe.MatchString(uuid) {
		return fmt.Errorf("netpolicy: invalid project id")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkMarkersDir(); err != nil {
		return err
	}
	return d.setMarkerLocked(uuid, on)
}

func (d *Dir) setMarkerLocked(uuid string, on bool) error {
	path := filepath.Join(d.markers, uuid)
	if on {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				return nil
			}
			return fmt.Errorf("netpolicy: create marker: %w", err)
		}
		return f.Close()
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("netpolicy: remove marker: %w", err)
	}
	return nil
}

// Sync makes the marker dir mirror the records: want() (called INSIDE the
// lock — see Dir) maps project id → LanAccess for every record on disk.
// Missing markers are created, markers whose id is not wanted are removed
// (fail-closed: a record that cannot be read is a record that is not on),
// and entries that are not plain files named by a uuid are left alone with a
// warning. Both counts are returned for the boot log.
func (d *Dir) Sync(want func() (map[string]bool, error)) (added, removed int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkMarkersDir(); err != nil {
		return 0, 0, err
	}
	w, err := want()
	if err != nil {
		return 0, 0, fmt.Errorf("netpolicy: sync snapshot: %w", err)
	}
	for id, on := range w {
		if !on || !uuidRe.MatchString(id) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(d.markers, id)); errors.Is(err, fs.ErrNotExist) {
			if err := d.setMarkerLocked(id, true); err != nil {
				return added, removed, err
			}
			added++
		}
	}
	des, err := os.ReadDir(d.markers)
	if err != nil {
		return added, removed, fmt.Errorf("netpolicy: list markers: %w", err)
	}
	foreign := 0
	for _, de := range des {
		name := de.Name()
		if !uuidRe.MatchString(name) || !de.Type().IsRegular() {
			foreign++
			continue
		}
		if w[name] {
			continue
		}
		if err := os.Remove(filepath.Join(d.markers, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return added, removed, fmt.Errorf("netpolicy: remove stray marker: %w", err)
		}
		removed++
		d.logger.Warn("lan access marker removed: no enabled record for it (project deleted, or its store unreadable)", "project", name)
	}
	if foreign > 0 {
		d.logger.Warn("marker dir holds entries that are not uuid-named plain files; root ignores them too", "dir", d.markers, "count", foreign)
	}
	return added, removed, nil
}

// Kick drops a trigger file into root's spool so the reconciler runs now
// rather than on the next 60 s tick. Best effort: false means the change
// still applies on the timer (the caller says so). The tag is only for the
// operator reading the spool; root never interprets names.
func (d *Dir) Kick(tag string) bool {
	if d.spool == "" || !kickTagRe.MatchString(tag) {
		return false
	}
	d.kickMu.Lock()
	defer d.kickMu.Unlock()
	return d.kickLocked(tag)
}

// kickLocked must be called with d.kickMu held.
func (d *Dir) kickLocked(tag string) bool {
	now := d.now()
	name := fmt.Sprintf("%s.%d", tag, now.UnixNano())
	f, err := os.OpenFile(filepath.Join(d.spool, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if now.Sub(d.lastKickWarn) > time.Hour {
			d.logger.Warn("host firewall spool unwritable — changes apply on the 60 s timer", "path", d.spool, "error", err)
			d.lastKickWarn = now
		}
		return false
	}
	_ = f.Close()
	d.lastKick[tag] = now
	return true
}

// KickThrottled is Kick for the boot paths (a terminal reconnect must not
// cost a root reconcile): a second kick with the same tag inside window is
// absorbed and reported as sent.
func (d *Dir) KickThrottled(tag string, window time.Duration) bool {
	if d.spool == "" || !kickTagRe.MatchString(tag) {
		return false
	}
	// One lock across the check and the write: two callers racing past the
	// check would otherwise both kick (harmless, but pointless — review).
	d.kickMu.Lock()
	defer d.kickMu.Unlock()
	if last, ok := d.lastKick[tag]; ok && d.now().Sub(last) < window {
		return true
	}
	return d.kickLocked(tag)
}

// Status returns root's last word, cached for StatusCacheTTL; nil when the
// feature is disabled or the file is absent, unparseable, or oversized.
func (d *Dir) Status(now time.Time) *Status {
	if d.statusPath == "" {
		return nil
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if !d.cachedAt.IsZero() && now.Sub(d.cachedAt) < StatusCacheTTL && now.After(d.cachedAt) {
		return d.cached
	}
	d.cached = d.readStatus()
	d.cachedAt = now
	return d.cached
}

func (d *Dir) readStatus() *Status {
	f, err := os.Open(d.statusPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > statusMaxBytes {
		return nil
	}
	st, err := ParseStatus(f)
	if err != nil {
		d.logger.Warn("host firewall status unparseable", "path", d.statusPath, "error", err)
		return nil
	}
	return st
}
