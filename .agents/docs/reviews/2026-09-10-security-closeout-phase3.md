---
owner: security
date: 2026-09-10
---
# Security review — TODO close-out Phase 3 (2026-09-10)

Range reviewed: `.agents/scratch/phase3.diff` (working tree vs HEAD `7dae197` for
`.agents/docs/testing/styrman/integration.sh`, `deploy/caddy/ingress/go.{mod,sum}`,
`deploy/firewall/styrman-firewall.{service,timer}`, `deploy/host-setup.sh`,
`deploy/update/styrman-update.timer`, `internal/httpapi/{check,format}.go`,
`web/e2e/run.sh`, plus the new untracked
`.agents/docs/testing/styrman/ensure-test-image.sh`).

## Verdict: FINDINGS

The boot-window fix does not work. `deploy/firewall/styrman-firewall.service` gains
`[Install] WantedBy=multi-user.target`, and both that unit and the timer carry
comments asserting the base rules now land "right after docker.service" — but an
`[Install]` section only takes effect when the unit is *enabled*, and nothing enables
it: `deploy/firewall/install.sh:109` does `systemctl start styrman-firewall.service`,
`deploy/update/styrman-update:534` does the same, and `systemctl enable` appears in
this tree only for the timer. Measured on the production host right now:
`systemctl is-enabled styrman-firewall.service` -> `static`; after the new unit is
installed it becomes `disabled`, never `enabled`. So the only real change to the
boot window is the timer's 15 s -> 5 s, and ADR-065 follow-up (f) is recorded closed
on a mechanism that is inert. Second: `ensure-test-image.sh` takes its tag from argv
and both suites forward their own `$1`, so the repo's own documented invocation
`integration.sh styrman-runtime:dev` now rebuilds and re-stamps the image production
boots — the exact regression `web/e2e/run.sh:9-12` records as a 2026-09-01 audit
finding. Everything else is clean: no new called vulnerability from the eight bumps,
no injection, no secret, and the address-pool change introduces no isolation
regression.

## Findings

- **[medium / confidence: high]** `deploy/firewall/install.sh:109` (with
  `deploy/firewall/styrman-firewall.service:19-26` and
  `deploy/firewall/styrman-firewall.timer:5-9`) — **the new `WantedBy=multi-user.target`
  is never enabled, so the boot-time application it claims never happens.**
  `[Install]` is inert until `systemctl enable` writes the
  `multi-user.target.wants/` symlink. `install.sh` line 109 *starts* the service and
  line 110 enables only the *timer*; `deploy/update/styrman-update:534` likewise only
  starts it. Verified live: `systemctl is-enabled styrman-firewall.service` -> `static`
  (the installed unit predates the change; with the new unit it reports `disabled`).
  Scenario: host reboots, dockerd restarts every project container with restart policy
  `unless-stopped` and recreates its `sdb-` bridge; the bridge carries no `STYRMAN-IN`
  INPUT drop and no `DOCKER-USER` -> `STYRMAN-FWD` jump until the timer's first tick
  (`OnBootSec=5s`, `AccuracySec=5s`, so up to ~10 s), during which a sandbox project
  reaches the host, the LAN, other project bridges, `169.254.169.254` and the tailnet.
  The window is smaller than before (15 s -> 5 s) but it is the timer that shrank it,
  not the unit. Fix: add `run systemctl enable styrman-firewall.service` beside
  install.sh:110 and the equivalent in `deploy/update/styrman-update` around line 534,
  and assert it (`systemctl is-enabled styrman-firewall.service` = `enabled`) in the
  installer's closing verification; or delete the `[Install]` section and both
  comments that claim it works, leaving the timer as the sole mechanism honestly.

