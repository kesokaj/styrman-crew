// Package httpapi exposes styrman's REST + WebSocket API and serves the
// embedded UI. JSON follows the camelCase conventions from the json-api-style
// standard; errors are {"error":{"message":...}}. Every request carries an
// identity (Google email via the ingress, or the configured default on the
// loopback dev listener); projects, terminals, files, and history are all
// scoped to that user's own home and stores (multi-tenancy, ADR-019).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kesokaj/styrman-crew/internal/config"
	"github.com/kesokaj/styrman-crew/internal/conversation"
	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/files"
	"github.com/kesokaj/styrman-crew/internal/gitrepo"
	"github.com/kesokaj/styrman-crew/internal/identity"
	"github.com/kesokaj/styrman-crew/internal/links"
	"github.com/kesokaj/styrman-crew/internal/membudget"
	"github.com/kesokaj/styrman-crew/internal/orchestrator"
	"github.com/kesokaj/styrman-crew/internal/project"
	"github.com/kesokaj/styrman-crew/internal/terminal"
	"github.com/kesokaj/styrman-crew/internal/userstore"
	"github.com/kesokaj/styrman-crew/web"
)

// Version is stamped via -ldflags at build time.
var Version = "dev"

// userSpace holds one user's metadata stores and profile.
type userSpace struct {
	projects *project.Store
	terms    *conversation.Store
	profile  *profileStore
}

// Server wires the API handlers to their dependencies.
// guestRuntime is the slice of the orchestrator this package needs to run a
// command inside a project's microVM: where the container is (Status) and how
// to run something in it (Exec).
//
// It exists to make the HTTP handlers TESTABLE. Every git verb, the syntax
// checker and the formatter reach the guest through exactly these two calls,
// and with the concrete *orchestrator.Orchestrator there was no way to
// exercise a handler without a live Docker daemon and a booted Kata microVM —
// so their argv, status mapping and error handling had no unit coverage at
// all (recorded three times over: TODO.md's checkFile/formatFile item, the
// v1.22.0 gate's note on the six new git handlers, and BUG-003's residual).
//
// Deliberately NARROW: the orchestrator's other fifteen-odd methods stay on
// the concrete `orch` field. This is a seam for the exec path, not an
// abstraction over the orchestrator.
//
// CONTRACT, which the concrete orchestrator enforced implicitly and an
// interface does not: the ContainerID returned by Status BELONGS to that
// user and project, and a caller may Exec into it on that basis alone. It is
// the only thing binding a subsequent Exec to the right tenant's microVM, so
// any implementation that returns an id not derived from the pair breaks
// tenant isolation, not merely a test (security review 2026-09-04).
type guestRuntime interface {
	Status(ctx context.Context, user, project string) (orchestrator.Status, error)
	Exec(ctx context.Context, containerID string, spec orchestrator.ExecSpec) (int, string, error)
	GuestGoingAway(ctx context.Context, user, project string, ran orchestrator.Status) bool
}

// modeRuntime is the network-mode handler's view of the orchestrator: one
// project under its lock. Same reason guestRuntime exists — the interesting
// behaviour here is what happens when one of the locked calls FAILS (the
// record must not keep claiming a mode that is not in force), and that is
// unreachable through the concrete orchestrator, which needs a real Docker.
type modeRuntime interface {
	WithProject(user, project string, fn func(orchestrator.Project))
}

// sizeRuntime is the Resources handler's read side of the orchestrator
// (ADR-083): the host's size, a container's current size (without the
// project lock — the dialog must not wait behind a boot), and the sizing a
// request works out to. An interface for the same reason as modeRuntime.
type sizeRuntime interface {
	Host(ctx context.Context) (orchestrator.Host, error)
	LiveSizing(ctx context.Context, project string) (orchestrator.Live, bool, error)
	WantSizing(h orchestrator.Host, r orchestrator.Resources, live *orchestrator.Live) orchestrator.Sizing
}

// runOptsFor is the ONE place a project's RunOpts is built, so every start
// path carries both reads the orchestrator requires under the project's lock:
// the network mode and the size. RunOpts.Resources is required (a nil one
// refuses the start), so a start path that built its own would fail on its
// first start rather than run a limited project at the whole host (ADR-083,
// plan D18; rule mechanism-reaches-every-caller). user is the project's
// owner: a non-admin's project starts at most at the admin cap (ADR-087).
func (s *Server) runOptsFor(sp *userSpace, user string, p project.Project, guest string) orchestrator.RunOpts {
	return orchestrator.RunOpts{Name: p.Name, Dir: p.Dir, GuestUser: guest,
		Lan: lanOf(sp, p.ID), Resources: s.resourcesOf(sp, user, p.ID)}
}

// resourcesOf is the RunOpts.Resources read: the record's size as of the
// moment the orchestrator sizes the container, like lanOf, with the non-admin
// cap applied. The record is never rewritten, so raising the cap restores
// what the owner asked for. A host read that fails fails the start, as a
// record read does — never a silent uncapped start.
func (s *Server) resourcesOf(sp *userSpace, user, id string) func() (orchestrator.Resources, error) {
	return func() (orchestrator.Resources, error) {
		p, err := sp.projects.Get(id)
		if err != nil {
			return orchestrator.Resources{}, err
		}
		r := recordResources(p)
		if s.ownerIsAdmin(user) {
			return r, nil
		}
		// RunOpts.Resources takes no ctx (orchestrator.go); Host is cached
		// after its first success, so this is a map read on every start but
		// the first.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h, err := s.size.Host(ctx)
		if err != nil {
			return orchestrator.Resources{}, fmt.Errorf("read the host size for the project cap: %w", err)
		}
		return s.startCapped(h, false, r), nil
	}
}

// lanOf is the RunOpts.Lan read for a project: the record's mode as of the
// moment the orchestrator decides the bridge name, under the project's
// lock, never a snapshot taken when the request began.
func lanOf(sp *userSpace, id string) func() (bool, error) {
	return func() (bool, error) {
		p, err := sp.projects.Get(id)
		if err != nil {
			return false, err
		}
		return p.LanAccess, nil
	}
}

