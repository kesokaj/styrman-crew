---
owner: changelog
date: 2026-09-13
---
# Whole-project audit and high/medium fixes (v1.30.0)

## Summary

A 31-agent workflow audited every Go package, the web app in three slices, deploy/image scripts, the Caddy ingress, and five security surfaces. It confirmed **2 high and 14 medium findings** and listed 38 lows. **One high is live in production** (no brokered credentials in login shells on /24 bridges, since 2026-09-10). **One high user-facing** (20 s timeout clips 150–210 s project start/stop/LAN operations). All confirmed defects are fixed; every medium is either resolved or explicitly recorded. High/medium/selected-low fixes land in v1.30.0; remaining lows are recorded in TODO.md or deferred by explicit decision.

## Type

task

## Files changed

**High priority:**
- `image/configs/styrman-credentials.sh` — deleted `/16` broadcast-1 recomputation; injected `STYRMAN_CRED_BROKER` is authoritative. Broker unreachable marker in MOTD.
- `web/src/lib/api.ts` — request() timeout does not override caller signal; `checkFile` binds its own 20 s timeout.
- `internal/orchestrator/orchestrator.go` — `recreateMissingNetwork` recreates the container (not reconnects) when a network vanishes; RUN correctly names the always-fresh broker env.

**Medium priority:**
- `web/src/components/SidePanelGit.tsx` — Discard/pop call `treeChanged()` in finally; stale editor buffer closed.
- `internal/httpapi/format.go` — format temp file beside the edited file (not in `~/.styrman/tmp`); config discovery restored. Handler validates `STYRMAN_FMT_DIR` confinement.
- `internal/config/config.go` — `STYRMAN_DOCKER_DISK_GB` validated at load, fail-closed like other env checks.
- `image/Dockerfile` — Go 1.27.1 + sha256, Node 22.23.2 + sha256, claude-code 2.1.270 without `--ignore-scripts`, agy floats by design (no upstream versioning). Comment and base-image digests corrected.
- `deploy/update/styrman-update` — signed-tag checker accepts all four SSH/PGP signature markers (was 2/4); GNUPGHOME cleanup on all paths.
- `deploy/caddy/ingress/generator.go` — trust suffix dotted (`.example.com` not `example.com`), public-block cookie `samesite lax`.
- `internal/terminal/bridge.go` — origin check FIRST statement in ServeHTTP and ServeSetup (not inside accept).
- `internal/httpapi/{projectenv,stats}_test.go` — handler tests through guest seam; zero-test-coverage handlers now covered.
- `internal/credbroker/broker.go` / ADR-030 amendment — workloads inside a project's dockerd share the project's identity at the broker (design record, no code change).

**Selected lows:**
- `internal/files/files.go` — `ContainedPath` deleted (deadcode); Read error handling accepted FAIL line (mid-I/O fault cannot be planted through `os.Root` seam).
- `internal/orchestrator/orchestrator.go` — workspace cleanup on creation failure; one helper for bridge-mode-mismatch logic.
- `internal/project/uuid.go` — NewUUID returns error to callers; createProject 500 "cannot generate an id", load-time migration quarantines.
- `internal/history/history.go` — Lstat + IsRegular parity before FIFO open.
- `internal/credstore/credstore.go` — chmod 0700 always (not on create only); non-NotFound errors logged.
- `internal/credbroker/broker.go` — non-NotFound `GetGit` errors logged.
- `internal/config/config.go` — parse errors logged (WARN); deleted comment-only credstore path.
- `internal/httpapi/browser.go` — relay pumps joined before slot released.
- `deploy/host-setup.sh` — `/etc/styrman` install guarded.
- `deploy/styrman-ctl` — key validation `^[A-Z][A-Z0-9_]*$`; handling of env assignment keys.
- `deploy/caddy/ingress/labels.go` — sanitizeHostname refuses empty names.
- `web/src/components/` — BrowserPane `attached` field deleted; icons unused icon deleted; settings orphan JSDoc deleted; portUrl through `u` tag.
- `web/src/lib/diffReveal.test.ts` — test moved beside DiffView.
- `web/src/lib/clips.ts` — documented invariants + vitest.
- `web/dist/` — zero-width Trojan source family added to DiffView.
- `web/.claude/agent-memory/` — untracked (`git rm -r --cached`, .gitignore).
- `.agents/docs/specs/styrman/api.md` — ADR-067 network section rewritten (lanAccess, lanMessage, firewall deprecated).

