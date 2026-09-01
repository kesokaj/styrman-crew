# Verification — PREFLIGHT release gate, v1.15.0..HEAD (6b41979)   (2026-09-01)

## Status: PASS

Scope: `git diff v1.15.0..HEAD` — one commit (`6b419799`), 85 files,
+5285/-478 ("feat: editor pane, syntax checking, tag-gated updater; fix
/format data loss"). Full local tier, no remote/GCP tiers used (none
requested).

**Concurrency note (read this first):** this repo's working tree is shared
with other agents today. Mid-run, `git status` showed uncommitted WIP on
`deploy/host-setup.sh` and `deploy/update/styrman-update` (a security
remediation in progress — see the newly-appeared, untracked
`.agents/docs/reviews/2026-09-01-security-preflight-v1.15.1.md` and
`.agents/docs/reviews/2026-09-01-preflight-release-gate.md`, neither written
by me) plus the expected `web/dist/client/_shell.html` e2e-rebuild churn.
Since this gate's job is to verify **HEAD, not someone else's WIP**, I did
not touch the working tree; instead I re-ran shellcheck against the actual
committed blobs (`git show HEAD:<path>`) rather than the on-disk copies. `go
build`/`vet`/`test`/`gofmt`, `tsc`, `oxlint` all ran against files the WIP
never touched (`git diff --stat -- '*.go' '*.ts' '*.tsx'` was empty
throughout), so those results stand unaffected. I left the other agent's WIP
and review docs untouched and did not commit anything.

## Commands run

