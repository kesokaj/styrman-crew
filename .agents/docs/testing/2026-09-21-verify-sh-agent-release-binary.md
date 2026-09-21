---
owner: verifier
date: 2026-09-21
---
# Verification — sh-agent release binary   (2026-09-21)

Range reviewed: working tree vs HEAD (uncommitted changes, `git diff --stat HEAD`) — harness mirror `image/harness/` and host harness channel deleted, `image/Dockerfile` downloads the sh-agent release binary at build via a build secret, entrypoint installs it, new updater step `retire_harness_units` with a Go test.

## Status: PASS

Scope: guide not written: `.agents/docs/testing/guide.md` is absent. Used `.agents/docs/testing/styrman/README.md` and `AGENTS.md` Setup Commands per dispatch instead. All 8 dispatch steps ran (step 7's Tier 2 read is the main agent's earlier run, recorded here per the coordinator's instruction, not re-run by this verifier). Scope is not "complete" per the strict definition (a guide.md was never written by this dispatch — writing it was out of scope for a verifier dispatch that names a specific log path and does not ask for the guide).

## Commands
1. `go build ./...` (root module) → exit 0, no output.
2. `go vet ./...` (root module) → exit 0, no output.
3. `go test -race -count=1 ./...` (root module) → exit 0. All packages `ok`: cmd/styrman, internal/config (5.9s), internal/conversation, internal/credbroker, internal/credstore, internal/files, internal/gitrepo, internal/history, internal/httpapi, internal/identity, internal/links, internal/orchestrator (11.1s), internal/project, internal/terminal, internal/userstore; cmd/wsprobe and web report `[no test files]`. No `-v` used so no PASS/FAIL counts printed. Log: `.agents/scratch/go-test-root.log`.
4. `(cd deploy/caddy/ingress && go test ./...)` (separate module) → exit 0: `ok github.com/spgo/devbox/caddy/ingress 2.023s`. Log: `.agents/scratch/go-test-caddy.log`.
5. `go test -count=1 -run 'TestStyrmanUpdateRetiresHarnessUnits|TestStyrmanCtl|TestStyrmanUpdate' ./internal/config/ -v` → exit 0. 18 subtests, all `--- PASS`, including `--- PASS: TestStyrmanUpdateRetiresHarnessUnits (0.03s)` and `--- PASS: TestStyrmanUpdateVerifyTagMarkers`. `ok github.com/kesokaj/styrman-crew/internal/config 4.060s`. counts: pass 18, fail 0, skip 0 at HEAD+dirty for this command. vs previous: none (first counts line for this exact command). Log: `.agents/scratch/go-test-config.log`.
6. Positive control on the PRE-change committed script: `STYRMAN_UPDATE_TEST=1 bash <(git show HEAD:deploy/update/styrman-update) __retire-harness-units /tmp/x /tmp/y` → exit 2, stderr `unknown flag: __retire-harness-units`. Confirms the flag/behaviour is new in the working-tree change. Log: `.agents/scratch/positive-control.log`.
7. `bash -n` on image/entrypoint.sh, deploy/update/styrman-update, deploy/styrman-ctl, deploy/host-setup.sh, deploy/verify-commit.sh, deploy/vulncheck.sh, .agents/docs/testing/styrman/integration.sh, .agents/docs/testing/styrman/ensure-test-image.sh → all exit 0, no syntax errors.
8. `shellcheck -S error` on the same 8 files → all exit 0 (no error-severity findings). Not surveyed at default (warning/info) severity — only error-severity was checked per the dispatch's "an error is [a FAIL]" wording.
9. Web Tier 1:
   - `(cd web && npm run check)` → exit 0 (`tsc -b`, no output).
   - `(cd web && npm run lint)` → exit 0 (`oxlint`, no output).
   - `(cd web && npm test)` → exit 0: `Test Files 15 passed (15)`, `Tests 178 passed (178)`.
10. Image build with secret: `GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN -t styrman-runtime:verify --build-arg STYRMAN_VERSION=verify image/` → exit 0. Build log shows the new `RUN --mount=type=secret,id=gh_token,required=true … gh release download "v1.6.0" --repo kesokaj/styrman-harness --pattern sh-agent-linux-amd64 …`, ssh-signature verification (`ssh-keygen -Y verify`) against the allowed signer, SHA256SUMS check, and an in-RUN self-check `test "$(/styrman/bin/sh-agent version)" = "sh-agent v1.6.0"` (this step executed, i.e. was not a cache hit for the secret-bearing layer on this run — later steps 9/12-14 were cached from a prior identical build).
    - `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:verify version` → exit 0, printed `sh-agent v1.6.0` — matches `image/Dockerfile`'s `ARG HARNESS_VERSION` default exactly.
    - `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:verify list` → exit 0, printed `### Skills (27)` and `### Subagents (7)`.
    - `docker rmi styrman-runtime:verify` → exit 0: `Untagged: styrman-runtime:verify`, `Deleted: sha256:56fe71e2254e…`. Image removed, no leftover tag.
11. Tier 2 integration (`bash .agents/docs/testing/styrman/integration.sh`) — run by the MAIN AGENT (not this verifier run) against this same working tree; per the coordinator's instruction this verifier read but did not re-run it. `.agents/scratch/tier2.log` tail shows individual `PASS` lines (BUG-062 undo, paused terminal, close-all, legacy-home migration, container naming, per-user metadata store, rename, duplicate-name refusal, stop/start/policy toggling incl. BUG-063 cases, reconnect/boot semantics, never-downgrade case incl. ADR-074 "left alone" boot and stamp-not-retried-every-boot, LAN re-enable, delete/netpolicy/workspace/API cleanup) and the summary line `passed=111 failed=0 skipped=3 aborted=0`. No `FAIL` or `aborted` lines present in the tail read. This count matches what the dispatch text stated the main agent's run printed.

## Root cause  (FAIL only)
N/A — no failures found in any step run or reviewed.

## Coverage of changed code
- `image/Dockerfile` (RUN downloading/verifying/installing sh-agent) — covered by step 10 (image build + `sh-agent version`/`list` inside the built image) and by Tier 2 (step 11), which boots the runtime image.
- `image/entrypoint.sh` (installs sh-agent, never-downgrade guard) — covered by Tier 2's never-downgrade case (manifest planted newer than image, stamp cleared, boot log says the newer home was left alone, home sh-agent/manifest untouched, stamp rewritten after the left-alone boot).
- `deploy/update/styrman-update` (`retire_harness_units` step) — covered by the new Go test `TestStyrmanUpdateRetiresHarnessUnits` (step 5, PASS) plus the positive control against the pre-change script (step 6) proving the test exercises genuinely new behaviour.
- `deploy/host-setup.sh`, `deploy/styrman-ctl install`, `deploy/verify-commit.sh`, `deploy/update/styrman-update deploy()` (build-secret plumbing lines) — covered only by `bash -n` (step 7) and shellcheck (step 8, error-severity only) plus, for `styrman-ctl`, the pre-existing `install --dry-run` plan test (part of `TestStyrmanCtlInstallPlan`/`TestStyrmanCtlInstallRefusals` in step 5's run, which passed) — these confirm the plan/dry-run text and syntax but do NOT exercise the actual `gh release download` + secret-mount path on any of these four call sites the way step 10 exercises it for the Dockerfile. This is a genuine, honestly-reported gap: no test actually invokes `host-setup.sh`'s, `styrman-ctl install`'s, `verify-commit.sh`'s, or `styrman-update deploy()`'s live build-secret code paths end-to-end.
- Deleted files (`image/harness/**`, `deploy/update/styrman-harness-update.{service,timer}`, `.agents/docs/testing/styrman/make-scratch-harness.sh`, `internal/orchestrator/harness.go`+`harness_test.go`, `internal/config/harnessdir_test.go`+`styrman_ctl_harness_test.go`+`styrman_harness_test.go`) — their removal is covered by: `go build ./...` and `go vet ./...` (step 1-2) finding no dangling references; `go test -race ./...` (step 3) passing with no reference to the deleted harness-mount/channel code; `internal/orchestrator/orchestrator.go` and `internal/config/config.go` changes covered by their packages' full test suites in step 3 (both `ok`).
- Doc-only changes (`.agents/docs/deploy/remote.md`, `specs/styrman/{deploy,runtime-image,versioning}.md`, `testing/styrman/README.md`, `AGENTS.md`, `README.md`, `ROADMAP.md`, `deploy/README.md`, `deploy/caddy/VENDORED.md`, `image/README.md`) — not code, not independently test-covered; not evaluated for content accuracy by this verifier (out of scope for a test-execution verifier; would be `documentation`/`/review --docs` territory).

## Not examined
- shellcheck at default (warning/info) severity — only `-S error` was run; style findings on the 8 shell scripts are unknown.
- `deploy/docker-compose.yml` change — not exercised (no compose-up smoke test in this dispatch's step list).
- Tier 2 was not re-run by this verifier; this log relies on the main agent's `.agents/scratch/tier2.log` for that result, as the coordinator instructed.
- Content accuracy of the doc-only changed files (listed above) — not reviewed.
- shellcheck / bash -n were not run on `deploy/docker-compose.yml`, `image/harness/*` (deleted, moot), or any files outside the 8 named in the dispatch.
- The end-to-end build-secret path for `host-setup.sh`, `styrman-ctl install`, `verify-commit.sh`, and `styrman-update deploy()` was not live-exercised (see Coverage section) — only syntax/dry-run checked.

## Environment
Go: go1.27.1 linux/amd64. shellcheck: 0.10.0. Node/npm: web deps pre-installed (`node_modules` present). Docker: available, build + run + rmi all succeeded natively (no explicit Colima check needed on this Linux host). gh: authenticated as kesokaj, used for `GH_TOKEN`. OS: Linux 6.12.107+deb13-amd64.