New test files: `internal/credbroker/snippet_test.go`, `internal/config/styrman_update_test.go`, `web/src/lib/api.timeout.test.ts`, `web/src/lib/clips.test.ts`.

## Agents dispatched

Audit workflow (v1): 11 reviewer slices (Go packages, web tiers, deploy, ingress), 5 security surfaces, 1 explorer inventory, 1 verifier, 1+ refuters per finding (13 in workflow + 5 by hand for scan stubs) — 31 agents total. Round 2–4 agents: reviewer ×3 (cold-read fix plan), security ×5 (re-runs on deleted ADR-065/066 machinery and new ADR-067), verifier ×2 (image rebuild, final gate), documentation (api.md and runtime-image.md rewrites), bugs (BUG-054 and BUG-055 records).

## Verification

**Locally passed (2026-09-13):**
- Image rebuilt: `docker build image/` green; `go version`/`node --version`/`claude --version`/`agy --version` answer in-image (`.agents/scratch/image-rebuild-audit.log`).
- `go test -race ./...` all packages (new: snippet pin + marker run, orchestrator recreate-not-reconnect, config bound, updater fixtures, terminal origin, projectenv/stats handlers, format script/dir). `files.Read`'s I/O-fault branch has NO test — an accepted FAIL line (no seam through os.Root), stated in the plan and the verifier log, not hidden.
- Ingress module `go test ./...` green.
- `cd web && npm run check && npm run lint && npm test` — tsc clean, oxlint clean, vitest 177/177.
- `bash deploy/vulncheck.sh` — run by the AUDIT's verifier earlier today (`.agents/docs/testing/2026-09-13-audit-verify.md`): exit 3, the two accepted docker/docker advisories; not re-run for this batch (no dependency changed).
- `bash -n` and `shellcheck -S warning` on every touched script — clean.

**In progress (release gate, will record in handoff):**
- Verifier: integration suite, e2e checks (v1.30.0 built image).
- Reviewer (final): Phase 4 docs/TODOs/release.
- Security (final): re-run on the closed ADR-065/066 machinery, new ADR-067.
- Live verification (post-deploy): new project login shell shows `GH_TOKEN` set (credentials now reachable via broker; operator-run only if broker must prove it on the live host).

## Requirements

- R1 (startup latency) — unchanged, no new operations at boot.
- R3 (shared credentials, per-project homes) — credentials now reach login shells again (fix for BUG-054); per-project home isolation preserved.
- R12 (cross-project network isolation) — unchanged; sandbox/LAN enforcement by bridge name (ADR-067, deletes root machinery from prior releases).
- R13 (provisioning fallback) — unchanged; clone failure still falls back to `git init`.

## Notes

**ADRs written by this entry:**

**ADR-069: Runtime image supply chain and broker address contract** — Go/Node/claude-code pinned at measured versions + sha256; Antigravity floats by design (no upstream versioning). Bump procedure documented. Broker-address contract: `STYRMAN_CRED_BROKER` injected by server at container create, authoritative in guest (never re-derive `/16` arithmetic). Network cannot change under RUNNING container; a STOPPED container whose network vanished is recreated with fresh env. Production proof: login shell receives credentials on new projects.

**ADR-030 amendment: workloads inside a project's dockerd share project identity at broker** — broker authenticates by bridge IP; every container a user runs inside their project reaches it with the guest's address and receives that user's credential set. Boundary is the project (R3). No code change; policy clarification.

**Cold-read trail:** three rounds of plan review (fixes 2026-09-13, rounds 2–3 2026-09-13) closed gaps in H1's orchestrator logic, M8's verb gating, M11's entry-point checks. Five findings blocked, each with one answer and no re-opened mechanisms.

**Accepted FAIL lines (verifier Definition of Done §1):** files.Read mid-I/O error branch (no testable seam without a mock Docker client). Closure is a durable trade: the branch is live code but untested in the gated suite. Regression would appear in production logs (500 + truncated file). Integration/e2e runs will measure format temp-file discovery and login-shell credentials.

---

Source: `.agents/docs/reviews/2026-09-13-audit-project.md`, `.agents/docs/plans/2026-09-13-audit-fixes.md` (Findings → fixes tables, cold-read sections, phase ticks), `.agents/docs/bugs/INDEX.md` (BUG-054, BUG-055).
