# Security review remediation (v1.9.0)

Date: 2026-08-30. A full multi-agent code review of the whole repo (backend,
ingress, deploy, image, scripts) reported 62 findings — 1 critical, 7 high,
25 medium, 30 low (as tallied by the review). Nearly all were fixed and
shipped across 5 commits; two structural refactors were deferred (below).
Deployed live as styrman-server v1.9.0 alongside a rebuilt caddy ingress and
`styrman-runtime:dev`; `crew.styrman.xyz` verified HTTP 200 post-deploy.

## Commits

1. `0192afd` — security batch 1 (critical + high)
2. `a74cc1b` — backend correctness (medium)
3. `7f01b53` — ingress + deploy (medium)
4. `8ca6368` — keyring tier removal + backend/script lows
5. `fec702d` — ingress cleanup, reproducibility pins, IDOR regression test

## `0192afd` — critical + high

- **C1 (critical) — host-side git RCE.** Host-side `git` invocations ran
  against project workspaces that are bind-mounted, guest-writable trees
  (ADR-027): a project VM (root in its own microVM) could plant a
  `.git/config` with `core.fsmonitor`, `core.hooksPath`, or the `ext::`
  transport pointing at an arbitrary program, and any host-side git command
  against that tree would execute it as the styrman server process. Fixed by
  removing every host-side git code path that could observe an
  attacker-controlled config (the ADR-022 checkpoint machinery —
  `internal/httpapi/checkpoints.go` and `gitrepo`'s snapshot/restore/
  commit-tree functions — is deleted outright, not merely unused) and
  hardening the two that remain (`gitrepo.EnsureRepo`/`IsRepo`, i.e. `git
  init`/`git rev-parse`) with `-c core.fsmonitor= -c core.hooksPath=/dev/null
  -c core.pager=cat -c core.editor=false -c protocol.ext.allow=never -c
  uploadpack.packObjectsHook=` plus `GIT_CONFIG_GLOBAL=/dev/null`/
  `GIT_CONFIG_SYSTEM=/dev/null`; `init` only ever runs when no repo exists
  yet. The interactive git client (`internal/httpapi/git.go`, ADR-035)
  already ran in-guest and is unchanged. See ADR-039.
- **H2 — auth could fail open.** `config.Load()` previously let
  `STYRMAN_AUTH=header` start with an empty `STYRMAN_INGRESS_TOKEN`, which
  meant the server trusted a raw client-supplied `X-Caddy-Email` from
  anything reachable on a project bridge — full identity spoofing, admin
  included. Now refuses to start in that case; an unrecognized `AuthMode`
  value is also rejected outright instead of silently falling through.
  `internal/config/config.go`.
- **H3 — username-slug collision across authorized users.** Two authorized
  emails that `identity.DeriveUsername` mapped to the same slug would share
  one per-user workspace — see and delete each other's projects, files, and
  broker credentials. `userstore.New` now refuses to start if the existing
  allowlist has a collision, and `Store.Add` rejects a new email that would
  collide with an already-authorized one. `internal/userstore/userstore.go`
  (+ regression test).
- **H4/H7 — unbounded guest exec output.** `Orchestrator.Exec` (readiness
  probes, tmux bookkeeping, git status/diff, stats) held a project's entire
  combined stdout/stderr in memory; the guest is untrusted root in its own
  VM and could OOM the shared server. Capped at 2 MiB via a `cappedBuffer`
  that keeps draining (so the command still completes and its exit code is
  read) but discards past the cap. `internal/orchestrator/exec.go`.
- **H6 — credential re-import read stale broker data.** The login-shell
  credential snippet normally overwrites a project's tool config with the
  broker's copy on every shell; running it during a harvest/import meant
  reading back what the broker just pushed instead of the guest's real
  current file, silently clobbering a token the user had just re-authed.
  Fixed with `STYRMAN_SKIP_CRED_PULL=1` set on the harvest's exec
  environment so the pull-on-login step is skipped for that one shell.
  `internal/httpapi/credentials.go` (`guestOut`), `image/configs/
  styrman-credentials.sh`.
- **H8 — OSC 52 clipboard read exfiltration.** xterm.js's clipboard addon
  implements both OSC 52 write (copy) and read (query); a `\x1b]52;c;?`
  sequence from the (untrusted) guest would have handed it the user's real
  system clipboard — including anything just copied from the credential
  viewer. Read is now hard-denied (`readText: () => ""`); write stays
  enabled only for the system selection so TUI copy (tmux/vim yank) keeps
  working. `web/src/lib/terminal.ts`.

## `a74cc1b` — backend correctness (medium)

