---
owner: explorer
date: 2026-09-08
---
# Recon — whole-project structure/uniformity/dead-code/size audit (2026-09-08)

## Question
Inventory and structure map of the whole styrman-crew repo: package sizes and
dependencies, uniformity across duplicated patterns, dead-code candidates
(grep-verified), and function/file size outliers — extra attention on the
ADR-066-churned areas (`deploy/firewall/`, `internal/netpolicy/`,
`internal/credbroker/`, `internal/httpapi/network.go`, `internal/project/store.go`).

## Summary
The repo is one Go module (`go.mod` at root) plus three separate modules
(`deploy/caddy/ingress`, `image/harness`, and a throwaway
`.agents/scratch/sec-2026-09-04/go.mod`). `internal/httpapi` is by far the
largest package (10,288 lines across 33 files) and does routing, auth,
provisioning, git, file explorer, credentials, network policy API, and proxy
all in one package — the one real "doing two (or six) unrelated jobs"
candidate. No orphaned exported symbols were found among the ones checked
(every candidate — `ParseStatus`, `RestoreLanAccess`, `isHostReachingFile`,
`ValidateName`, both env vars that looked unused in compose —
resolved to a real caller or a documented intentional escape hatch). The
`writeJSON` helper is duplicated (different signatures) in
`internal/httpapi/server.go` and `internal/credbroker/broker.go` — same
name, same job, two packages, not shared. The longest functions are
`main()` (221 lines), `Bridge.serve` (204), `Server.RoutesWithAuth` (184),
`Orchestrator.EnsureRunning` (177) and `Server.createProject`/`deleteProject`
(134–138) — all in the packages the guide already flags as doing multiple
jobs. `deploy/firewall/styrman-firewall.sh` (776 lines) and
`deploy/update/styrman-update` (695 lines) are root-run bash with real
per-concern function decomposition (30+ named functions each), not a single
giant script.

## Relevant files

- `internal/httpapi/*.go` (10,288 lines, 33 files) — REST/WS routing
  (`server.go`, 1,325 lines, `RoutesWithAuth`), auth guard (`auth.go`), git
  client (`git.go`, 907 lines), file explorer (`explorer.go`, 692 lines),
  credentials (`credentials.go`, `credharvest.go`), provisioning
  (`provision.go`), network policy API (`network.go`, 345 lines, ADR-066
  churn), proxy (`proxy.go`), checks/lint (`check.go`), stats, links, shares,
  setup roster — one package, many responsibilities.
- `internal/orchestrator/orchestrator.go` (1,388 lines) — Kata microVM
  lifecycle: create/EnsureRunning/EnsureSetup/CleanupLegacy, plus the
  symlink-confinement helpers `realBelow`/`projectHomeIsReal`/
  `mkdirAllConfined` used by the guest-writable-paths rule.
- `internal/netpolicy/{dir,status}.go` (1,049 lines total) — `Dir` (marker
  files + kick spool, `dir.go`) and `Status`/`ParseStatus`/`Availability`/
  `Derive` (status-file grammar + UI state derivation, `status.go`) — two
  cleanly separated concerns in one small package, both ADR-066 churn.
- `internal/credbroker/broker.go` (216 lines) — bridge-only HTTP surface
  (git/env/files) for the credential broker (ADR-030/032), ADR-066 churn
  (LAN-exposure gating via `lanExposed`/`isHostReachingFile`).
- `internal/project/store.go` (372 lines) — per-user JSON project store;
  `SetLanAccess`/`RestoreLanAccess` are the ADR-066 rollback pair used by
  `internal/httpapi/network.go`.
- `deploy/firewall/styrman-firewall.sh` (776 lines) / `install.sh` (351) /
  `test.sh` (412) — root-run iptables reconciler + installer + its own test
  suite; heavily churned for ADR-066 (per TODO.md, a known follow-up: base
  insert-if-missing repair ordering, deliberately deferred).
- `deploy/update/styrman-update` (695 lines) — root-run self-update script,
  same bash-function-per-concern style.
- `cmd/styrman/main.go` (891 lines) — entry point + legacy-layout migration;
  contains the two intentional exceptions to the `internal/files`-only
  confinement rule (`pruneTranscripts`, `migrateBaseHomes`), both carrying
  inline BUG-002/003/ADR-064 provenance comments.
- `web/src/{routes,components,lib}` — 11,402 lines of TS/TSX, not further
  broken down in this pass (out of stated audit scope's Go emphasis, but
  present).

## Flows (package dependency shape)

- `cmd/styrman` → wires `internal/config`, `internal/orchestrator`,
  `internal/httpapi`, `internal/credbroker`, `internal/netpolicy`,
  `internal/project`, `internal/userstore`, `internal/identity`; also does
  its own confined filesystem walks for the transcript janitor and the
  ADR-064 base-home migration (see BUG-002/003 comments in `main.go`).
- `internal/httpapi` is the hub: depends on `orchestrator`, `project`,
  `netpolicy`, `credstore`, `files`, `links`, `conversation`, `history`,
  `identity`, `config` — every other package feeds it, none of the others
  import it back (no cycle observed).
- `internal/netpolicy.Dir.readStatus` → `ParseStatus` (package-internal
  call, not a dead export — grep for the bare symbol name across the repo
  undercounts internal callers; had to check call sites inside the same
  package explicitly).
