---
owner: reviewer
date: 2026-09-13
---
# Audit — internal stores, config, cmd/styrman (2026-09-13)

Scope: `internal/config/config.go`, `internal/credstore/credstore.go`,
`internal/credbroker/broker.go`, `internal/userstore/userstore.go`,
`internal/links/store.go`, `internal/identity/identity.go`,
`internal/project/store.go`, `internal/project/uuid.go`,
`internal/gitrepo/gitrepo.go`, `cmd/styrman/main.go` — all non-test `.go`
files, at HEAD `e56e775` (clean).

This is an `/audit` scan against recorded intent (`AGENTS.md`,
`REQUIREMENTS.md` R1–R13, the three newest `.agents/docs/changes/` entries,
`.agents/docs/architecture/DECISIONS.md`, `.agents/docs/bugs/INDEX.md`), not
a diff review — every file above was read in full.

## Verdict: APPROVE WITH NITS

Nothing broken rises to high severity. This slice is unusually well
hardened: every store does atomic temp+fsync+rename writes, every mutation
rolls back its in-memory copy on a failed save, path inputs that reach the
filesystem are validated against traversal (`validRelPath`, `safe`,
`nameRe`, `uuidRe`), and `config.Load()` fails closed on every
security-relevant misconfiguration it recognizes (dev listener not
loopback, header auth without a token, a non-VM runtime without the
override flag). The one real gap is that this fail-closed discipline does
not extend to `STYRMAN_DOCKER_DISK_GB`, whose valid range is enforced only
on the operator-settings path, not at boot — a plausible misconfiguration
degrades every new project's Docker-in-Docker storage silently instead of
refusing to start.

## Findings

- **[medium / confidence: medium]** `internal/config/config.go:170` (`envIntOr("STYRMAN_DOCKER_DISK_GB", 20)`) — `Load()` accepts any integer for the per-project Docker disk size with no bounds check, while `SetOperator` (config.go:326) enforces 5–500 GB for the identical field reached through the Settings API, and `loadOverrides` (config.go:297) silently ignores an out-of-range `settings.json` value and keeps the (also unchecked) env value. A boot-time `STYRMAN_DOCKER_DISK_GB=0` (or a negative value, or a typo like a stray leading zero making it huge) flows unchecked into `orchestrator.go:417` → `image/entrypoint.sh`'s `truncate -s ${DISK_GB}G` / `mkfs.ext4`, which fails and falls back to the unbounded VFS storage driver for every newly created project on the deployment (entrypoint.sh:231-241) — a project-provisioning-scale bug (R1/scale targets: 10+ concurrent, 100+ total projects) with no operator-visible signal until a project's DinD misbehaves. Every other security/config invariant in this same function fails closed (`AuthMode`, `DevAddr`, `IngressToken`, `Runtime`); this one field is the exception. Fix: clamp/validate `dockerDisk` in `Load()` the same way `SetOperator` does (5–500), and fail loudly (log or refuse to start) rather than silently defaulting on an out-of-range or unparseable value, matching `loadOverrides`'s existing WARN-on-unparseable pattern.

- **[low / confidence: medium]** `internal/project/uuid.go:12-14` — `NewUUID`'s `crypto/rand.Read` failure path returns `"fallback-" + nanotime`, which does not match `uuidRe` (`internal/project/store.go:138`). `Store.Add` (store.go:155) calls `NewUUID()` and stores whatever it returns without validating the ID's shape; on the *next* `NewStore` load, that exact record fails `uuidRe.MatchString(p.ID)` (store.go:106) and is silently quarantined into `.rejected` (store.go:107-110) — a project created during a `crypto/rand` outage would vanish from the account on the next server restart. `crypto/rand` essentially never fails on Linux, so likelihood is very low, but the failure mode (quiet, permanent data loss of a live project, matching the shape of BUG-005/BUG-017's "invisible until the next reload" family) is easy to close: have `Add` reject/report a fallback ID instead of silently persisting a record its own loader will later reject.

- **[low / confidence: low]** `internal/config/config.go:503-519` (`envIntOr`, `envDurationOr`) — parse failures on `STYRMAN_VCPUS`, `STYRMAN_MEMORY_MB`, `STYRMAN_IDLE_TIMEOUT`, `STYRMAN_TRANSCRIPT_TTL` are swallowed with no log line, unlike `loadOverrides` (config.go:291-293) which WARNs on an unparseable `settings.json`. Not security-relevant (all have safe defaults), but a typo'd env var (e.g. `STYRMAN_IDLE_TIMEOUT=8hr` instead of `8h`) silently reverts to the 8h default with zero operator feedback. Low benefit, cheap fix: log at WARN when `strconv.Atoi`/`time.ParseDuration` fails, mirroring the settings.json path.

## Criteria

1. **Dead code** — pass. Checked every exported symbol in the slice against repo-wide callers (`SetupContainer`, `BridgeNameFor`, `EnsurePrivate`, `IsRepo`, etc.) — all have live callers outside their own tests; no orphaned functions or commented-out blocks found.
2. **Flow tracing** — pass, with the one gap above. Traced `credstore`/`credbroker` end to end (broker resolves caller by IP → store methods always receive a server-derived `u.Username`/project UUID, never a raw client-supplied path component — verified every `s.creds.*` call site in `internal/httpapi`); traced `DeleteProjectAll` to project deletion (server.go:1071, per ADR-043's stated invariant); traced `config.DockerDiskGB()` to `orchestrator.go:417` to `image/entrypoint.sh`'s disk creation, which is where the missing boot-time bound actually bites.
3. **Standards conformance** — pass. Every store follows the same atomic-write discipline (temp+fsync+rename, some with parent-dir fsync too); error wrapping is consistent (`fmt.Errorf` with `%w` where errors propagate); `gitrepo.go`'s hardened `-c` overrides and env sandboxing match the documented threat model (guest-controlled `.git/config`) exactly.
4. **Directory layout** — pass. Everything sits where `AGENTS.md`'s Key Files table says it should; no stray files.
5. **Simplicity** — pass. No unjustified dependencies; `NewUUID` is a correct minimal stdlib v4 UUID; no hand-rolled auth/crypto/session logic in this slice (auth is header-trust + ingress token, not home-grown crypto).
6. **Decoupling** — pass. Small, well-scoped interfaces (`credbroker.Resolver` is exactly what the broker needs); no cross-package reach-ins observed; `internal/config` is the single source of truth for paths/labels as its own doc comment claims, and every other package in this slice consumes it rather than re-deriving paths.

## Not examined

- `internal/httpapi/`, `internal/orchestrator/`, `internal/terminal/`, `internal/files/`, `internal/history/`, `internal/netpolicy/`, `web/` — out of the named scope; only spot-checked (via `grep`) as callers of the reviewed packages to confirm flow-tracing claims above, not reviewed for their own defects.
- `image/entrypoint.sh` and the rest of `image/` — read only the ~15 lines around `DISK_GB` to confirm the downstream effect of finding #1; not reviewed otherwise.
- Test files in every package listed (`*_test.go`) were intentionally excluded per the dispatch ("all non-test .go files").
- No `go vet`/`go build`/test run was performed as part of this audit (static read-through only).