- **M9 — exec stdin via base64-in-argv hit the argv size limit.** Large
  payloads (e.g. server-side file formatting) are now piped over the exec's
  stdin stream with an explicit half-close (EOF), not base64-encoded into
  the command line.
- **M17 — credential snippet could strand on a stale broker IP.** The broker
  address baked into the container env at project-creation time goes stale
  after a network recreation (ADR-036 hands the bridge a new subnet); the
  login-shell snippet now derives the broker's address from the VM's own
  current IP (`broadcast-1` of its own `/16`) at each login instead of
  trusting the injected value. `image/configs/styrman-credentials.sh`.
- **M18 — a failed project delete could vanish from the UI.** Delete errors
  are kept visible (`deleteError`) instead of dropping the failed attempt
  silently, so the user can retry. `internal/httpapi/server.go`.
- **M19 — a credential-store save could stall indefinitely.**
- **M20 — in-memory store didn't roll back on a failed disk write.**
  `userstore.Add` (and other per-entity stores) now revert the in-memory
  change if `save()` fails, so memory matches disk instead of silently
  drifting ahead of it.
- **M21 — invalid project records were silently dropped on load.**
  `project.Store`'s migration path now quarantines records with an invalid
  UUID/dir to a `.rejected` sidecar file instead of letting `save()`
  (which rewrites from valid records only) erase them from disk forever.
  `internal/project/store.go`.
- **M22 — revoking a credential didn't sweep already-materialized copies.**
- **M30 — shutdown didn't fully drain in-flight work.** The credential
  broker's HTTP listener now closes on context cancellation, and
  `cmd/styrman/main.go` warns loudly if graceful shutdown doesn't finish
  draining rather than exiting mid-drain silently.
- **M31 — some guest execs had no deadline.** Several callers passed a
  request-scoped or bare background context into `Orchestrator.Exec` with no
  timeout, so a wedged guest command could pin a goroutine and a hijacked
  Docker connection forever. `Exec` now guarantees a 30s deadline when the
  caller's context has none. `internal/orchestrator/exec.go`.

## `7f01b53` — ingress + deploy (medium)

- **M15 — unverified Kata tarball extraction.** `host-setup.sh` previously
  warned and continued if it couldn't obtain a SHA256 for the Kata release
  tarball before extracting it as root into `/`. Now fails closed: refuses
  to extract without a verified checksum (env override `KATA_SHA256`
  available for offline installs).
- **M23 — a bad Caddy config reload could wedge the ingress on stale
  config forever.** `deploy/caddy/ingress/watcher.go`.
- **M24 — public preview host didn't strip client-supplied identity
  headers.** A public (`*.preview` / no-login) app route now strips any
  inbound identity/ingress headers before proxying, so a preview visitor
  cannot inject a forged `X-Caddy-Email` downstream. `deploy/caddy/ingress/
  generator.go` (+ regression test in `generator_test.go`).
- **M26 — ingress test/doc gap** closed.
- **M27 — `docker.sock` mount comment clarified** (Caddy's host-root Docker
  access via the ingress watcher; documented as an accepted, known
  trade-off rather than an unexplained bind mount — see also TODO.md's
  deferred I14 docker-socket-proxy item).
- **M32 — OpenTofu install verification.**

## `8ca6368` — keyring tier removal + backend/script lows