- **[medium / confidence: high]** `.agents/docs/testing/styrman/ensure-test-image.sh:15`
  with `.agents/docs/testing/styrman/integration.sh:20` and `web/e2e/run.sh:15` —
  **the ":dev is never touched" promise is not enforced; the tag is whatever argv says.**
  The header states "Never touches `:dev` — that is what production boots (deploy/.env)"
  and then, one line later, "the tag is always the caller's" — which is the hole.
  `TAG="${1:?...}"` and both suites pass their own `${IMAGE}`, which is `${1:-styrman-runtime:test}`.
  This repo documents `integration.sh styrman-runtime:dev` as a normal invocation
  (`.agents/docs/testing/2026-09-03-e2e-harness-upgrade-path.md:9` "Golden path —
  `integration.sh styrman-runtime:dev` on real Kata";
  `.agents/docs/testing/2026-09-01-verify-editor-pane-and-ordering.md:89`). Running it
  now rebuilds `:dev` with `--build-arg STYRMAN_VERSION=test-<epoch>` and retags it,
  re-stamping the image the production compose deploy boots — the version arg keys
  harness re-seeding, and ADR-053's no-mutable-tags/rollback guarantee is that `:dev`
  maps to the built release. `web/e2e/run.sh:9-12` records this as a closed 2026-09-01
  audit finding ("the integration tier's prereq used to REBUILD it, re-stamping what
  production boots"); this reintroduces it. Fix: refuse the tag rather than trust the
  caller — `case "${TAG}" in *:dev|*:latest) echo "refusing to rebuild ${TAG}" >&2; exit 1;; esac`
  at the top of `ensure-test-image.sh`.

- **[low / confidence: medium]** `deploy/firewall/styrman-firewall.service:6` —
  `After=docker.service` with no `Wants=` orders **only if docker.service is already in
  the boot transaction.** The unit correctly cannot start dockerd (`After=` alone never
  pulls a unit in — that part of the design holds). But on a host where Docker is
  socket-activated (`docker.socket` enabled, `docker.service` not), `After=` is a no-op,
  and once finding 1 is fixed the unit will run before dockerd creates `DOCKER-USER`.
  `ensure_jump DOCKER-USER sdb-+ STYRMAN-FWD` (`styrman-firewall.sh:79`) then fails —
  `iptables -I` never creates a chain, so nothing worse happens than a failure — and
  `set -eu` aborts the run at that line, leaving `STYRMAN-IN` and its INPUT jump
  installed (lines 60-62) but no forward filtering, and skipping the IPv6 mirror
  (92-111) and the ADR-065/066 stale-allowance cleanup (128-153) entirely. That is a
  partial application, not a fail-open into a wrong chain, and the timer heals it
  within 60 s. Fix: `Wants=docker.socket` plus `After=docker.socket docker.service`, or
  make the `DOCKER-USER` jump a `warn` rather than an errexit abort so the rest of the
  script still runs and the unit reports the condition.

- **[low / confidence: high]** `internal/httpapi/check.go:410` and
  `internal/httpapi/format.go:121` — **editor content that survives a crash is now
  retained where nothing can reap it and the user cannot delete it.** Trust boundary
  first, since it is the answer to the redirect question: the script runs INSIDE the
  guest as the project user, over stdin, via `orchestrator.Exec`. `$HOME`, `~/.styrman`
  and `~/.styrman/tmp` all belong to that user, and the destination is either the
  project home bind (`/var/lib/styrman/homes/<u>/p/<uuid>`) or the workspace bind
  (`/var/lib/styrman/projects/<u>/<uuid>`) — both already fully writable by the same
  principal, and one project is one home is one user (ADR-031). A symlink planted at
  `~/.styrman/tmp` therefore redirects the write only to somewhere the guest already
  owns; it grants nothing, and the server never reads the file back by host path (it
  reads the exec's stdout). No escalation. The *new* problem is retention: `~/.styrman`
  is refused by `managedHomeTarget` (`internal/httpapi/explorer.go:98-112`), so the user
  cannot delete anything under it through the file API; and the transcript janitor walks
  `.styrman/logs` only (`cmd/styrman/main.go:350`), so `.styrman/tmp` is never pruned.
  The `trap ... EXIT` does not run on SIGKILL (container stop, exec teardown, OOM), so a
  full copy of the editor buffer — a `.env`, a key, whatever was open — persists
  indefinitely, invisible to the janitor and undeletable from the UI, where the old
  workspace-root temp was at least visible in `git status` and the file explorer.
  Fix: prune `.styrman/tmp` by mtime in `pruneTranscripts`, or have the guest script
  clear files older than an hour from `"$D"` before creating a new one.
  (Checked and clean: the trap edits are correct in both directions — `checkScript`
  creates only `${F}.out` and now traps exactly that; `formatScript` creates only
  `${F}.err` and now traps exactly that. Neither leaks a file the trap forgot.)

- **[low / confidence: medium]** `deploy/host-setup.sh:122-131` — the pool is pinned to
  `172.16.0.0/12` size 24 **with no check that it does not overlap the host's own
  routes**, and it now contains the /16s docker0 and the ingress network already hold.
  Measured on this host: `ip -4 route` shows `172.17.0.0/16 docker0`,
  `172.18.0.0/16 br-02a045873a1d` (styrman-ingress), `172.19.0.0/16 sdb-e64fe297000`,
  LAN `192.168.5.0/24` — so the change is safe here and its stated rationale is right.
  But no `bip`/`fixed-cidr` carves those /16s out of the pool, so on a host where they
  are recreated (fresh install, or `docker network prune` then restart) a pool /24 can
  be allocated inside a prefix an existing interface already routes; the more-specific
  /24 then shadows the /16 in the host routing table. Isolation still holds — Docker's
  per-bridge `! -i <br> -o <br> -j DROP` and the styrman rules match on interface and
  destination, not on route — so this is a routing/availability hazard, not a bypass.
  Separately, an operator whose LAN *is* inside 172.16/12 now gets a project bridge on
  their LAN's subnet. Fix: in the same python block, refuse or warn when the chosen base
  overlaps any prefix in `ip -4 route`, and consider a narrower base (e.g.
  `172.24.0.0/13`) that excludes the existing docker /16s.

- **[low / confidence: high]** `deploy/caddy/Dockerfile:12-16` — **xcaddy honours the
  ingress `go.mod` only as an MVS floor, so the govulncheck run on that module is a
  proxy for the shipped binary, not a measurement of it.** `xcaddy build --with
  github.com/spgo/devbox/caddy/ingress=/src/caddy-ingress` generates its own main module
  and resolves versions across caddy's requirements and every plugin's. MVS takes the
  maximum, so this bump cannot be *undone* by the build (that part is safe, and answers
  the question asked) — but the internet-facing binary may carry versions neither
  `go.mod` lists, and the reachability of the two open `docker/docker` findings in it
  was not measured. Fix: add `govulncheck -mode=binary /usr/bin/caddy` (or at minimum
  `go version -m /usr/bin/caddy`) as a build stage or a `deploy/vulncheck.sh` step, so
  the security row is measured on the artifact that serves traffic.

- **[low / confidence: high]** `.agents/docs/testing/styrman/ensure-test-image.sh` is
  **untracked** (`git ls-files --error-unmatch` -> "Did you forget to 'git add'?") while
  two committed suites now hard-depend on it (`integration.sh:20`, `web/e2e/run.sh:15`).
  Any checkout of the commit — including the detached worktree
  `deploy/verify-commit.sh` builds — gets suites that abort on
  `bash: .../ensure-test-image.sh: No such file`. Commit it together with its two callers.

- **[low / confidence: high]** `.agents/docs/testing/styrman/ensure-test-image.sh:39` —
  the build log is redirected to `${ROOT}/.agents/scratch/ensure-test-image.log`. That
  directory is gitignored, so in a fresh clone or a `verify-commit.sh` worktree it does
  not exist, the redirect fails before `docker build` runs, and the script prints
  `FATAL: ... docker build of <tag> failed — see .agents/scratch/ensure-test-image.log`
  naming a log that was never created. A misdiagnosis of a build that never started, in
  a FATAL path. Fix: `mkdir -p "${ROOT}/.agents/scratch"` before the redirect.

- **[low / confidence: low]** `STYRMAN_TEST_IMAGE_NO_REBUILD=1` — checked as a gate
  bypass and it is not one: `deploy/verify-commit.sh` invokes neither `integration.sh`
  nor `web/e2e/run.sh` (grep found no reference), so no release gate depends on the
  rebuild, and the skip echoes a line naming itself. Residual: it is read from the
  ambient environment and appears nowhere a verifier's log preserves, so a value
  `export`ed during a fast-iteration session silently makes a later hand-run suite test
  a stale image and report green — the same discipline hazard as `SKIP_WEB_FRESHNESS`,
  which at least AGENTS.md names. Consider having the suites print the flag's state into
  their own header line so it lands in the verification log.

## Observations (not introduced by this diff)

- `deploy/firewall/styrman-firewall.sh:69-78` — the pool change makes the
  `-m physdev --physdev-is-bridged -j RETURN` exemption load-bearing for **every**
  project rather than incidentally: with all bridges guaranteed inside `172.16.0.0/12`,
  that RETURN is the only thing keeping the credential broker (`:8091`) and the
  port-preview proxy reachable on the project's own bridge. `ensure_rule` **appends**,
  and the physdev rule's own failure path (line 70) only echoes a WARN and lets the
  script continue — so on a host where `STYRMAN-FWD` was first built while `xt_physdev`
  was unavailable, a later successful run appends the RETURN *after* the DROPs, where it
  can never match. That fails closed (the broker breaks, credentials stop flowing), not
  open, and the script has no ordering assertion. Pre-existing; worth an ordering check
  now that it applies to 100% of projects.
- `deploy/update/styrman-update.timer` — removing `Persistent=true` is a no-op:
  systemd.timer(5) states `Persistent=` "only has an effect on timers configured with
  `OnCalendar=`", and this timer is monotonic (`OnBootSec`/`OnUnitActiveSec`). No delay
  to security updates, and it stops systemd keeping a stamp file it never consulted.
- `internal/httpapi/check.go:123,197` — the `gcc` -> `tofu` tool-name fix corrects a
  mislabel only; `parseCheckOutput` routes `tofu` to the same `reGCC` line parser that
  already handled the output, and no new format is trusted.

## Answers to the questions asked

1. **Dependency bump.** No new called vulnerability. Authoritative scan
   (`golang:1.25.13-trixie` per `deploy/Dockerfile.server`, govulncheck v1.7.0,
   `GOTOOLCHAIN=local`, matching `deploy/vulncheck.sh`) -> **exit 3, exactly two called**:
   GO-2026-4887 and GO-2026-4883, both `github.com/docker/docker@v28.5.2+incompatible`,
   `Fixed in: N/A` — the accepted-per-release daemon-side findings named in
   `deploy/vulncheck.sh:11-14`, and `docker/docker` is at the identical version in the
   root module, unchanged by this bump. All eight bumps are indirect and **every one
   matches the root module's selected version exactly**: go-connections v0.8.1,
   httpsnoop v1.1.0, logr v1.4.4, otelhttp v0.71.0, otel + metric + trace v1.46.0,
   x/sys v0.47.0. `go.yaml.in/yaml/v3 v3.0.5` is ingress-only (absent from the root
   module), so it is *not* covered by the root module's vetting — no called finding
   against it. `go mod verify` -> "all modules verified"; the go.sum delta is 24 added /
   37 removed, consistent with a tidy, `stretchr/testify v1.12.1` appearing as a
   transitive test-dependency sum only (not a `require` in the ingress go.mod).
   A **host-toolchain** run (`go1.25.0`, resolving go1.25.1) reports 29 — all stdlib —
   which is precisely the false surface `deploy/vulncheck.sh` exists to avoid; do not
   use it for the security row.
2. **Guest temp path.** Trust boundary and no-escalation argument stated in the finding
   above; the residual is retention, not disclosure.
3. **Address pool.** No isolation regression, and the drop list already covered both
   the old and the new pool. Sandbox forward path:
   `DOCKER-USER -i sdb-+ -j STYRMAN-FWD` -> physdev-is-bridged RETURN (own bridge L2
   only) -> `-d 172.16.0.0/12 -j DROP`, which covers every other project bridge, docker0
   and styrman-ingress (all three verified inside 172.16/12 on this host). Host path:
   `INPUT -i sdb-+ -j STYRMAN-IN` -> DROP, covering the host's gateway address on the
   project's own bridge. So a sandbox project reaches neither. LAN (`lan-`) bridges are
   matched by neither jump — unchanged from ADR-067's third amendment, which already
   records that a LAN project reaches the host at every address it holds including other
   bridges' gateway addresses; the pool change neither widens nor narrows that. Existing
   networks keep their subnets (confirmed: docker0 172.17/16, ingress 172.18/16, the one
   live project 172.19/16), so the change affects only networks created after the
   restart. Coverage was already complete before the change — Docker's defaults
   (172.17-172.31/16, then 192.168.0.0/16) are both in the drop list — so this is about
   LAN *collision*, as the comment says, not about drop coverage.
4. **Systemd units.** Cannot start dockerd: `After=` without `Wants=`/`Requires=` is
   ordering only. Does not widen anything ADR-067 closed: no rule, chain, jump or
   matching prefix changed, and the `-i sdb-+` keying is untouched. The DOCKER-USER race
   is real but fails partial-closed (finding above) — `iptables -I` cannot create the
   chain, so there is no path where styrman creates an empty `DOCKER-USER` that Docker
   then leaves without its own RETURN. `OnBootSec` 15 s -> 5 s is a strict improvement
   and, given finding 1, is the *only* improvement this change actually delivers.
5. **ensure-test-image.sh.** It *can* retag `:dev` — finding above. The
   `NO_REBUILD` escape hatch gates nothing a release depends on — finding above.

## Checked
- **Secrets** — none added. No credential, token or key in the diff; no secret echoed
  into a log or an error message. `deploy/vulncheck.sh`'s `/dev/null` shadow of
  `deploy/.env` is untouched and still applies to the scans run here.
- **Injection** — clean. `ensure-test-image.sh` interpolates `$TAG` from argv into
  `docker image inspect` / `docker build -t`, fully quoted, no `eval`, no word
  splitting — a hostile tag reaches docker as one argv element; `date -d "${created}"`
  consumes docker's own output. The two guest scripts interpolate only `suffix`/`ext`
  and `cmd` from the closed switch in `checkerFor`/the formatter table (never the
  request body) and an integer `n`; the request body reaches the guest solely over
  stdin under an exact byte count. `$HOME` is guest-controlled but the script already
  executes as the guest.
- **Authz/authn** — untouched by this range. No route, middleware, guard, ingress-token
  or `X-Caddy-Email` handling changed.
- **Crypto** — nothing hand-rolled, nothing added; no crypto surface in the diff.
- **Dependencies** — pinned-image `govulncheck ./...` on `deploy/caddy/ingress` run and
  reported above (exit 3, two accepted `docker/docker` findings, no fix available).
  `go mod verify` clean. No new direct dependency; all eight bumps are indirect and
  version-aligned with the root module bar `go.yaml.in/yaml/v3`. `npm audit` not run:
  this range touches no JavaScript dependency (`web/package.json` is modified in the
  working tree but is not part of `phase3.diff`).
- **GCP surface** — not applicable. This is a self-hosted deployment
  (`.agents/docs/deploy/gcp.md` says so); the diff contains no IAM binding, service
  account, Cloud Run flag, Secret Manager reference or service-account key path.

## Not examined
- The working tree carries changes **outside** the stated range that were not reviewed:
  `internal/config/config.go`, `internal/httpapi/server.go`, `internal/terminal/bridge.go`,
  `web/e2e/run.mjs`, `web/package.json`, `web/src/components/{CodeEditor,EditorPane}.tsx`
  and the regenerated `web/dist/client/*` bundle (Phase 1/2 work). `server.go` (+59/-x)
  and `bridge.go` (+69/-x) are the two that most warrant their own pass.
- Findings 1 and 3 are read from `install.sh`, the unit files and the updater, plus one
  live `systemctl is-enabled` — **not** measured by rebooting a host and observing the
  ordering, and not measured by running the suites with a `:dev` argument (which would
  have caused the very damage described).
- The `image/harness` module's govulncheck was not re-run (unchanged by this range);
  the shipped caddy binary was not scanned in binary mode (see the xcaddy finding).
- The credential-broker test that ADR-067's third amendment records as **owed** (compose
  deploy, two users, four probes) is still owed and was not performed here.

## Response (main, 2026-09-10)

- **Finding 1 (medium) — fixed** in both places that install units:
  `deploy/firewall/install.sh` and the updater's `host_converge` now
  `systemctl enable styrman-firewall.service` beside the timer, so the
  `[Install]` section takes effect on the operator's next `install.sh`
  run and on every converge after that. Until then the 5 s timer is what
  narrows the window; the changes entry says so.
- **Finding 2 (medium) — fixed.** `ensure-test-image.sh` refuses
  `styrman-runtime:dev`, any `*:latest`, and whatever `deploy/.env` names
  as `STYRMAN_IMAGE` (measured: `styrman-runtime:dev` → FATAL, exit 1;
  `styrman-runtime:test` → current).
- **Finding 3 (low) — mitigated.** Both guest scripts sweep files older
  than an hour from `~/.styrman/tmp` before creating a new one, so a
  SIGKILLed exec's buffer does not persist beyond the next check or
  format; the dir is the guest's own, unreachable across tenants.
