---
owner: security
date: 2026-09-08
---
# Security review — per-project LAN access, server + UI half (ADR-065 phases 2–3)   (2026-09-08)

Range reviewed: `893bf03..working tree` — the uncommitted server/UI half
(`/tmp/.../phase23.diff`, 41 files) plus two in-flight fixes the author landed
while this review ran (`project.RestoreLanAccess`, `netpolicy.kickLocked`);
findings below are stated against the tree as of the last read. Blast radius
followed outward from the diff: the two new HTTP routes and every caller of
`projectView`/`GET /api/settings`, the marker dir and spool as the trust
boundary to the root reconciler, `ParseStatus` against the root-written status
file, the compose bind, and the two test harnesses. The host half
(`deploy/firewall/` at HEAD) was read only to check that this half honours its
contract — not re-reviewed; that is
`.agents/docs/reviews/2026-09-08-security-lan-phase1.md`.

**Excluded from the diff, and therefore from this review:** `web/dist/client/**`
— the bundle this change regenerates and `go:embed`s into the shipped binary.
See L5.

## Verdict: FINDINGS

The authorization model is sound and the contract with root is honoured:
enable is admin-only behind a 404-before-authz lookup scoped to the caller's
own store, disable never refused, the LAN prefix and the LAN-enabled project
count reach admins only, marker and kick names are regex-pinned so the only
thing crossing to root is a uuid, `ParseStatus` is bounded and fail-closed, and
`-race` is clean. What is broken is the *reporting* direction: the design is
fail-closed on granting and fail-**open** on telling the truth. When the status
file cannot be read — the run dir unmounted, `STYRMAN_FIREWALL_DIR` cleared, or
the file's group changed — a project whose rule is live in `STYRMAN-FWD` right
now reports `unavailable` with the words "host firewall support is not
installed", and its LAN chip disappears from the sidebar and the dashboard.
The markers stay on disk, `Sync` keeps recreating them from the records, and
root keeps applying them. An operator who removes the compose volume or the env
var to turn the feature off gets a UI that says nobody has LAN access and a
kernel that still grants it to every previously enabled project — the exact
inversion of the rule the rest of this code is careful about ("a project root
listed as applied is reported as an error, never as off").

## Findings

- **[medium / confidence: high]** `internal/netpolicy/status.go:157-163` (with
  `web/src/lib/lan.ts:39-40`, `internal/netpolicy/dir.go:141-161`,
  `internal/config/config.go:125-131`) — **a live LAN grant reads as
  "unavailable" and its chip vanishes whenever root's status cannot be read.**
  `Derive` with `st == nil` returns `StateUnavailable` + "host firewall support
  is not installed"; `lanChip`'s `default` branch renders nothing for
  `unavailable`, so the standing exception stops being flagged anywhere in the
  UI. Meanwhile nothing has been revoked: the marker files persist, the
  periodic `Sync` recreates them from any record with `LanAccess:true`, and the
  root reconciler keeps its `RETURN` rules in `STYRMAN-FWD`. Concretely: the
  operator deletes the `/run/styrman-firewall` bind from
  `deploy/docker-compose.yml:130` (or sets `STYRMAN_FIREWALL_DIR=` — the config
  comment and `api.md` both promise "empty disables the per-project LAN
  feature") and restarts; Settings shows "not installed", every project shows
  off or unavailable, and every project that had LAN access still reaches the
  LAN. The same state arrives by accident when `STYRMAN_UID/GID` or the state
  dir's group changes, because root writes `status` `0640 root:<state group>`
  and the server then gets EACCES. Remediation: (a) in `Derive`, when
  `lan == true` (or the marker exists) and the status is unreadable, return
  `StateError` with the wording the stale-and-applied branch already uses at
  `status.go:166-169` — "cannot confirm; the rule may still be present"; (b)
  make `lanChip` render the flag for any `lanAccess:true` regardless of state;
  (c) either have `Sync` delete every marker when `FirewallRunDir` is empty, so
  "disabled" actually tightens, or drop that claim from the config comment and
  `api.md`.

- **[medium / confidence: medium]** `internal/httpapi/network.go:140` — **the
  PUT's kick is the one unthrottled kick, and a no-op PUT still kicks.**
  `SetLanAccess` returns early when the value is unchanged
  (`internal/project/store.go:237`), but `Kick` runs unconditionally
  afterwards, and the disable direction needs only the project owner (no admin,
  no availability gate). Any authorized tenant can loop
  `PUT /api/projects/<own id>/network {"lanAccess":false}`: every request
  answers 200, writes no record, and drops one trigger file into root's spool,
  driving the root reconciler at the ceiling of
  `TriggerLimitBurst=20/60s` — and once that limit trips, the `.path` unit
  fails, so event-driven reconciliation (including the tightening that follows
  a genuine *disable* by anyone else) stops until the script revives it, at
  most once per ten minutes. Every other kick site is already throttled
  (`kickLAN` → `KickThrottled(p.ID, 10s)`). Remediation: kick only when
  `SetLanAccess` actually changed the record, or call
  `KickThrottled(p.ID, kickThrottle)` here too.

- **[medium / confidence: medium]** `deploy/docker-compose.yml:130` — **the
  spool the server writes may not be the bounded tmpfs root drains.** Docker
  binds `/run/styrman-firewall` with the default `rprivate` propagation, so a
  `styrman-server` container that started before the spool's tmpfs `.mount` unit
  first mounted keeps seeing the *underlying* directory at `…/spool` — operator
  owned, on plain `/run`, with none of the `size=1M,nr_inodes=1024` bound the
  install unit exists to impose. `.agents/docs/deploy/remote.md` documents that
  state and calls it harmless; it is harmless for correctness (kicks are simply
  invisible to root and toggles fall back to the 60 s timer) but not for
  availability: nothing drains that directory, so combined with the missing
  throttle above a tenant's PUT loop accumulates files in host RAM
  indefinitely. Remediation: bind `/run/styrman-firewall/spool` as its own
  compose volume (so the container always gets the tmpfs), or recreate the
  server whenever the mount unit is (re)started, and add the throttle from the
  previous finding.

- **[low / confidence: medium]** `internal/netpolicy/status.go:143` and `:180`
  — **root's per-project error text and `conf_error` reach a non-admin owner in
  `lanMessage`, and that text can contain the operator's LAN prefix.**
  `network.go:33-34` states the invariant ("a tenant never learns the
  operator's LAN prefix from a project they own") and enforces it for the
  `host` object, but `Derive`'s messages are returned to every owner through
  both `GET /api/projects/{id}/network` and `projectView.lanMessage`, and the
  root script builds per-project errors as `error=<uuid>:<cidr> <uuid>:insert
  failed` (`deploy/firewall/styrman-firewall.sh:389`; the repo's own `Derive`
  test uses exactly `error=<uuid>:192.0.2.0/24:insert failed`). Today every
  LAN-enabled project is owned by an admin, because enable is admin-only and
  scoped to the admin's own store — so the reachable path is an owner whose
  admin flag was removed from `STYRMAN_ADMIN_EMAILS`/the userstore while
  `lanAccess` stayed true, or a hand-edited record. Remediation: pass
  `lanMessage` verbatim only when `admin`, and give non-admins a fixed "the
  host firewall reported an error — ask an operator".

- **[low / confidence: high]** `cmd/styrman/main.go:39`,
  `internal/netpolicy/dir.go:89` — **the marker dir is world-readable**
  (`0o755` under a `0755` `/var/lib/styrman`), so any local uid on the host can
  enumerate which project UUIDs hold a standing LAN exception. Root goes out of
  its way to avoid exactly this for the status file
  (`styrman-firewall.sh:414-416`: "root `0640` with the state dir's group, so
  the server reads it and other local uids do not"). Remediation: create and
  keep the dir `0o750`; root reads it as root and the script's owner assertion
  is unaffected.

- **[low / confidence: low]** `internal/netpolicy/status.go:138` — **no guard
  against a status timestamped in the future.** `now.Sub(st.AppliedAt) >
  StaleAfter` is false for any `applied_at` ahead of the server's clock, so a
  skewed stamp makes the status permanently "fresh": `Availability` keeps
  accepting enables and `Derive` keeps reporting `on` long after the reconciler
  has stopped running. The input is root-written, so this is clock skew between
  the container and the host (or an NTP step), not an attacker. Remediation:
  treat `st.AppliedAt.After(now.Add(smallSkew))` as stale.

- **[low / confidence: high]** `internal/httpapi/server.go:1018-1021` — the
  delete path's marker removal runs **outside `netMu`**, so a concurrent
  `PUT {"lanAccess":true}` can recreate the marker between the removal and
  `projects.Remove`, leaving an orphan until the next `Sync` (≤2 min). No live
  rule follows (the bridge is gone with the container, so root records
  `pending=<uuid>:no-bridge`), and the code comment acknowledges the leftover —
  but taking `s.netMu` around those two lines closes it for the cost of one
  line, and removes the "record and marker diverge" case the rest of the design
  is built to exclude.

- **[low / confidence: high]** `web/dist/client/_shell.html` (and the rest of
  `web/dist/client/**`) — **the artifact this change actually ships was outside
  the reviewed diff, and part of it is unreadable to any reviewer.** The
  embedded SPA is regenerated in the working tree; `git diff --numstat` reports
  `- -` for `_shell.html`, `file` says `data`, and it carries one NUL at byte
  7417 inside a TanStack hydration payload. The NUL count is identical at HEAD
  (1), so this is BUG-007's known residual rather than anything introduced
  here, and it does not block this diff — but per `.agents/rules/writing-files.md`
  it is recorded rather than skipped, and the regenerated bundle should be
  named in the range whoever reviews the commit sees.

- **[low / confidence: medium]** `.agents/docs/testing/styrman/integration.sh`
  (live block) — when `STYRMAN_IT_LIVE_LAN=1`, the block plants
  `/var/lib/styrman/netpolicy/lan/<id>` where `<id>` belongs to the *scratch*
  server's store. A production `styrman-server` on the same host will see that
  marker as stray and delete it on its next 2-minute `Sync` (logging "lan
  access marker removed"), racing the block's 5 s assertion. Flaky rather than
  unsafe — the marker is removed by exact name either way — but worth a note in
  the block so a failure there is not mistaken for a reconciler defect. The
  non-opt-in path is clean: both harnesses point `STYRMAN_FIREWALL_DIR` at a
  scratch dir and never touch `/run/styrman-firewall` or the production marker
  dir (ADR-052 honoured).

## Checked

- **Secrets** — none added. `deploy/.env.example` gains a path, not a value; no
  token, key, or credential appears in the diff, and the audit line
  (`network.go:142`) logs actor email, project id and name only — the same
  fields the rest of the API already logs. Root's status file is parsed but
  never echoed to a non-admin except through `lanMessage` (see the low finding
  above).
- **Injection** — the only thing crossing to root is a filename: marker names
  are pinned by `uuidRe`, kick tags by `kickTagRe` (`^[a-z0-9-]{1,64}$`, so no
  `.`, `/` or `..`), and both are joined, never shelled. `ParseStatus` is a
  bounded key=value reader: `f.Stat()` on the open fd rejects >64 KiB and
  non-regular files, `io.LimitReader` + `bufio.Scanner` bound it again, garbage
  rejects the whole file (fail-closed to `nil`), and no branch can panic —
  including `applied_at` at `MaxInt64`, where `Time.Sub` clamps. PUT bodies are
  a two-field struct behind the csrfGuard's 1 MiB `MaxBytesReader`; a missing
  or non-boolean `lanAccess` is 400 before anything else. React renders every
  root-supplied string as text — no `dangerouslySetInnerHTML`.
- **Authz/authn** — `getProjectFor` resolves through the *caller's own* store,
  so another user's project is indistinguishable from a nonexistent one on both
  verbs (404 before any authorization signal; `isolation_test.go` now covers
  the route, `network_test.go` covers member GET/PUT and asserts no marker is
  written). Enable is `requireAdmin` after the 404 and after the 503
  availability gate; disable is owner-only by design. `host` /
  `settings.hostFirewall` — the LAN prefix, device, warnings and the count of
  LAN-enabled projects — are emitted only when `u.Admin`, which is the same
  identity the auth guard set. No client-side path bypasses anything: the
  disabled `Switch` is cosmetic and the server re-derives every gate.
- **Crypto** — nothing hand-rolled and nothing new. `BridgeNameFor`'s SHA-1 is
  a name derivation, not a security primitive, and
  `internal/config/bridgename_test.go` pins it against the shell's `sha1sum` so
  the two halves cannot drift.
- **Dependencies** — no new modules; `go.mod` and `package.json` are untouched,
  everything new is stdlib. `govulncheck ./...` on the local go1.25.0 toolchain
  reports 31 called vulnerabilities: 29 stdlib, all fixed at ≤ go1.25.13, which
  is what `deploy/Dockerfile.server` pins (`golang:1.25.13-trixie`), so the
  shipped binary is not affected — the local toolchain is what is behind. The
  remaining two are `github.com/docker/docker@v28.5.2+incompatible`
  (GO-2026-4887 AuthZ-plugin bypass, GO-2026-4883 plugin-privilege off-by-one),
  both daemon-side, both `Fixed in: N/A`, reached only through package `init`
  traces; pre-existing and unchanged by this diff. `npm audit` in `web/`: 0
  vulnerabilities (`--omit=dev` and full). `go vet ./...` clean;
  `go test -race ./internal/netpolicy/ ./internal/httpapi/ ./internal/config/
  ./internal/project/` all pass.
- **GCP surface** — none. This platform is self-hosted; `.agents/docs/deploy/gcp.md`
  records GCP as not applicable, and the diff adds no IAM, service account, or
  Secret Manager surface.
- **Contract with the root half** (checked, not re-reviewed) — markers are
  name-only and uuid-validated on both sides; the spool is root-parented with
  an operator-owned `0700` tmpfs inside, so the compose `rw` bind of a
  `root:root 0755` directory into a container running as `${STYRMAN_UID}:${STYRMAN_GID}`
  lets the server create spool entries and nothing else — it cannot replace
  `status`, and `O_CREATE|O_EXCL` means it cannot be tricked into following a
  planted symlink either; `status` is root-written `0640 root:<state group>`,
  readable by the server's primary gid (the compose comment saying `0644` is
  stale). A compromised server uid gains nothing here that it did not already
  have — it holds `/var/run/docker.sock`, which is host root by another name.