1. `go build ./...` → exit 0, no output.
2. `go vet ./...` → exit 0, no output.
3. `go test -race -count=1 ./...` → all packages `ok` (cmd/styrman,
   internal/backup, config, conversation, credbroker, credstore, files,
   gitrepo, history, httpapi, links, orchestrator, project, terminal,
   userstore; `cmd/wsprobe`, `internal/identity`, `web` report "no test
   files"). Zero failures.
4. `gofmt -l internal/ cmd/` → empty output (clean).
5. `(cd deploy/caddy/ingress && go test -count=1 ./...)` → `ok
   github.com/spgo/devbox/caddy/ingress` (separate module, confirmed not
   covered by the root `go test ./...` run above).
6. `(cd web && npx tsc -b)` → exit 0, no diagnostics.
7. `(cd web && npx oxlint)` → exit 0, no diagnostics.
8. `shellcheck -S error` against the **committed HEAD blobs** (`git show
   HEAD:<path>` piped to a scratch file, not the dirty working-tree copies)
   for `deploy/update/styrman-update`, `deploy/host-setup.sh`,
   `web/e2e/run.sh` → all three exit 0, no findings. (An earlier pass against
   the working-tree copies also returned exit 0 for all three, for what
   that's worth, but the committed-blob run is the one that actually answers
   the gate's question.)
9. `docker build -t styrman-runtime:dev --build-arg
   STYRMAN_VERSION=dev-$(date +%s) image/` (AMD64, native) → exit 0, built
   clean against the committed `image/Dockerfile`.
10. `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:dev` →
    **passed=57 failed=0 skipped=3** — matches the expected 57/0/3 exactly.
    Full check list PASS (create/boot, terminals incl. pause/close-all, WS
    I/O, transcripts, files incl. traversal guard, ports/preview (2 of the 3
    SKIPs are the documented host-firewall-blocks-host→bridge cases; the
    third SKIP is the matching public-link-serves case), git status/diff,
    settings, persistence across container recreate, UUID identity +
    rename, sticky-stop/boot=1 revive, wipe). One earlier run of this same
    command was killed by my own tool's 2-minute default timeout at the
    "stop + delete" section (not a script failure) and left one scratch
    project + state dir behind; I removed the leftover container
    (`styrman-53d54d9a-…`), its docker network, and its `/tmp/styrman-itest-*`
    dir before re-running clean to completion in the background.
11. `STYRMAN_E2E_PORT=18071 bash web/e2e/run.sh styrman-runtime:dev` (private
    port, run once, in the background to avoid my own tool timeout) →
    **passed=48 failed=0 skipped=2** — matches the expected 48/0/2 exactly
    (2 SKIPs are the documented host-firewall port/private-link-serves
    cases). Final gate: "no uncaught client errors" PASS (3 recoverable
    hydration warnings, noted as known-benign per ADR-045). Rewrote
    `web/dist/client/**` with fresh content-hashed asset names as documented
    in the testing README — left as build-output noise, not a stray edit.
12. `~/.agents/bin/trail-check.sh --check` → `RESULT: FRESH` ("newest
    changes/ entry (2026-09-01) covers HEAD (2026-09-01)", "changelog entry
    for today present").

## Release-gate specific checks

- **Server image + LABEL**: `docker build -f deploy/Dockerfile.server
  --build-arg STYRMAN_VERSION=1.16.0 -t styrman-server:verify-amd64 .` → exit
  0. `docker inspect --format '{{ index .Config.Labels "styrman.version" }}'
  styrman-server:verify-amd64` → `1.16.0`. Confirmed present and correctly
  templated from `ARG STYRMAN_VERSION` (`deploy/Dockerfile.server`, LABEL is
  the LAST layer per its own comment, so a version bump doesn't bust the
  build cache above it).
- **The two newest e2e gates are real, not decorative**:
  - "terminal keeps its width across the round-trip"
    (`web/e2e/run.mjs:232`) — measures the rendered `.xterm-screen` box width
    in pixels and fails under 400px; this is exactly the collapsed-terminal
    regression class the comment above it describes (ResizeObserver collapse
    on hide, debounced recovery). It ran and passed live in my run.
  - "syntax check leaves no temp files in the workspace"
    (`web/e2e/run.mjs:338`) — now opens a `.py` file (not just `.sh`) and
    asserts both the `tmp\.[A-Za-z0-9]+\.` regex AND a literal `__pycache__`
    absence (`run.mjs:358-361`). I re-derived the Python case directly rather
    than trusting the assertion: in a scratch container off
    `styrman-runtime:dev` (`docker run --rm --entrypoint bash
    styrman-runtime:dev`), compiling a `.py` file with plain `python3 -m
    py_compile` left a `__pycache__/` directory in the workspace; the same
    compile with `PYTHONPYCACHEPREFIX=/tmp/styrman-pyc` (the actual fix,
    `internal/httpapi/check.go:115`) left the workspace clean and redirected
    every `.pyc`, including the workspace file's own
    `lint.cpython-313.pyc`, under `/tmp/styrman-pyc/`. This is a genuine
    negative control confirming the mechanism, not just the test.

## Dual-arch image builds (the preflight-specific requirement)

**`image/` (styrman-runtime) — ARM64 is architecturally out of scope, and
this is documented, not just asserted by me:** `AGENTS.md:84` states "The
runtime image is AMD64-only (single NUC target)" and
`.agents/docs/specs/styrman/runtime-image.md:4` says "Debian trixie, AMD64
only." The Dockerfile itself hardcodes amd64/x86_64 artifacts: `yq_linux_amd64`,
`hadolint-Linux-x86_64` (checksum-pinned to that exact arch's binary), and
`go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz`.

I did not stop at citing the docs — I attempted the ARM64 build for real,
twice, with an important nuance worth recording plainly because it is
counter-intuitive:
- **Without QEMU emulation registered** (this host's baseline state),
  `docker buildx build --platform linux/arm64 image/` fails immediately at
  the very first `apt-get` `RUN` with `exec format error` — it cannot even
  install packages, let alone reach the amd64-specific fetches later in the
  file.
- **After registering QEMU emulation** (`docker run --rm --privileged
  tonistiigi/binfmt --install arm64`, temporary — uninstalled again after,
  see Cleanup), the full `docker buildx build --platform linux/arm64
  --build-arg STYRMAN_VERSION=dev-arm64test image/` **completes successfully
  (exit 0)**. This is NOT evidence that the image is ARM64-portable: the
  `RUN` steps that fetch `yq`/`hadolint`/`go` only `curl` + `chmod` those
  binaries into the filesystem, they never execute them during the build, so
  an amd64 binary silently lands in an "arm64" image layer without the build
  ever noticing. I confirmed this directly: `docker run --rm --entrypoint
  bash styrman-runtime:arm64test -c 'go version'` printed `go1.27.0
  linux/amd64`, and `yq --version`/`hadolint --version` also ran — but only
  because **this test host is itself amd64**, so those amd64 binaries execute
  natively even inside a container declared `--platform linux/arm64`; the
  QEMU layer here only emulates genuinely-arm64 code (bash, apt, dpkg), and
  never gets a chance to reject the smuggled amd64 binaries the way real
  ARM64 silicon would. On real ARM64 hardware, `go`/`yq`/`hadolint` inside
  this image would fail with `exec format error` at first use. **Bottom
  line: the build succeeding is an artifact of testing amd64-on-amd64 through
  an incomplete emulation layer, not proof of ARM64 compatibility — the
  image is genuinely AMD64-only as documented, and a green `buildx build`
  exit code for this particular Dockerfile must not be read as a
  green ARM64 signal.**

**`deploy/Dockerfile.server` — genuinely portable, and I confirmed it, not
just inferred it.** The Dockerfile has no arch-specific pins: `golang:1.25-trixie`
and `debian:trixie-slim` are both official multi-arch images, `go build`
picks up the platform's native `GOOS`/`GOARCH` from the builder stage, and
the only other package installs are plain `apt-get` (`ca-certificates curl
git`, all multi-arch). `docker buildx build --platform linux/arm64 -f
deploy/Dockerfile.server --build-arg STYRMAN_VERSION=1.16.0 .` (with QEMU
registered) completed in ~1–2 min and produced a real arm64 image:
`docker inspect --format '{{.Architecture}}' styrman-server:verify-arm64` →
`arm64` (vs `amd64` for the native build), and the embedded `styrman.version`
LABEL was still `1.16.0`. Unlike the runtime image, here the go build stage
itself compiles for the target arch (`go build` inside an emulated-arm64
`golang` container), so this is a real cross-arch compile, not a smuggled
native binary — a meaningfully different, and honest, signal from the
runtime-image result above. AMD64 native build also succeeded
independently (`styrman-server:verify-amd64`).

## Coverage of changed code

- `internal/httpapi/check.go` (+414) / `check_test.go` (+248) — covered.
  `TestParseCheckOutput`/`TestParseCheckOutputClean`, `TestCheckerFor`,
  `TestShellDialectFlag`, `TestMissingTool`, `TestSafeSuffixBlocksInjection`,
  `TestGuestScriptsNeverWaitForStdinEOF` exercise every pure-logic seam.
  `go test -cover ./internal/httpapi/...` → 32.0% of statements (package-wide;
  the HTTP-handler surface needs Docker and is exercised by
  `integration.sh` + `web/e2e` instead, both of which ran clean above and
  both of which hit `checkFile`/`formatFile` live). The Go-handler-level
  test gap for `checkFile`/`formatFile` is a **pre-existing, already-tracked**
  item (`TODO.md:30-31`), not a new regression introduced by this diff — I
  am not failing the gate over it, consistent with the three prior verifier
  passes on this same feature set, but it remains the honest caveat: a
  handler-only bug that unit tests can't see would only be caught by the
  Docker-backed suites, not by `go test ./...` alone.
- `internal/httpapi/format.go` (+21/-?), `internal/orchestrator/exec.go`
  (+29/-?) — covered by `TestGuestScriptsNeverWaitForStdinEOF` plus live
  `/format` exercise in `integration.sh`/e2e. The original bug this commit
  fixes ("/format data loss") was a transport-layer stdin/half-close issue,
  not a pure-logic one — unit tests structurally cannot catch that class,
  which is exactly why the Docker-backed suites matter here and both passed.
- `internal/conversation/store.go` (+68) / `store_test.go` (+106) —
  covered; `go test -cover ./internal/conversation/...` → 63.3%.
- `deploy/update/styrman-update` (+267 new file), `.service`/`.timer`
  (new) — **zero automated test coverage**, matching this repo's existing,
  accepted convention (no shell-test harness anywhere in this codebase;
  `deploy/host-setup.sh` and `deploy/backup/styrman-host-backup` are
  likewise shellcheck-only). `shellcheck -S error` is clean on the committed
  blob. This is root-privileged, security-sensitive code; a dedicated
  security review already ran against it today
  (`.agents/docs/reviews/2026-09-01-security-preflight-v1.15.1.md`, not
  authored by me) and found three findings that block *enabling* the
  updater timer but none that block *tagging* the release — consistent with
  this task's constraint that I not install/enable the updater units. I did
  not re-litigate that review; it is the security owner's row, not mine.
- `deploy/Dockerfile.server`, `image/Dockerfile` — no unit-test concept
  applies; verified by real builds (four total: server AMD64, server ARM64,
  runtime AMD64, runtime ARM64-with-caveats) plus the LABEL inspection
  above.
- `web/src/components/{EditorPane,FileEditor,CodeEditor,FileExplorer,
  TerminalPane}.tsx`, `web/src/lib/editorPane.ts`, `web/src/routes/
  p.$project.$term.tsx` — this repo has no component/unit test runner for
  the frontend (`web/package.json` scripts: `dev`, `build`, `lint` — no
  `vitest`/`jest`; confirmed by grep). `tsc -b` and `oxlint` both pass
  clean, and the new editor-pane/syntax-check UI is exercised end-to-end by
  10 of the 48 passing Playwright checks (editor pane open/content/toggle,
  terminal-survives-round-trip, width-after-round-trip, reopen-same-file,
  word-wrap, Dockerfile highlighting, discard-and-open, delete-open-file,
  syntax-check flag/clean/no-litter). This is the same class of coverage
  the codebase already relies on for all its other UI surfaces (no gap
  specific to this diff).

### Test that would pass even if the feature were broken

Checked for this specifically, per task instruction. Did not find a new one
beyond what the prior verifier round already surfaced and fixed (the old
`.sh`-only litter check that couldn't see `__pycache__` — now fixed and
negative-controlled above). No other check in this diff's scope looked
structurally incapable of catching its own regression.

## Environment

Tier: local, on the actual target host (bare-metal Debian, per R9;
`uname -a`: Linux 6.12.101+deb13-amd64). `docker version`: Engine 29.7.2,
buildx v0.36.1. Go 1.25.0 linux/amd64. Node v20.19.2, npm 9.2.0. shellcheck
0.10.0.

## Cleanup performed

- Removed the leftover scratch project/container/network/state-dir from the
  integration-test run that my own tool's 2-minute timeout interrupted
  mid-script (`styrman-53d54d9a-…` container, its `sdb-…` network,
  `/tmp/styrman-itest-Wysh`); re-ran clean to completion afterward.
- Removed all scratch verification images: `styrman-runtime:arm64test`,
  `styrman-server:verify-amd64`, `styrman-server:verify-arm64`.
- Uninstalled the QEMU arm64 binfmt registration I added for the emulated
  builds (`docker run --rm --privileged tonistiigi/binfmt --uninstall
  arm64`), restoring the host to its pre-existing amd64-only buildx platform
  set.
- Removed `/tmp/styrman-pyc` and `/tmp/styrman-shots` scratch dirs.
- Did not touch, start, stop, or exec into any operator project.
  `docker ps -a` before and after shows the same five pre-existing
  containers (`styrman-982ffddc…`, `styrman-36782cc5…`, `styrman-server`,
  `styrman-928e709a…`, `styrman-caddy`) untouched throughout.
- Did not install or enable any `styrman-update` systemd unit, did not
  create `/opt/styrman`.
- Left the other agent's in-progress WIP (`deploy/host-setup.sh`,
  `deploy/update/styrman-update` working-tree diffs) and their two new
  review docs untouched — not mine to resolve or commit.
- `web/dist/client/**` churn from the `web/e2e/run.sh` run is expected
  build-output noise per the testing README convention — left as-is.