type Server struct {
	cfg *config.Config
	// orch is the real orchestrator; guest is the same object behind the
	// narrow interface above, swapped for a fake in tests. Both point at the
	// same instance in production — see New.
	orch  *orchestrator.Orchestrator
	guest guestRuntime
	// browserUpstreamFn, when set, replaces browserUpstream's orchestrator
	// lookup — tests point the CDP relay at an httptest server.
	browserUpstreamFn func(ctx context.Context, user, project string) (host string, fresh bool, err error)
	// browserPeerFn likewise replaces the orchestrator's BrowserPeer in tests.
	browserPeerFn func(ctx context.Context, project string) string
	// browserRelays counts live CDP relays (capped globally and per user).
	browserRelays relayCounter
	// stillAuthorized is the same re-check the terminal bridge runs (nil in
	// AuthMode=none, "" exempt — see New); an open browser relay asks it
	// every browserReauthEvery and closes 4003 when the owner is gone.
	stillAuthorized    func(email string) bool
	browserReauthEvery time.Duration
	mode               modeRuntime
	size               sizeRuntime
	tmux               *conversation.Manager
	bridge             *terminal.Bridge
	users              *userstore.Store
	logger             *slog.Logger
	links              *links.Store

	spacesMu sync.Mutex
	spaces   map[string]*userSpace

	// proxyMu/proxies cache port-preview reverse proxies (see proxy.go).
	proxyMu sync.Mutex
	proxies map[string]*httputil.ReverseProxy

	// sharesMu serializes setShares' store-write + link-mint/delete pair so
	// concurrent PUTs can't interleave into "shares without a link" (or the
	// reverse). Consistency only — authz re-checks the list on every request.
	sharesMu sync.Mutex

	// netMu serializes setProjectNetwork's record write against
	// deleteProject's record removal, so a mode change and a deletion of the
	// same project cannot interleave (ADR-067; the marker/spool this used to
	// also guard are gone). Never held across a rebuild — see network.go.
	netMu sync.Mutex

	// searchSlots bounds history searches server-wide (searchConcurrency);
	// see searchHistory.
	searchSlots *slotPool
	// readSlots bounds transcript reads server-wide (readConcurrency); see
	// readHistory.
	readSlots *slotPool
	// fileSlots bounds the editor's file endpoints (fileConcurrency).
	fileSlots *slotPool
	// budget is the server-wide memory budget every API request is charged
	// against (budgetGuard, ADR-086); a test may replace it with a small one.
	budget *membudget.Budget

	// creds is the credential broker's store (ADR-030); nil disables the
	// onboarding endpoints. Set by main after construction.
	creds *credstore.Store

	// deletingMu guards these maps (key user+"/"+projectID): deleting tracks
	// in-flight background destroys so the UI can show a "deleting" state, a
	// repeated DELETE is idempotent, and boot paths refuse to resurrect the
	// container mid-destroy; deleteErrs keeps the last failed destroy's
	// message so a 202'd delete that later fails is not silently invisible;
	// provisionErrs does the same for the background repo provisioning that
	// runs after the 201 (clone / GitHub repo create) — without it a failed
	// clone presents as a project that silently ignored the request.
	deletingMu sync.Mutex
	deleting   map[string]bool
	deleteErrs map[string]string
	// provisioning holds the IDs whose background boot+repo provisioning is
	// still running (set right before the goroutine, deleted by its first
	// deferred statement), so GET/list can report "provisioning" for the
	// window between the 201 and the clone finishing — during which the
	// workspace is briefly not yet a repo (R13) and the container may still
	// be absent (TODO close-out 2026-09-10, item N).
	provisioning  sync.Map
	provisionErrs map[string]string

	// previewOwners memoizes projectID→owner for the preview proxy's
	// cross-owner resolution (hits are re-validated against the live record;
	// see previewAccess).
	previewOwners sync.Map
	// notifCache memoizes each user's notifications scan for a few seconds
	// (see listNotifications).
	notifCache sync.Map
	// credMu guards credPushing and credPushRerun (key user+"/-"):
	// credPushing single-flights the per-user credential push into running
	// projects, credPushRerun coalesces pushes requested while one is in
	// flight (see credharvest.go). They had shared deletingMu, which guards
	// the project-lifecycle maps above — two unrelated features on one lock,
	// so a credential push and a project delete serialised against each other
	// for no reason and a reader had to prove they were unrelated (simplify
	// lens, 2026-09-22).
	credMu        sync.Mutex
	credPushing   map[string]bool
	credPushRerun map[string]bool

	// provisionDelay and brokerProbeInterval pace the background repo
	// provisioning (see provisionRepo). Fields rather than package globals so
	// a test can drive the sequence without sleeping AND without mutating
	// state shared with every other test in the package.
	provisionDelay      time.Duration
	brokerProbeInterval time.Duration
}

// notifCacheEntry is one user's memoized notifications payload.
type notifCacheEntry struct {
	at    time.Time
	items []notification
}

// New creates the Server.
func New(cfg *config.Config, users *userstore.Store, orch *orchestrator.Orchestrator, logger *slog.Logger) *Server {
	// Tenant bases: every files.* root under them is opened confined to the
	// tenant, never by following a guest-plantable symlink on the host
	// (BUG-002). The workspaces base is included for uniformity; its
	// per-project mount point is not guest-replaceable, the homes' nested
	// `p/<uuid>` is.
	files.Trust(cfg.HomesDir, filepath.Join(cfg.StateDir, "projects"))
	s := &Server{
		cfg:           cfg,
		orch:          orch,
		guest:         orch,
		mode:          orch,
		size:          orch,
		tmux:          conversation.NewManager(orch, cfg),
		users:         users,
		logger:        logger,
		spaces:        map[string]*userSpace{},
		deleting:      map[string]bool{},
		searchSlots:   newSlotPool(searchConcurrency),
		readSlots:     newSlotPool(readConcurrency),
		fileSlots:     newSlotPool(fileConcurrency),
		budget:        membudget.New(budgetTotal, budgetPerTenant),
		deleteErrs:    map[string]string{},
		provisionErrs: map[string]string{},

		provisionDelay:      time.Second,
		brokerProbeInterval: 500 * time.Millisecond,
		credPushing:         map[string]bool{},
		credPushRerun:       map[string]bool{},
	}
	// Public preview-link store (best-effort: a load failure degrades to
	// no-links, never blocks server start).
	if ls, err := links.NewStore(filepath.Join(cfg.StateDir, "links.json")); err != nil {
		logger.Error("links store unavailable — public links disabled", "error", err)
	} else {
		s.links = ls
	}
	// Re-check terminal authorization only where identity comes from the
	// userstore: in AuthMode=none the minted identity is in no store.
	var stillAuthorized func(string) bool
	if cfg.AuthMode == "header" {
		// An identity without an email was minted, not looked up (the
		// loopback dev listener); the store cannot vouch for it either way,
		// so it is not re-checked (security review of the close-out's phase
		// 4, low: with an empty allowlist that listener's terminals would
		// otherwise close every minute).
		stillAuthorized = func(email string) bool { return email == "" || users.IsAuthorized(email) }
	}
	s.stillAuthorized = stillAuthorized
	s.browserReauthEvery = 60 * time.Second
	s.bridge = terminal.NewBridge(orch, cfg, logger, s.termStoreFor, s.ensureFor, s.guestUserFor, stillAuthorized)
	return s
}

// SetCredStore wires the credential broker's store so onboarding endpoints
// can write to it (ADR-030). Called by main after New.
//
// The broker's ADDRESS is delivered to each project as the STYRMAN_CRED_BROKER
// container env (a fixed per-bridge IP the server also binds on attach), so
// there is no per-project hint file — see orchestrator.brokerIPOnBridge.
func (s *Server) SetCredStore(cs *credstore.Store) { s.creds = cs }

// NetPolicy exposes the marker dir so main can run the boot-time and periodic
// Sync against every user's records (ADR-065).

// ensureFor lets the WS bridge boot a project it only knows by user+UUID.
func (s *Server) ensureFor(ctx context.Context, user, projectID string) (containerID, guestUser, dir string, err error) {
	// A terminal WS auto-reconnects when its container is force-removed —
	// without this guard it would resurrect the container mid-destroy and
	// leave an orphan microVM with no project record.
	if inFlight, _ := s.deletingState(user, projectID); inFlight {
		return "", "", "", fmt.Errorf("project is being deleted")
	}
	sp, err := s.space(user)
	if err != nil {
		return "", "", "", err
	}
	p, err := sp.projects.Get(projectID)
	if err != nil {
		return "", "", "", err
	}
	guest := sp.profile.GuestUser()
	id, err := wsBoot(ctx, s.orch, user, p.ID, s.runOptsFor(sp, user, p, guest))
	return id, guest, p.Dir, err
}

