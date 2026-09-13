---
owner: reviewer
date: 2026-09-13
---
# Audit — internal/httpapi core (server/auth/profile/stats/projectenv/provision/network)

Scope: `internal/httpapi/server.go`, `internal/httpapi/auth.go`, `internal/httpapi/profile.go`,
`internal/httpapi/stats.go`, `internal/httpapi/projectenv.go`, `internal/httpapi/provision.go`,
`internal/httpapi/network.go` at commit `e56e775` (repo `styrman-crew`, branch `main`, clean).
Full-file audit against recorded intent (AGENTS.md, REQUIREMENTS.md R1–R13, the three newest
`.agents/docs/changes/` entries, `.agents/docs/specs/styrman/*.md`, `.agents/docs/bugs/INDEX.md`),
not a diff review — these files were not materially touched by the most recent change set
(only `server.go`, last touched at `c4128f5` for the terminals-500 fix, BUG-039, already recorded).

## Verdict: APPROVE WITH NITS

The `.go` code itself is clean: no dead code, every changed-in-spirit function traces cleanly
to its callers, error paths are wrapped or logged, and the network-mode / provisioning /
delete paths show the scars of real incidents (BUG-002/003/015/019/028/029/030/031/032,
R12/R13) properly closed. The one real defect found is outside the `.go` files: the API spec
this feature is supposed to be documented against has fallen out of sync with the ADR-067
redesign these very files implement, still describing the deleted ADR-065 root-firewall-spool
mechanism as live, including a dead env var no code reads anymore. Two low housekeeping items
round out the findings.

## Findings