- `internal/httpapi/network.go` → `netpolicy.Availability`, `netpolicy.Derive`,
  `netpolicy.Dir.KickThrottled`, `project.Store.SetLanAccess`/
  `RestoreLanAccess` — the ADR-066 server-side surface.
- Path confinement: `internal/files` (`files.Trust` + `openRoot`) is the
  general-purpose confined-root API; `internal/orchestrator`'s
  `realBelow`/`projectHomeIsReal`/`mkdirAllConfined` is a second,
  independent implementation of the same symlink-safety idea, scoped to
  Docker mount-source validation (documented as necessary because Docker
  re-resolves bind sources on every start, per `guest-writable-paths.md`).
  `cmd/styrman/main.go`'s two `os.OpenRoot` calls are a third, one-off
  instance, each with an inline justification comment — not silently
  duplicated, but three separate confinement code paths to keep in sync.

## Conventions

- Error responses in `internal/httpapi` funnel through `writeJSON(w, status,
  v)` and `writeErr(w, status, err)` (`server.go:1280`); `internal/credbroker`
  has its OWN `writeJSON(w, v)` (no status arg, always 200) — same name,
  different signature, no shared helper package. Not a bug, but a real
  "same thing two ways" instance: any future third HTTP surface could copy
  either shape.
- Config is centralized: `internal/config.Load()` is the only place that
  reads `STYRMAN_*` env vars (confirmed — no other package/`os.Getenv` on a
  `STYRMAN_` var outside `internal/config`, `deploy/caddy/ingress` which is
  a separate module, and `image/harness` which is a separate binary).
- Path confinement is documented and enforced by
  `.agents/rules/guest-writable-paths.md`; violations are checked for at
  review time, not by a lint rule — this is a convention held by discipline
  and code comments, not tooling.
- Bash scripts (`deploy/firewall/styrman-firewall.sh`,
  `deploy/update/styrman-update`) both use small named functions
  (`log()`, `warn()`, `fail()` etc.) and both ship a sibling `test.sh` /
  rely on `.agents/docs/testing/`; `styrman-firewall.sh` has its own
  `test.sh` (412 lines) in the same directory — no analogous test file
  exists next to `styrman-update`.

## Risks

- `internal/httpapi` at 10,288 lines / 33 files is the single largest
  concentration of unrelated concerns (routing, auth, git client, file
  explorer, credentials, network policy, proxy, checks). Any change here
  touches a package with the widest blast radius in the repo; TODO.md and
  the 2026-09-08 review docs already show this area under heavy, repeated
  review pressure (three security review rounds recorded the same day for
  ADR-066 alone).
- `deploy/firewall/styrman-firewall.sh`'s TODO.md entry (line 15) is a
  live, acknowledged gap: the base insert-if-missing repair only runs on a
  60s timer, not immediately after a LAN-section change, deliberately
  deferred out of the ADR-066 gate. This is exactly the churned root-run
  script the dispatch asked to flag.
- Duplicate `writeJSON` in `httpapi` and `credbroker` is a small but real
  seam: a future author adding JSON responses to a third package has no
  canonical helper to reach for.
- Three independent symlink-confinement implementations
  (`internal/files`, `internal/orchestrator`'s `realBelow` family,
  `cmd/styrman/main.go`'s inline `os.OpenRoot` uses) all correctly
  documented but not unified — the guest-writable-paths rule file exists
  precisely because this pattern has caused three numbered bugs
  (BUG-002/003/004) before.
- `EnsureRunning` (177 lines, `internal/orchestrator/orchestrator.go`) and
  `RoutesWithAuth` (184 lines, `internal/httpapi/server.go`) are long
  enough that a reviewer skimming a diff inside them is likely to miss a
  local change to unrelated logic — both are doing genuine sequential setup
  work (not an obviously-uncut seam) but were not independently verified
  line-by-line for internal cohesion in this pass.

## Open questions

- Whether `web/src/{routes,components,lib}` (11,402 lines) has its own
  duplicated-pattern or dead-code issues was not investigated — this pass
  focused on the Go tree per the dispatch's explicit path list; a follow-up
  recon would need to grep TS/TSX separately (different idioms, different
  tooling — `npm run lint`/`tsc` already gate this at CI/local-check time
  per AGENTS.md).
- `deploy/update/styrman-update` has no sibling `test.sh` the way
  `deploy/firewall/` does — unclear whether that script is covered by
  `.agents/docs/testing/styrman/` some other way, or is untested; not
  confirmed either way in this pass.
- Did not exhaustively check every exported symbol in every package for
  dead-code — spot-checked the ones the dispatch's flagged directories
  surfaced (`netpolicy`, `credbroker`, `project.Store`) plus a few config
  keys; a full sweep of `internal/files`, `internal/conversation`,
  `internal/links`, `internal/history`, `internal/userstore` exported API
  was not performed.
- `internal/httpapi`'s "doing many jobs" framing is a structural
  observation, not a verified claim that any one file/function inside it
  mixes concerns badly at the code level — each individual file
  (`git.go`, `explorer.go`, `network.go`, etc.) is topically coherent; it is
  the PACKAGE boundary, not any single file, that is overloaded.