// wsBooter is the slice of the orchestrator the terminal-WS boot path uses;
// an interface so the path is testable without Docker (as the mode toggle's
// Project is).
type wsBooter interface {
	EnsureRunning(ctx context.Context, user, project string, opts orchestrator.RunOpts) (string, error)
	EnsureSelfAttached(user, project string) bool
}

// wsBoot boots the project for the WS bridge and joins this server to its
// bridge BEFORE the caller upgrades the socket. The login shell the terminal
// is about to spawn pulls its credentials from the broker at the bridge's
// fixed address (STYRMAN_CRED_BROKER), which answers only once this server
// holds that address. EnsureRunning does not attach on purpose (see its
// NOTE), and every other boot path attaches in a goroutine after its
// response — this path has no response to protect, so it never attached, and
// after every server restart the first terminal of a stopped project came up
// without credentials (BUG-060). The "fresh" return is ignored here: nothing
// is upgraded yet, and a client whose handshake a fresh attach disturbed
// simply reconnects into the memoized attach.
func wsBoot(ctx context.Context, o wsBooter, user, project string, opts orchestrator.RunOpts) (string, error) {
	id, err := o.EnsureRunning(ctx, user, project, opts)
	if err != nil {
		return "", err
	}
	o.EnsureSelfAttached(user, project)
	return id, nil
}

// guestUserFor resolves the in-guest username for a user.
func (s *Server) guestUserFor(user string) string {
	sp, err := s.space(user)
	if err != nil {
		return user
	}
	return sp.profile.GuestUser()
}

// userSlugRe matches a DeriveUsername slug: starts with a letter, then only
// [a-z0-9_-], at most 32 chars. Used to reject a malformed username before it
// becomes a filesystem path.
var userSlugRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// space returns (lazily creating) a user's stores and home skeleton.
func (s *Server) space(user string) (*userSpace, error) {
	s.spacesMu.Lock()
	defer s.spacesMu.Unlock()
	if sp, ok := s.spaces[user]; ok {
		return sp, nil
	}
	// Defense in depth: `user` is normally DeriveUsername's slug, but it also
	// arrives from persisted sources (e.g. Link.Owner) that aren't re-validated
	// on load — reject anything that isn't a clean slug before it's joined into
	// a state path, so a hand-edited record can't traverse into another user's
	// tree (low finding).
	if !userSlugRe.MatchString(user) {
		return nil, fmt.Errorf("invalid user slug %q", user)
	}
	// Homes are per-project now (ADR-031) and seeded by the image entrypoint,
	// so space() no longer creates a home — only the user's metadata dir.
	dir := s.cfg.UserStateDir(user)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	projects, err := project.NewStore(filepath.Join(dir, "projects.json"))
	if err != nil {
		return nil, err
	}
	terms, err := conversation.NewStore(filepath.Join(dir, "conversations.json"))
	if err != nil {
		return nil, err
	}
	prof, err := newProfileStore(filepath.Join(dir, "profile.json"), user)
	if err != nil {
		return nil, err
	}
	sp := &userSpace{projects: projects, terms: terms, profile: prof}
	s.spaces[user] = sp
	return sp, nil
}

// termStoreFor adapts the per-user terminal store for the WS bridge.
func (s *Server) termStoreFor(user string) (terminal.TermStore, error) {
	sp, err := s.space(user)
	if err != nil {
		return nil, err
	}
	return sp.terms, nil
}

// Routes returns the root handler with the configured auth mode.
func (s *Server) Routes() http.Handler { return s.RoutesWithAuth(s.cfg.AuthMode) }

// RoutesWithAuth builds the handler with an explicit auth mode — the dev
// listener (loopback-published, see config.DevAddr) uses "none".
func (s *Server) RoutesWithAuth(authMode string) http.Handler {
	mux := s.newMux()
	// previewHostRewrite outermost so a <token>.preview host is mapped to the
	// public /link route before auth even looks at it; then auth (identity /
	// permission), then CSRF, then the memory budget (ADR-086), then routing.
	return s.previewHostRewrite(s.authGuard(authMode, mux, csrfGuard(mux, s.budgetGuard(mux))))
}