- **[medium / confidence: high]** `.agents/docs/specs/styrman/api.md:15,200,266` — the spec
  documents `GET /api/projects` as returning `lanState`/`lanMessage`/`lanChangedAt` "root's last
  word on the per-project local-network exception (ADR-065)", `GET /api/settings` as returning
  `hostFirewall`, and `STYRMAN_FIREWALL_DIR` as a live env var the server reads/writes
  (`spool/`, `status`) — all three describing the ADR-065/066 design that ADR-067 (implemented
  in `network.go`, shipped v1.25.0 per the third-newest change entry) explicitly superseded.
  `projectView` in `server.go:657-675` carries only `LanAccess bool`; there is no `lanState`,
  `lanMessage`, `lanChangedAt`, or `hostFirewall` field anywhere in the Go source
  (`grep -rn "hostFirewall\|HostFirewall"` and `grep -rn "STYRMAN_FIREWALL_DIR"` across the
  repo's `.go` files both return nothing). A reader or future agent trusting api.md as the
  living contract for this exact feature (which `network.go`'s own doc comment cites ADR-067
  for) will build against fields that don't exist and expect a root-firewall spool mechanism
  the code deliberately deleted. Fix: update api.md's `GET /api/projects`, `GET /api/settings`,
  and the env var table to match the current `{lanAccess, canEnable}` / no-root-state design
  already correctly described at api.md:19-20.

- **[low / confidence: medium]** `internal/httpapi/server.go:836-853` (`createProject`) — if
  `os.MkdirAll(workdir, 0o700)` at line 831 succeeds but the subsequent tenant-home creation
  (`os.MkdirAll(s.cfg.TenantDir(...))` or `files.MkdirAllPerm(...)`) fails, the handler removes
  the project record (`sp.projects.Remove(p.ID)`) but never removes the just-created `workdir`
  directory — it leaks a directory under the tenant's workspace tree that nothing will ever
  reference again (the UUID is gone from the store). Not a security issue (R7's "no database"
  design tolerates orphaned state, and disk-only cost), but it is an incomplete rollback on a
  path that otherwise carefully unwinds the project record. Fix: `os.RemoveAll(workdir)` in that
  error branch, mirroring the cleanup already done for the workdir-creation failure two lines
  above.

- **[low / confidence: low]** `internal/httpapi/server.go:167-169` (`provisioning sync.Map`) —
  keyed by bare `p.ID` while every sibling in-flight map on `Server` (`deleting`, `deleteErrs`,
  `provisionErrs`, `credPushing`, `credPushRerun`) is keyed by `user+"/"+projectID`. Harmless
  today only because project IDs are server-generated UUIDs (ADR-020) and therefore globally
  unique in practice, but it is an inconsistent convention on the same struct and would become
  a real cross-tenant collision risk if project IDs were ever made user-supplied or
  non-UUID. Fix: key `provisioning` the same way as its neighbors for consistency, or add a
  one-line comment stating the UUID-uniqueness assumption it currently relies on silently.

## Criteria

1. **Dead code** — pass in the `.go` files; fail in the docs surface (api.md documents a
   removed mechanism and a dead env var — see finding above). No unused Go exports, no
   commented-out blocks, no unreachable branches found in the seven files.
2. **Flow tracing** — pass. `createProject`→background boot→`provisionRepo`/`bootFailed` is
   consistently ordered (attach before anything credentialed, per the file's own header
   comment and `provision_test.go`'s pinned order); `setProjectNetwork`'s inspect→decide→
   persist→rebuild sequence is entirely inside one `WithProject` lock scope with fail-closed
   replies on every branch (no bare `err != nil` folded into a default-allow, unlike the
   BUG-028/029/031 history this file's comments cite); `deleteProject`'s background goroutine
   recovers its own panics and releases `deletingMu` state on every path; every guest-facing
   error string in `provision.go`/`server.go` is redacted (`redactSecrets`) and capped
   (`guestErrMessage`) before reaching a response.
3. **Standards conformance** — pass. Errors wrapped with `%w` and context throughout; JSON
   responses use the `{"error":{"message":...}}` shape and camelCase per `json-api-style`;
   `gofmt`/`go vet` cleanliness assumed per the project's stated CI gates, not re-litigated here.
4. **Directory layout** — pass. All seven files sit in `internal/httpapi/`, consistent with the
   package's REST/WS routing role; `provision.go` is deliberately split out per its own header
   comment (keeps the attach-order sequence next to `provision_test.go`), which is a legitimate,
   documented layout decision, not drift.
5. **Simplicity / DIY-except-security** — pass. No hand-rolled auth/crypto: `authGuard` defers
   identity to the ingress header + `crypto/subtle.ConstantTimeCompare` for the shared token,
   authorization to `userstore`; no new dependencies introduced in this slice.
6. **Decoupling** — pass. `guestRuntime`/`modeRuntime` are narrow, consumer-declared interfaces
   sized to exactly the two/one calls each handler needs (documented rationale in `server.go`
   for why they exist and why they stay narrow); `provision.go` isolates the provisioning
   sequence behind an injectable `provisionSpec.attach`/`run` pair so it's unit-testable without
   Docker, a real second "caller" (the test suite) justifying that seam.

## Not examined

- `internal/httpapi/proxy.go`, `shares.go`, `browser.go`, `check.go`, `credharvest.go`,
  `settings.go`, `git.go`, and every other file in the package not named by the dispatch —
  referenced only incidentally here (e.g. to confirm `hostFirewall`/`STYRMAN_FIREWALL_DIR` are
  truly dead) and not reviewed for their own correctness.
- `internal/orchestrator`, `internal/project`, `internal/credstore`, `internal/conversation`,
  `internal/files` internals — read only as far as needed to verify the seven files' calls into
  them are consistent (e.g. `WithProject`/`ModeMismatch`/`lockedProject` in `orchestrator.go`,
  `SetLanAccess` in `project/store.go`); not audited in their own right.
- No tests were run (`go test ./...`, `go vet ./...`) as part of this audit — this is a static
  read against recorded intent, not a verification pass; the changelog entries cite prior green
  verifier runs for this range.
- Frontend (`web/`) and deploy/runtime-image surfaces are out of scope for this dispatch.