- **Dead broker keyring tier removed.** ADR-038 (2026-08-29) had already
  moved agy's credential to the file tier and left the keyring tier
  (`credstore.KeyringCred`/`PutKeyring`, `GET /keyring`, the login
  snippet's `secret-tool store` block) in place as dormant/unused code,
  tracked as a TODO cleanup. This batch deletes it outright — no broker
  code, store kind, endpoint, or shell step references keyring credentials
  anymore. See ADR-040.
- A batch of additional low-severity backend and shell-script findings from
  the review, fixed alongside the keyring removal (not individually
  itemized here).

## `fec702d` — ingress cleanup, reproducibility, IDOR test

- **M14 / #14 — ingress hostname sanitization.** `sanitizeHostname` /
  `sanitizePolicyName` in `deploy/caddy/ingress/labels.go` and
  `generator.go` harden container-name-derived subdomains and Caddy policy
  names against malformed/hostile input (regression tests in
  `labels_test.go`).
- **#15 — removed the unused `picture_extractor` code path** from the
  ingress build.
- **#16 — removed the unused `caddy-l4` module/dependency** from the
  vendored Caddy ingress.
- **#21 — removed stale committed `web/dist` build artifacts** left over
  from earlier releases.
- **#10 — `daemon.json` pinned for reproducibility.** `host-setup.sh`'s
  daemon.json merge now also pins `live-restore: true`, `log-driver:
  json-file` with bounded `log-opts` (`max-size: 50m`, `max-file: 3`), and
  `ipv6`/`ip6tables: false` at the Docker daemon (the project-isolation
  firewall is IPv4-only, so an IPv6-enabled bridge could otherwise bypass
  it) — not just the pre-existing `mvm` runtime + time-namespaces flags
  (ADR-009).
- **#11 — Docker CE version pinned.** `host-setup.sh` installs a pinned
  `DOCKER_VERSION` (currently `29.7.2`) instead of always tracking latest.
- **#3 — cross-tenant IDOR regression test.** New
  `TestProjectIsolationAcrossUsers` (`internal/httpapi/isolation_test.go`)
  asserts two distinct authorized users can never reach each other's
  projects by UUID and that one user's project list never leaks another's —
  closing a gap the review flagged (this class of check existed for
  preview-guest access, ADR-024, but not for two full authorized users).

## H5 — firewall rework (deploy/host-setup.sh, applied live separately)

Not part of the 5 commits' automated deploy — a host firewall change needs
the operator's own privileged apply. `styrman-firewall.sh` was reworked:

- Dedicated `STYRMAN-IN`/`STYRMAN-FWD` chains, populated by
  INSERT-IF-MISSING rather than the old delete-then-insert self-heal timer —
  closes the every-60s window where isolation was briefly absent.
- A `physdev --physdev-is-bridged` RETURN exemption so a project can still
  reach the credential broker and port-preview proxy on its OWN bridge
  (same-bridge L2 traffic does traverse `DOCKER-USER` once
  `bridge-nf-call-iptables=1`, so without this exemption the broker was
  silently dropped alongside everything else — the regression this fixes).
- IPv6 handling: `disable_ipv6` set on every `sdb-*` bridge, plus
  best-effort mirrored `ip6tables` drop rules, so a future IPv6-capable
  bridge can't bypass an IPv4-only firewall.

**This firewall change was applied live on the host as a separate operator
step** (it needs root and rewrites live iptables state) — it is not gated by
or verified through the same deploy as the other 5 commits and should be
confirmed present (`iptables -S STYRMAN-FWD`, `systemctl status
styrman-firewall.timer`) independently.

## Deploy

Rebuilt and redeployed: `styrman-caddy` (ingress), `styrman-server` (v1.9.0),
`styrman-runtime:dev`. `curl -L https://crew.styrman.xyz/` → HTTP 200
post-deploy.

## Deferred (not in this batch)

Two findings were pure maintainability refactors with no security or
correctness impact, deliberately left out of this remediation to keep it
reviewable, and pushed to TODO.md for a separate change:

- **`web/src/components/SidePanel.tsx` split** — currently ~969 lines
  (the review's own count may have been taken at a slightly different
  point, reported as ~1008); should be split into `components/panel/*`
  (this was already flagged in the 2026-08-28 full audit at 770 lines and
  has grown since).
- **`harvestCredentials` living in the HTTP layer.** `(*Server)
  harvestCredentials` (`internal/httpapi/credentials.go`) mixes credential
  extraction/parsing logic into the `httpapi` package alongside request
  handlers; moving it to a dedicated package (e.g. alongside `credstore`/
  `credbroker`) would separate concerns without changing behavior.

## Verification

- Spot-checked against the current source (no CI/test-run output captured
  in this session): `internal/gitrepo/gitrepo.go`'s hardened `-c` flags,
  `internal/orchestrator/exec.go`'s `cappedBuffer`/`maxExecOutput`,
  `internal/userstore/userstore.go`'s slug-collision guard (+
  `userstore_test.go`), `internal/httpapi/isolation_test.go`'s
  `TestProjectIsolationAcrossUsers`, `deploy/caddy/ingress/
  generator_test.go`'s M24 header-strip coverage, and `deploy/caddy/ingress/
  labels_test.go`'s `sanitizeHostname` coverage all exist in the tree as
  described above. `credstore`/`credbroker` keyring symbols (`KeyringCred`,
  `PutKeyring`, `GET /keyring`) no longer exist in any `.go` source file —
  only historical docs/changelogs mention them now.
- Live: `crew.styrman.xyz` returns HTTP 200 after the redeploy of caddy,
  styrman-server (v1.9.0), and `styrman-runtime:dev`.
- **Not yet independently verified in this session:** the H5 firewall
  rework's live effect on the host (needs the operator's own
  `iptables -S` / timer check, per the note above).