// newMux registers every route; budgetGuard looks the matched pattern up in
// routeWeight, so the table test reads the same mux.
func (s *Server) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("GET /api/projects/{project}", s.getProject)
	mux.HandleFunc("DELETE /api/projects/{project}", s.deleteProject)
	mux.HandleFunc("POST /api/projects/{project}/start", s.startProject)
	mux.HandleFunc("POST /api/projects/{project}/stop", s.stopProject)
	mux.HandleFunc("GET /api/projects/{project}/stats", s.projectStats)
	mux.HandleFunc("GET /api/projects/{project}/logs", s.projectLogs)
	mux.HandleFunc("GET /api/git/summary", s.gitSummary)
	// Project-scoped env variables (ADR-043) — SidePanel Env tab.
	mux.HandleFunc("GET /api/projects/{project}/env", s.listProjectEnv)
	mux.HandleFunc("PUT /api/projects/{project}/env", s.putProjectEnv)
	mux.HandleFunc("GET /api/projects/{project}/env/{key}", s.projectEnvValue)
	mux.HandleFunc("DELETE /api/projects/{project}/env/{key}", s.deleteProjectEnv)
	// Git side panel — a real git client (runs in the guest VM).
	mux.HandleFunc("GET /api/projects/{project}/git/status", s.gitStatus)
	mux.HandleFunc("GET /api/projects/{project}/git/diff", s.gitDiff)
	mux.HandleFunc("POST /api/projects/{project}/git/stage", s.gitStage)
	mux.HandleFunc("POST /api/projects/{project}/git/unstage", s.gitUnstage)
	mux.HandleFunc("POST /api/projects/{project}/git/discard", s.gitDiscard)
	mux.HandleFunc("POST /api/projects/{project}/git/commit", s.gitCommit)
	mux.HandleFunc("POST /api/projects/{project}/git/push", s.gitPush)
	// v1.22.0 verbs. Reads are GET and answer 200 with a state/repo flag;
	// mutations are POST and answer 4xx, matching the split above.
	mux.HandleFunc("GET /api/projects/{project}/git/branches", s.gitBranches)
	mux.HandleFunc("GET /api/projects/{project}/git/log", s.gitLog)
	mux.HandleFunc("POST /api/projects/{project}/git/fetch", s.gitFetch)
	mux.HandleFunc("POST /api/projects/{project}/git/pull", s.gitPull)
	mux.HandleFunc("POST /api/projects/{project}/git/checkout", s.gitCheckout)
	mux.HandleFunc("POST /api/projects/{project}/git/stash", s.gitStash)
	// Format a file with its language's real CLI formatter, in the guest VM.
	mux.HandleFunc("POST /api/projects/{project}/format", s.formatFile)
	// Syntax check: same exec-in-guest shape as /format (check.go).
	mux.HandleFunc("POST /api/projects/{project}/check", s.checkFile)
	// Terminals: durable records, one tmux session each (ADR-016).
	mux.HandleFunc("GET /api/projects/{project}/terminals", s.listTerminals)
	mux.HandleFunc("POST /api/projects/{project}/terminals", s.createTerminal)
	mux.HandleFunc("DELETE /api/projects/{project}/terminals", s.deleteAllTerminals)
	mux.HandleFunc("PATCH /api/projects/{project}/terminals/{term}", s.updateTerminal)
	mux.HandleFunc("DELETE /api/projects/{project}/terminals/{term}", s.deleteTerminal)
	mux.HandleFunc("POST /api/projects/{project}/terminals/{term}/pause", s.pauseTerminal)
	mux.Handle("GET /ws/term/{project}/{term}", s.bridge)
	mux.HandleFunc("GET /ws/setup", s.bridge.ServeSetup)
	// Browser view (ADR-068): owner-only status/start/stop of the project's
	// in-guest Chromium, and the CDP relay the Browser pane speaks over.
	mux.HandleFunc("GET /api/projects/{project}/browser", s.getBrowser)
	mux.HandleFunc("POST /api/projects/{project}/browser/start", s.startBrowser)
	mux.HandleFunc("POST /api/projects/{project}/browser/stop", s.stopBrowser)
	mux.HandleFunc("GET /ws/browser/{project}", s.browserSocket)
	// Ports + preview proxy. /p/... are the shareable links; /proxy/... is
	// the terminal-independent form. Per-method registration: a method-less
	// pattern conflicts with the "GET /" SPA catch-all.
	mux.HandleFunc("GET /api/projects/{project}/ports", s.listPorts)
	// Port-preview sharing (owner grants specific Google emails access to ONE
	// exposed port).
	mux.HandleFunc("GET /api/projects/{project}/ports/{port}/shares", s.getShares)
	mux.HandleFunc("PUT /api/projects/{project}/ports/{port}/shares", s.setShares)
	// Public TTL preview links (owner mints/lists/revokes; the link itself is
	// public — see the /link/{token} route below).
	mux.HandleFunc("POST /api/projects/{project}/ports/{port}/links", s.createLink)
	mux.HandleFunc("GET /api/projects/{project}/links", s.listLinks)
	mux.HandleFunc("DELETE /api/projects/{project}/links/{token}", s.revokeLink)
	// Public capability route (authGuard-exempt; token-validated in serveLink).
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(m+" /link/{token}/", s.serveLink)
	}
	mux.HandleFunc("GET /link/{token}", redirectSlash)
	// Private share route: like /link but served on the OAuth-gated private
	// host — authGuard resolves the visitor's identity, serveShared checks it
	// against the port's shared-email list.
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(m+" /shared/{token}/", s.serveShared)
	}
	mux.HandleFunc("GET /shared/{token}", redirectSlash)
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		mux.HandleFunc(m+" /proxy/{project}/{port}/", s.proxyPort)
		mux.HandleFunc(m+" /p/{project}/{term}/port/{port}/", s.proxyPort)
	}
	mux.HandleFunc("GET /p/{project}/{term}/port/{port}", redirectSlash)
	mux.HandleFunc("GET /proxy/{project}/{port}", redirectSlash)
	// File explorer: the user's project workspace and their home.
	mux.HandleFunc("GET /api/projects/{project}/files", s.listFiles)
	mux.HandleFunc("GET /api/projects/{project}/file", s.readFile)
	mux.HandleFunc("PUT /api/projects/{project}/file", s.writeFile)
	mux.HandleFunc("DELETE /api/projects/{project}/file", s.deleteFile)
	mux.HandleFunc("POST /api/projects/{project}/file/rename", s.renameFile)
	mux.HandleFunc("POST /api/projects/{project}/mkdir", s.makeDir)
	mux.HandleFunc("POST /api/projects/{project}/upload", s.uploadFile)
	mux.HandleFunc("GET /api/projects/{project}/download", s.downloadFile)
	// The project's OWN home (ADR-031: homes are per-project). Pattern
	// carries "/home/" so scopeFor roots at ProjectHome.
	mux.HandleFunc("GET /api/projects/{project}/home/files", s.listFiles)
	mux.HandleFunc("GET /api/projects/{project}/home/file", s.readFile)
	mux.HandleFunc("PUT /api/projects/{project}/home/file", s.writeFile)
	mux.HandleFunc("DELETE /api/projects/{project}/home/file", s.deleteFile)
	mux.HandleFunc("POST /api/projects/{project}/home/file/rename", s.renameFile)
	mux.HandleFunc("POST /api/projects/{project}/home/mkdir", s.makeDir)
	mux.HandleFunc("POST /api/projects/{project}/home/upload", s.uploadFile)
	mux.HandleFunc("GET /api/projects/{project}/home/download", s.downloadFile)
	// What happened: terminal transcripts. (The working-tree diff + checkpoint
	// endpoints were removed — the git side panel runs in-guest via git.go, and
	// the host-side git they used was a guest→server RCE vector; ADR-035/037.)
	mux.HandleFunc("GET /api/projects/{project}/history", s.listHistory)
	mux.HandleFunc("GET /api/projects/{project}/history/search", s.searchHistory)
	mux.HandleFunc("GET /api/projects/{project}/history/{id}", s.readHistory)
	// Settings + admin.
	mux.HandleFunc("GET /api/notifications", s.listNotifications)
	mux.HandleFunc("GET /api/settings", s.settings)
	mux.HandleFunc("POST /api/settings/profile", s.updateProfile)
	mux.HandleFunc("PATCH /api/projects/{project}", s.renameProject)
	mux.HandleFunc("GET /api/projects/{project}/network", s.projectNetwork)
	mux.HandleFunc("PUT /api/projects/{project}/network", s.setProjectNetwork)
	mux.HandleFunc("GET /api/projects/{project}/resources", s.projectResources)
	mux.HandleFunc("PUT /api/projects/{project}/resources", s.setProjectResources)
	mux.HandleFunc("GET /api/settings/tools", s.toolDetails)
	mux.HandleFunc("PATCH /api/settings/runtime", s.updateRuntime)
	mux.HandleFunc("POST /api/settings/credentials/git", s.importGitCredential)
	mux.HandleFunc("GET /api/settings/credentials", s.credentialsList)
	mux.HandleFunc("POST /api/settings/credentials/import", s.importCredentials)
	mux.HandleFunc("POST /api/settings/credentials/token", s.putTokenCredential)
	mux.HandleFunc("DELETE /api/settings/credentials/{kind}/{name}", s.deleteCredential)
	mux.HandleFunc("GET /api/settings/credentials/{kind}/{id}/content", s.credentialContent)
	mux.HandleFunc("POST /api/settings/users", s.addUser)
	mux.HandleFunc("DELETE /api/settings/users/{email}", s.removeUser)
	mux.HandleFunc("GET /api/setup/status", s.getSetupStatus)
	mux.HandleFunc("POST /api/setup/start", s.startSetup)
	mux.HandleFunc("POST /api/setup/done", s.finishSetup)
	mux.HandleFunc("POST /api/setup/harvest", s.harvestSetup)
	mux.HandleFunc("DELETE /api/setup", s.discardSetup)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": Version})
	})

	// The embedded SPA: real files served directly, everything else falls
	// back to the prerendered SPA shell so client-side routes deep-link.
	// Unknown /api and /ws paths get a JSON 404; directories are excluded so
	// the bundle can't be listing-browsed.
	dist, err := fs.Sub(web.Dist, "dist/client")
	if err != nil {
		panic("embedded UI missing: " + err.Error()) // built into the binary; cannot fail at runtime
	}
	fileServer := http.FileServerFS(dist)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			writeErr(w, http.StatusNotFound, fmt.Errorf("no such endpoint: %s", r.URL.Path))
			return
		}
		// A preview-only guest passes authGuard only for a preview or shared
		// ROUTE (ADR-086 amendment 3), so none should reach here; this stays
		// as the second line: such a guest must NOT be served the SPA shell
		// (I1). Only real styrman users get the app.
		if !identity.FromContext(r.Context()).Authorized {
			writeErr(w, http.StatusNotFound, fmt.Errorf("no such endpoint: %s", r.URL.Path))
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if st, err := fs.Stat(dist, p); err == nil && !st.IsDir() {
				// Vite content-hashes /assets/* filenames — safe to cache
				// forever; a new build references new names.
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// The shell must never be cached: without this, browsers
		// heuristically cache it and keep loading a stale app after
		// deploys — invisible, maddening "my fix didn't deploy" bugs.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, dist, "_shell.html")
	})
	return mux
}

// previewHostRewrite maps a preview subdomain to its internal token route:
// <token>.STYRMAN_PREVIEW_BASE → /link/{token}/… (public, no login) and
// <token>.STYRMAN_PRIVATE_BASE → /shared/{token}/… (email-gated, OAuth at the
// ingress). The token lives in the subdomain, so the previewed app gets its
// OWN origin and root — its root-relative asset paths work unchanged. The two
// routes are distinct on purpose: the serve handlers reject a token of the
// wrong kind, so a private token pasted on the public host (or vice versa)
// can never change its auth model. No-op when neither base matches.
func (s *Server) previewHostRewrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		host = strings.ToLower(strings.TrimSuffix(host, ".")) // normalize (nit)
		bases := []struct{ base, route string }{
			{s.cfg.PreviewBase, "/link/"},
			{s.cfg.PrivateBase, "/shared/"},
		}
		// Longest base first: if one base is a suffix of the other (e.g.
		// crew.x + private.crew.x), the shorter would otherwise shadow the
		// longer and 404 every one of its links.
		if len(bases[1].base) > len(bases[0].base) {
			bases[0], bases[1] = bases[1], bases[0]
		}
		for _, m := range bases {
			if m.base == "" {
				continue
			}
			suffix := "." + m.base
			if !strings.HasSuffix(host, suffix) {
				continue
			}
			token := strings.TrimSuffix(host, suffix)
			if !links.ValidToken(token) {
				// A preview host whose subdomain isn't a live token must
				// serve NOTHING (not /api/health, not the SPA) — 404 (S6).
				http.NotFound(w, r)
				return
			}
			// Stash the original path so serveProxy's cold-attach redirect
			// bounces to the URL the visitor used, not the internal token
			// form (B1).
			r = r.WithContext(context.WithValue(r.Context(), origPathKey{}, r.URL.Path))
			r.URL.Path = m.route + token + r.URL.Path
			break
		}
		next.ServeHTTP(w, r)
	})
}

// origPathKey carries the pre-rewrite request path from previewHostRewrite to
// serveProxy (for the cold-attach redirect).
type origPathKey struct{}

// redirectSlash sends /p/x/1/port/3000 → …/3000/ so the proxied app's
// relative asset paths resolve under the prefix. The query string must
// survive the redirect (audit finding: a preview URL with ?params silently
// lost them, and browsers cache the 301).
func redirectSlash(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Path + "/"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// csrfGuard rejects cross-origin mutating requests. Browsers always send
// Origin on cross-origin POST/DELETE; same-origin requests either omit it or
// match Host. curl and other non-browser clients send no Origin and pass.
// (WebSocket origin checking is handled separately by websocket.Accept.)
func csrfGuard(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Preview / link routes merely tunnel bytes to a guest app — styrman's
		// CSRF concern (protecting its OWN API) doesn't apply, and the JSON body
		// cap would truncate the app's uploads. A sandboxed preview also sends
		// Origin: null, which would 403 every mutation the app makes (B2).
		// Judged by the ROUTE the request will take, never its decoded path:
		// ".../terminals/..%2F..%2F..%2F..%2Flink%2Fx" cleans to /link/x but
		// routes to the terminal API (security addendum 5a8afa1..07f5517).
		if isTunnelRoute(mux, r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					writeErr(w, http.StatusForbidden, fmt.Errorf("cross-origin request rejected"))
					return
				}
			}
			// The body cap is applied by budgetGuard, from the matched route
			// (bodyCap): uploads carry real payloads; editor saves, /format
			// and /check carry file content.
		}
		next.ServeHTTP(w, r)
	})
}

// who returns the request's user and their space.
func (s *Server) who(r *http.Request) (identity.User, *userSpace, error) {
	u := identity.FromContext(r.Context())
	sp, err := s.space(u.Username)
	return u, sp, err
}

// getProjectFor validates that the user owns the named project.
func (s *Server) getProjectFor(r *http.Request) (identity.User, *userSpace, project.Project, error) {
	u, sp, err := s.who(r)
	if err != nil {
		return u, sp, project.Project{}, err
	}
	p, err := sp.projects.Get(r.PathValue("project"))
	return u, sp, p, err
}

type projectView struct {
	project.Project
	State string `json:"state"`
	// DeleteError carries the last failed background destroy's message —
	// the DELETE already answered 202, so this is the only way the user
	// learns the delete didn't stick.
	DeleteError string `json:"deleteError,omitempty"`
	// ProvisionError carries the last failed background repo provisioning
	// (clone / GitHub repo create). Same reason as DeleteError: POST
	// /api/projects answers 201 the moment the record exists, long before the
	// clone runs, so this is the only channel back to the user. A project
	// whose clone failed still exists as an empty repo (R13) — the message is
	// what tells them why it is empty.
	ProvisionError string `json:"provisionError,omitempty"`
	// LanAccess is the project's network mode (ADR-067): true = LAN, false =
	// SANDBOX. One boolean off the record, because there is no root-side
	// state to reconcile it against and therefore no derived state to report.
	LanAccess bool `json:"lanAccess,omitempty"`
}

// guestErrMaxBytes caps how much guest-controlled text reaches the API.
const guestErrMaxBytes = 300

// guestErrMessage condenses a guest command's output into something a user can
// act on. The guest is untrusted and controls this text, so it is stripped of
// control and formatting characters, redacted (a token can ride in a remote
// URL) and capped — it is rendered as React text, never as markup.
func guestErrMessage(prefix, out string) string {
	msg := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\r", ""))
		// git's useful line is the diagnosis, not the "Cloning into…" banner.
		if line == "" || strings.HasPrefix(line, "Cloning into") {
			continue
		}
		msg = line
		if strings.HasPrefix(line, "fatal:") || strings.HasPrefix(line, "ERROR:") {
			break
		}
	}
	// Strip FIRST: a control byte planted inside a URL would otherwise break
	// userinfoRe's match and leave the credential in the clear (review nit).
	msg = redactSecrets(stripControls(msg))
	if len(msg) > guestErrMaxBytes {
		msg = clampBytes(msg, guestErrMaxBytes) + "…"
	}
	if msg == "" {
		return prefix
	}
	return prefix + ": " + msg
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	statuses, err := s.orch.Adopt(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := []projectView{}
	for _, p := range sp.projects.List() {
		live := "absent"
		if st, ok := statuses[u.Username+"/"+p.ID]; ok {
			live = st.State
		}
		inFlight, delErr := s.deletingState(u.Username, p.ID)
		state := deriveState(inFlight, s.isProvisioning(p.ID), live)
		out = append(out, projectView{Project: p, State: state, DeleteError: delErr,
			ProvisionError: s.provisionError(u.Username, p.ID), LanAccess: p.LanAccess})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

type createProjectRequest struct {
	Name string `json:"name"`
	// GitInit is accepted for compatibility and ignored: every project is a
	// git repository (R13).
	GitInit bool `json:"gitInit"`
	// GitHub: "" or "none" = local repo only; "private"/"public" = also
	// create a GitHub repo via the user's gh credentials and push.
	GitHub string `json:"github"`
	// CloneURL: clone an existing repository as the workspace instead of
	// initializing an empty one. Mutually exclusive with GitHub.
	CloneURL string `json:"cloneUrl"`
}

// cloneURLRe accepts the common remote shapes and nothing that could read
// as a git option or shell metacharacter. The clone itself passes the URL
// as a bare argv element after "--".
var cloneURLRe = regexp.MustCompile(`^(https://|ssh://|git@)[\w.@:/~+-]+$`)

// cloneURLHasSecret reports whether a clone URL embeds a PASSWORD — a token or
// secret in the URL itself.
//
// Such a URL is refused rather than redacted. Nothing needs it: the credential
// broker already delivers the user's ssh key (so `git@host:path` works) and
// answers git's credential helper per HOST (so `https://host/...` works for any
// host onboarded via `gh auth token --hostname`, ADR-030/032). A token in the
// URL is a second, worse path that git writes verbatim into the project's
// .git/config on the host, where nothing redacts it — and refusing it at the
// boundary deletes that whole class rather than mitigating it (operator
// decision 2026-09-02, on the security review's recommendation).
//
// A bare USERNAME is not a secret and stays allowed: `ssh://git@host/o/r` is
// the standard SSH form, and `git@host:path` carries no scheme at all.
func cloneURLHasSecret(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return false // shape is already gated by cloneURLRe
	}
	_, hasPassword := u.User.Password()
	return hasPassword
}

// userinfoRe matches the userinfo of a scheme-bearing URL — where a token
// rides when someone clones `https://user:tok@host/...` (R10). The scp-style
// `git@host:path` form carries a USERNAME, not a secret, and is deliberately
// left readable.
var userinfoRe = regexp.MustCompile(`(?i)\b(https?|ssh|git)://[^\s/]*@`)

// redactSecrets strips userinfo from EVERY scheme-bearing URL in a string.
// It replaced an earlier positional scan (find the last "@" after "//") that
// was both lossy — it mangled a legitimate `https://host/o/r@v1.git` into
// `https://***@v1.git` — and wrong for guest command output, which is many
// lines and can name a remote more than once.
//
// `[^\s/]*@` is greedy and stops at the path, so it consumes up to the LAST
// "@" of the AUTHORITY — the same split git, curl and net/url use. Anchoring
// on the first "@" instead would leave the tail of a password containing "@"
// in the clear (security review 2026-09-02).
func redactSecrets(text string) string {
	return userinfoRe.ReplaceAllString(text, "$1://***@")
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	u, sp, err := s.who(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	if err := project.ValidateName(req.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.CloneURL != "" {
		if !cloneURLRe.MatchString(req.CloneURL) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid clone URL"))
			return
		}
		if cloneURLHasSecret(req.CloneURL) {
			writeErr(w, http.StatusBadRequest, errors.New(
				"remove the credential from the clone URL — styrman supplies your git "+
					"credentials automatically (ssh key and gh login, from first-time setup). "+
					"A token in the URL would be written into the project's .git/config"))
			return
		}
	}
	// A new project records the operator's current Docker disk default, so a
	// later change of that default never resizes it (ADR-083).
	p, err := sp.projects.Add(project.Project{Name: req.Name, CreatedAt: time.Now().UTC(), DiskGB: s.cfg.DockerDiskGB()})
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	// Per-project workspace dir (ADR-027) — server-controlled path outside
	// the shared home, bind-mounted into the microVM at ~/projects/<dir>.
	workdir := s.cfg.ProjectWorkspace(u.Username, p.ID)
	if err := os.MkdirAll(workdir, 0o700); err != nil {
		_ = sp.projects.Remove(p.ID)
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create workspace"))
		return
	}
	// The per-project home (ADR-031) — created here too so the Files "Home"
	// scope works before the first boot; the entrypoint seeds it on boot.
	// Through an os.Root at the tenant dir homes/<user>, which is server-only
	// since ADR-064 (the guest mounts are base/ and p/<uuid>); before that the
	// setup VM mounted it and a path MkdirAll would have followed a planted
	// link (release review 2026-09-04, BUG-003 class). Belt and braces now.
	if err := os.MkdirAll(s.cfg.TenantDir(u.Username), 0o700); err == nil {
		err = files.MkdirAllPerm(s.cfg.TenantDir(u.Username), filepath.Join("p", p.ID), 0o700)
		if err != nil {
			_ = sp.projects.Remove(p.ID)
			_ = os.Remove(workdir) // just created, empty (audit 2026-09-13: it leaked)
			writeErr(w, http.StatusInternalServerError, errors.New("cannot create project home"))
			return
		}
	} else {
		_ = sp.projects.Remove(p.ID)
		_ = os.Remove(workdir)
		writeErr(w, http.StatusInternalServerError, errors.New("cannot create project home"))
		return
	}
	// R13: the workspace is a git repo from second zero. A clone fills the
	// (still empty) directory in the background boot instead.
	if req.CloneURL == "" {
		if err := gitrepo.EnsureRepo(r.Context(), workdir); err != nil {
			_ = sp.projects.Remove(p.ID)
			// gitrepo errors embed git's output, which can carry host paths
			// (its own doc says it must not reach a response verbatim).
			s.logger.Error("project git init failed", "project", p.Name, "error", err)
			writeErr(w, http.StatusInternalServerError, errors.New("could not initialize the project repository"))
			return
		}
	}

	// Respond immediately and boot the microVM in the background: a
	// seconds-long POST proved fragile in browsers (connection reuse during
	// the docker network churn it causes), and the terminal attach waits for
	// readiness anyway (per-project lock serializes with this boot).
	guest := sp.profile.GuestUser()
	// No runtime image configured = handler-level tests: skip the boot (it
	// would otherwise reach the host's real Docker daemon and race test
	// cleanup with its writes). config.Load always sets an image.
	if s.cfg.Image == "" {
		s.logger.Warn("no runtime image configured — skipping microVM boot", "project", p.Name)
		writeJSON(w, http.StatusCreated, projectView{Project: p, State: "creating"})
		return
	}
	s.provisioning.Store(p.ID, struct{}{})
	go func() {
		defer s.provisioning.Delete(p.ID) // every return below releases the state
		defer func() {                    // outlives the request — net/http's recover doesn't cover it
			if rec := recover(); rec != nil {
				s.logger.Error("background project boot panicked", "project", p.Name, "panic", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		// A delete can land between the 201 and this boot. Booting anyway
		// could win the per-project lock ordering against Destroy and
		// resurrect a container+network for a record that no longer exists —
		// an orphan microVM invisible to the UI (audit finding; same guard as
		// ensureFor and startProject).
		if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
			s.logger.Info("background boot skipped — delete in flight", "project", p.Name)
			return
		}
		spec := provisionSpec{
			user:     u.Username,
			project:  p.ID,
			name:     p.Name,
			workdir:  workdir,
			cloneURL: req.CloneURL,
			github:   req.GitHub,
			attach:   func() bool { return s.orch.EnsureSelfAttached(u.Username, p.ID) },
		}
		containerID, err := s.orch.EnsureRunning(ctx, u.Username, p.ID, s.runOptsFor(sp, u.Username, p, guest))
		if err != nil {
			s.logger.Error("background project boot failed", "project", p.Name, "error", err)
			s.bootFailed(ctx, spec)
			return
		}
		// Repo provisioning runs in-guest as the user so their gitconfig,
		// ssh key, and gh login apply. The bare `git init` case needs none
		// of that and already happened server-side before the 201.
		home := "/home/" + guest
		guestWorkdir := home + "/projects/" + p.Dir
		run := func(cmd ...string) (int, string, error) {
			return s.guest.Exec(ctx, containerID, orchestrator.ExecSpec{
				User:    guest,
				WorkDir: guestWorkdir,
				Env:     []string{"HOME=" + home, "USER=" + guest},
				Cmd:     cmd,
			})
		}
		spec.run = run
		s.provisionRepo(ctx, spec)
	}()
	writeJSON(w, http.StatusCreated, projectView{Project: p, State: "creating"})
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	st, err := s.guest.Status(r.Context(), u.Username, p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	inFlight, delErr := s.deletingState(u.Username, p.ID)
	state := deriveState(inFlight, s.isProvisioning(p.ID), st.State)
	writeJSON(w, http.StatusOK, projectView{Project: p, State: state, DeleteError: delErr,
		ProvisionError: s.provisionError(u.Username, p.ID), LanAccess: p.LanAccess})
}

// deleteProject responds 202 and destroys in the background: a Kata VM
// teardown can legitimately take minutes, far past any sane fetch timeout,
// and a dropped connection used to surface as "Failed to fetch" for a delete
// that then completed anyway. The UI polls the project list; the project
// shows state "deleting" until the record disappears.
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	key := u.Username + "/" + p.ID
	s.deletingMu.Lock()
	if s.deleting[key] {
		s.deletingMu.Unlock()
		w.WriteHeader(http.StatusAccepted) // already in flight — idempotent
		return
	}
	s.deleting[key] = true
	delete(s.deleteErrs, key)    // a fresh attempt clears the previous failure
	delete(s.provisionErrs, key) // the record is going away; so does its message
	s.deletingMu.Unlock()
	wipe := r.URL.Query().Get("wipe") == "true"
	go func() {
		defer func() {
			// This goroutine outlives the request and net/http's per-request
			// recover — a panic here would take down the whole server.
			if rec := recover(); rec != nil {
				s.logger.Error("project delete panicked", "project", p.Name, "panic", rec)
				s.deletingMu.Lock()
				s.deleteErrs[key] = "internal error during delete"
				s.deletingMu.Unlock()
			}
			s.deletingMu.Lock()
			delete(s.deleting, key)
			s.deletingMu.Unlock()
		}()
		// Detached context: nothing client-side can cancel a half-done
		// destroy and strand container-gone/record-present state.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		// NOTE: the v1.14.0 pre-wipe safety backup was removed with the whole
		// backup feature (operator decision 2026-09-01: no backups at all —
		// durability is git). A wipe-delete is now plainly irreversible; the
		// UI's confirm dialog is the only gate.
		if err := s.orch.Destroy(ctx, u.Username, p.ID); err != nil {
			// Record stays; the project reappears in the list (flagged with
			// deleteError) and the user can retry.
			s.logger.Error("project destroy failed", "project", p.Name, "error", err)
			s.deletingMu.Lock()
			s.deleteErrs[key] = err.Error()
			s.deletingMu.Unlock()
			return
		}
		// Credentials go with the record either way. A non-wipe delete keeps
		// the user's WORK (the workspace) but must not leave their ssh private
		// key and tokens in an orphaned home nothing can reach (security
		// review 2026-09-02). Harmless when wiping — the dir goes next.
		s.sweepProjectCredentials(u.Username, p.ID)
		// wipe=true also removes the workspace, docker storage, transcripts,
		// and apt manifest — irreversible, so it is opt-in per request.
		if wipe {
			// Per-project storage (ADR-031/027) is server-controlled — the
			// whole per-project home (docker disk, transcripts, apt manifest,
			// dotfiles, any creds) AND the workspace. os.RemoveAll doesn't
			// follow symlinks out.
			wipeFailed := false
			for _, dir := range []string{
				s.cfg.ProjectHome(u.Username, p.ID),
				s.cfg.ProjectWorkspace(u.Username, p.ID),
			} {
				err := os.RemoveAll(dir)
				if err != nil {
					// Pre-v1.11.1 homes carry ROOT-owned bind-mount stubs the
					// unprivileged server can't delete — empty the dir as root
					// via the server's own container, then retry (ADR-044).
					if rerr := s.orch.RootRemoveContents(ctx, dir); rerr == nil {
						err = os.Remove(dir)
					} else {
						s.logger.Warn("root-assisted wipe unavailable", "path", dir, "error", rerr)
					}
				}
				if err != nil && !os.IsNotExist(err) {
					s.logger.Error("wipe failed", "project", p.ID, "path", dir, "error", err)
					wipeFailed = true
				}
			}
			// A requested wipe that only partially succeeded must NOT then remove
			// the record — that would report the project (and its materialized
			// ssh key + agent tokens) as gone while the secrets stay on disk
			// (M18). Keep it visible with a deleteError so the user can retry.
			if wipeFailed {
				s.deletingMu.Lock()
				s.deleteErrs[key] = "wipe incomplete — some files could not be removed; retry"
				s.deletingMu.Unlock()
				return
			}
		}
		// Under netMu so a concurrent mode change cannot write this project's
		// record between the check and its removal (ADR-067; the marker this
		// used to remove alongside is gone, but the record race is not).
		s.netMu.Lock()
		err = sp.projects.Remove(p.ID)
		s.netMu.Unlock()
		if err != nil {
			s.logger.Error("project record removal failed", "project", p.Name, "error", err)
			return
		}
		if err := sp.terms.DeleteProject(p.ID); err != nil {
			s.logger.Warn("terminal records cleanup failed", "project", p.Name, "error", err)
		}
		// S8: public links must not outlive the project they point at.
		if s.links != nil {
			if err := s.links.DeleteProject(p.ID); err != nil {
				s.logger.Warn("public-link cleanup failed", "project", p.Name, "error", err)
			}
		}
		// Project-scoped env variables die with the project (ADR-043) — they
		// must not leak into any future project reusing the id.
		if s.creds != nil {
			if err := s.creds.DeleteProjectAll(u.Username, p.ID); err != nil {
				s.logger.Warn("project env cleanup failed", "project", p.Name, "error", err)
			}
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

// deletingState reports whether a background destroy is in flight for the
// project, plus the last destroy failure (empty if none), for the list/get
// views and the boot-path guards.
func (s *Server) deletingState(user, projectID string) (inFlight bool, lastErr string) {
	s.deletingMu.Lock()
	defer s.deletingMu.Unlock()
	key := user + "/" + projectID
	return s.deleting[key], s.deleteErrs[key]
}

func (s *Server) startProject(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	// Same resurrection guard as ensureFor.
	if inFlight, _ := s.deletingState(u.Username, p.ID); inFlight {
		writeErr(w, http.StatusConflict, errors.New("project is being deleted"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	// Reuse sp from getProjectFor (N23: the old sp2, _ := s.space(...) ignored
	// the error and could nil-deref).
	if _, err := s.orch.EnsureRunning(ctx, u.Username, p.ID, s.runOptsFor(sp, u.Username, p, sp.profile.GuestUser())); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	go func() { // background pre-attach (see createProject)
		time.Sleep(time.Second) // let this response flush first
		s.orch.EnsureSelfAttached(u.Username, p.ID)
	}()
	writeJSON(w, http.StatusOK, map[string]string{"state": "running"})
}

func (s *Server) stopProject(w http.ResponseWriter, r *http.Request) {
	u, _, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.orch.Stop(ctx, u.Username, p.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": "stopped"})
}

// terminalView is a durable terminal record merged with live tmux state.
type terminalView struct {
	conversation.Record
	Live     bool `json:"live"`
	Attached bool `json:"attached"`
	Bell     bool `json:"bell"`
}

func (s *Server) listTerminals(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	live, err := s.tmux.Live(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username))
	if err != nil {
		// The RECORDS are the durable truth (R6); liveness is a best-effort
		// scan of the guest. While a container is going down (a stop, a
		// network-mode rebuild) that scan fails for 30–50 s, and a 500 here
		// made every terminal in the sidebar vanish behind an error for the
		// whole window (seen 2026-09-11 in the browser-view hand-walk and its
		// e2e gate). Report the records with nothing live instead, and say
		// why in the log.
		s.logger.Warn("terminals: live scan failed, reporting none live", "project", p.ID, "error", err)
		live = nil
	}
	out := []terminalView{}
	for _, rec := range sp.terms.List(p.ID) {
		v := terminalView{Record: rec}
		if l, ok := live[rec.ID]; ok {
			v.Live = true
			v.Attached = l.Attached
			v.Bell = l.Bell
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"terminals": out})
}

func (s *Server) createTerminal(w http.ResponseWriter, r *http.Request) {
	_, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty body = default title
	rec, err := sp.terms.Create(p.ID, clampTitle(body.Title))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, terminalView{Record: rec})
}

// updateTerminal updates a terminal record: title, autostart command, or
// both (PATCH semantics — absent fields are untouched; command "" clears).
func (s *Server) updateTerminal(w http.ResponseWriter, r *http.Request) {
	_, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Title   *string `json:"title"`
		Command *string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Title == nil && body.Command == nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("title or command required"))
		return
	}
	id := r.PathValue("term")
	if body.Title != nil {
		if *body.Title == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("title cannot be empty"))
			return
		}
		if err := sp.terms.Rename(p.ID, id, clampTitle(*body.Title)); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
	}
	if body.Command != nil {
		// The command runs as the user, in their own VM — not a security
		// boundary, but keep it one sane line (chain with && or ;).
		cmd := strings.TrimSpace(*body.Command)
		if len(cmd) > 500 || strings.ContainsAny(cmd, "\n\r\x00") {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("command must be one line, at most 500 characters"))
			return
		}
		if err := sp.terms.SetCommand(p.ID, id, cmd); err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pauseTerminal kills the tmux session but keeps the record: processes end,
// the transcript stays, and opening the terminal again starts a fresh shell
// under the same name. (While "live", closing the tab already keeps
// everything running — that's tmux — so pause is for reclaiming the session.)
func (s *Server) pauseTerminal(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	id := r.PathValue("term")
	if _, ok := sp.terms.Get(p.ID, id); !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("terminal %s not found", id))
		return
	}
	if err := s.tmux.KillSession(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username), conversation.Prefix, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteTerminal removes the record and kills its tmux session. The
// transcript in the shared home is left intact (ADR-016).
func (s *Server) deleteTerminal(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	id := r.PathValue("term")
	if err := s.tmux.KillSession(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username), conversation.Prefix, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := sp.terms.Delete(p.ID, id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAllTerminals closes every terminal in the project.
func (s *Server) deleteAllTerminals(w http.ResponseWriter, r *http.Request) {
	u, sp, p, err := s.getProjectFor(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	for _, rec := range sp.terms.List(p.ID) {
		if err := s.tmux.KillSession(r.Context(), u.Username, p.ID, s.guestUserFor(u.Username), conversation.Prefix, rec.ID); err != nil {
			s.logger.Warn("kill session failed", "project", p.Name, "terminal", rec.ID, "error", err)
		}
		_ = sp.terms.Delete(p.ID, rec.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	// /api/health is auth-exempt for the container's own probe (the HEALTHCHECK
	// curls localhost). Only that loopback caller triggers the Docker daemon
	// ping; an untrusted shared-bridge VM gets a static OK so it can't make the
	// server hammer dockerd on demand (low finding).
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if err := s.orch.Ping(r.Context()); err != nil {
			writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("docker unavailable: %w", err))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	if errors.Is(err, project.ErrNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}

// deriveState is the project state a view reports: a delete in flight wins
// over everything (the record is going away), provisioning wins over the
// live container state (the container may still be absent while the clone
// has not run — that IS the window the state exists to show), and otherwise
// the container's own state.
func deriveState(deleting, provisioning bool, live string) string {
	switch {
	case deleting:
		return "deleting"
	case provisioning:
		return "provisioning"
	default:
		return live
	}
}

func (s *Server) isProvisioning(id string) bool {
	_, ok := s.provisioning.Load(id)
	return ok
}

// clampTitle bounds a terminal title (N25): titles land in conversations.json
// and every list response, so cap the length and strip control characters.
func clampTitle(t string) string { return clampBytes(stripControls(t), 120) }

// stripControls drops the characters that let untrusted text lie about itself.
// Two whole Unicode categories rather than a hand-listed set: Cc (C0, DEL and
// the C1 block) via unicode.IsControl, and Cf — every invisible formatting
// character, which is what the bidi overrides and isolates (U+202A-U+202E,
// U+2066-U+2069), the zero-width marks (U+200B-U+200F) and the BOM all are.
// The enumerated version of this shipped missing U+061C and drifts by
// construction; categories cannot. U+2028/U+2029 are Zl/Zp rather than Cf, so
// they are named explicitly — they are line breaks, and this text is rendered
// as a single line.
func stripControls(t string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 {
			return -1
		}
		return r
	}, t)
}

// clampBytes truncates to max BYTES without ever splitting a multi-byte rune.
func clampBytes(t string, max int) string {
	if len(t) <= max {
		return t
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(t[cut]) {
		cut-- // never split a multi-byte rune (audit nit)
	}
	return t[:cut]
}
