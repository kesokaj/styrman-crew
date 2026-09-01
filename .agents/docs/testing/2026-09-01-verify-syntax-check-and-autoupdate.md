# Verification — syntax-check, exec stdin fix, tag-gated auto-updater (2026-09-01)

## Status: PASS

Second verification pass of this batch, after two rounds of review remediation
(`2026-09-01-review-syntax-check-and-autoupdate.md`,
`-rereview.md`) plus a security review
(`2026-09-01-security-editor-check-and-autoupdate.md`), both REQUEST
CHANGES / FINDINGS. Every finding those three documents raised as blocking
was independently re-derived here — not read and trusted — against the
current working tree (`deploy/update/styrman-update`, `internal/httpapi/check.go`,
`deploy/Dockerfile.server`, `image/Dockerfile`). All check out as genuinely
fixed. Two coverage gaps remain and are named below; neither is hidden and
one of them (the e2e litter check) is not yet recorded in `TODO.md` and
should be.

## Commands run (all independently re-executed, not taken from prior claims)

- `go build ./...` → clean, no output.
- `go vet ./...` → clean, no output.
- `go test -race -count=1 ./...` → all packages `ok` (backup, config,
  conversation, credbroker, credstore, files, gitrepo, history, httpapi,
  links, orchestrator, project, terminal, userstore, cmd/styrman); no
  failures.
- `gofmt -l internal/ cmd/` → no output (clean).
- `(cd deploy/caddy/ingress && go test ./...)` → `ok github.com/spgo/devbox/caddy/ingress`.
- `(cd web && npx tsc -b)` → clean, no output.
- `(cd web && npx oxlint)` → clean, no output.
- `shellcheck -S error` on `deploy/update/styrman-update`, `deploy/host-setup.sh`,
  `web/e2e/run.sh` → clean (exit 0, no output) on all three.
- `shellcheck` (default severity, no `-S`) on `deploy/update/styrman-update`
  → one **info**-level SC1091 ("not following: /etc/styrman/update.env was
  not specified as input") on the conditional `. /etc/styrman/update.env`
  sourcing line — expected and benign (the file legitimately may not exist),
  not a regression.
- `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:dev`
  (after `docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/`)
  → **passed=57 failed=0 skipped=3**. Matches the operator's measurement.
- `bash web/e2e/run.sh styrman-runtime:dev`:
  - **First run**: passed=4 failed=6 skipped=0 (timeouts booting a microVM
    and reaching the shell prompt). This run overlapped with heavy
    concurrent `sudo`/`docker build`/git activity from the tag-gate scratch
    testing below; host load/memory were otherwise unremarkable
    (`load average: 0.42`, 58 GiB reclaimable buff/cache) so I do not have a
    root-caused explanation beyond contention, and I do not count this run
    as evidence of a regression given the immediate clean re-run below.
  - **Second run, nothing else running concurrently**: **passed=48
    failed=0 skipped=2** — clean. This is the number that stands.
    `grep -c "await check("` on `web/e2e/run.mjs` returns 49 (one check
    apparently reachable more than once per static count vs. runtime
    invocations, not investigated further since the runtime number is what
    matters and it reconciles with the docs below).
  - Documentation note: `AGENTS.md` and
    `.agents/docs/testing/styrman/README.md` were updated **during this
    verification session** (by a concurrent documentation pass, not by me)
    to say "50 checks (48 on firewall-hardened hosts, plus 2 auto-SKIP)" /
    "48 passed + 2 auto-SKIP" — this now matches my independently measured
    48/0/2 exactly. The changelog
    (`.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md:239,259`)
    still says "47 passed" — a stale historical snapshot from when it was
    written, not corrected retroactively, and not something a verifier
    should edit. Not a blocking discrepancy since the living source of
    truth (README/AGENTS) is now accurate.

## Independent verification of the specific claims (re-derived from scratch)

**1. Tag signature gate.** Built a scratch repo (`git init --bare` origin +
working clone) with real SSH signing (`gpg.format=ssh`,
`user.signingkey`, `gpg.ssh.allowedSignersFile`), then ran the actual,
unmodified `deploy/update/styrman-update --check` as root
(`sudo`) against a scratch deploy clone (`STYRMAN_DEPLOY_DIR=<scratch>`,
never `/opt/styrman`), with the clone and its origin `chown root:root` to
satisfy the script's own ownership guard:
- Lightweight tag only on origin → `FATAL: v0.0.1 is a lightweight tag and
  cannot be verified.` — refused, confirmed.
- Annotated-but-unsigned tag only → `FATAL: v0.0.2 has no valid
  signature — REFUSING to build and run it as root.` — refused, confirmed.
- Annotated + SSH-signed tag → `v0.0.3 signature OK`, proceeds to the
  update-available check — accepted, confirmed.
- Verification config entirely absent (`gpg.ssh.allowedSignersFile`
  unset) → `error: gpg.ssh.allowedSignersFile needs to be configured…` →
  `FATAL: … has no valid signature` — fails CLOSED when signing cannot be
  verified at all, confirmed.
- Same unsigned tag with `/etc/styrman/update.env` containing
  `STYRMAN_UPDATE_REQUIRE_SIGNED=0` → `WARNING: signature checking
  DISABLED …` then proceeds — confirmed this is the only path past the
  gate; without the file, the exact same unsigned tag is refused.

**2. State files survive `git clean`.** In the same scratch clone:
- **Old layout reproduced**: wrote `.deployed-version`/`.failed-version` at
  the clone root (root-owned, confirmed `git check-ignore` reports no
  match — i.e. genuinely untracked-and-unignored), ran
  `git clean -d --force --quiet` → both files gone. Reproduces the exact
  regression the re-review found.
- **New layout**: wrote the same two files at
  `/var/lib/styrman/update/{deployed,failed}-version` (outside the work
  tree), ran the identical `git clean -d --force --quiet` inside the clone
  → both files intact afterward. Confirmed the fix holds.

**3. `LABEL styrman.version` round-trip.** Built
`deploy/Dockerfile.server` standalone
(`docker build -f deploy/Dockerfile.server --build-arg STYRMAN_VERSION=9.9.9 .`)
and confirmed `docker inspect --format '{{index .Config.Labels
"styrman.version"}}'` returns `9.9.9`; an unlabelled image
(`debian:trixie-slim`) returns an empty string. Extracted the *actual*
`reports_version()` function body verbatim from the script (container name
substituted only to avoid colliding with the host's real, running
`styrman-server`) and exercised the exact decision table against real
containers:
  - correct label + running + healthy → `VERIFIED`
  - wrong `want` version → `REFUSED`
  - empty `want` → `REFUSED` (the empty-label-never-matches-empty-want
    guard holds)
  - unlabelled image (`debian:trixie-slim`) → `REFUSED` with the
    "carries NO styrman.version label" message
  - stopped container (label correct, `running=false`) → `REFUSED`
  All five cases match the script author's own claimed table.

**4. `PYTHONPYCACHEPREFIX` vs `PYTHONDONTWRITEBYTECODE`.** Inside the
actual rebuilt `styrman-runtime:dev` image (hadolint 2.15.1 + sha256
verified during the build, shellcheck present):
  - `PYTHONDONTWRITEBYTECODE=1 python3 -m py_compile tmp.ABCDEF.py` →
    `__pycache__/tmp.ABCDEF.cpython-313.pyc` created next to the source —
    confirmed inert, exactly as both reviews found.
  - `PYTHONPYCACHEPREFIX=/tmp/styrman-pyc python3 -m py_compile tmp.GHIJKL.py`
    → no `__pycache__` next to the source; the `.pyc` lands under
    `/tmp/styrman-pyc` instead — confirmed fixed.
  - Live end-to-end: started an isolated `styrman` instance
    (`STYRMAN_STATE_DIR`/`STYRMAN_ADDR` pointed at scratch/loopback, per the
    Manual/UX tier), created a throwaway project via the API, called
    `POST /api/projects/{id}/check` on a `.py` file, then `docker exec`'d
    into the project's own microVM and confirmed the workspace directory
    contains neither a `tmp.*` file nor a `__pycache__` directory.

**5. `safeSuffix` injection.** `TestSafeSuffixBlocksInjection` passes
(`go test ./internal/httpapi/ -run TestSafeSuffixBlocksInjection`). Live
confirmation against the throwaway project above:
`POST /check {"path":"Dockerfile.$(id)", ...}` returns a normal hadolint
diagnostic (`DL3007`), and the guest workspace afterward shows no evidence
of command execution (no new files, no `uid=` output anywhere) — the
generated `mktemp --suffix=.id` (letters only) never reaches the shell as
`$(id)`.

**6. `TestGuestScriptsNeverWaitForStdinEOF` negative control.** Backed up
`check.go`/`format.go`, edited the generated script in `checkScript` to
`cat > "$F"` in place of `head -c %d > "$F"` (keeping the `fmt.Sprintf` arg
count valid via a no-op `: %d`), reran the test:
`--- FAIL: TestGuestScriptsNeverWaitForStdinEOF` — both assertions
(missing `head -c`, presence of the dangerous `cat > "$F"` pattern) fired
correctly. Restored the file from the backup; confirmed `go build ./...`
and the test pass again afterward, and `git diff` shows the file unchanged
from HEAD's working-tree state.

**7. `/format` data-loss + `/check` matrix.** Built the server, started an
isolated instance, created a throwaway project (`verifytest`), waited for
the microVM to report ready, then:
  - `POST /format` on unformatted Go source → returned properly formatted
    Go (`package main\n\nfunc main() { println("hi") }\n`), **not**
    `{"content":""}`.
  - `POST /check` on a broken shell script → 4 shellcheck diagnostics on
    the correct lines (SC1046/SC1073/SC1047/SC1072).
  - `POST /check` on a clean shell script → `diagnostics: []`.
  - `POST /check` on a Dockerfile → hadolint DL3007 warning + DL3009 info.
  - `POST /check` on broken JSON → a jq parse error at the correct
    line/column.
  - `POST /check` on an unsupported `.txt` → `{"supported":false,
    "diagnostics":[]}` with **HTTP 200**, not a 4xx.
  - Deleted the throwaway project via `DELETE /api/projects/{id}` before
    tearing down my scratch server; confirmed (after the async delete
    finished) that the container and its bridge network were both gone.
    No operator project (`test3321`/928e709a, `asdasd`/36782cc5, or the
    third running project) was started, stopped, or exec'd into at any
    point — confirmed by `docker ps -a` before and after showing only
    those five pre-existing containers untouched.

## Coverage of changed code

- `internal/httpapi/check.go` + `check_test.go` — **covered**. Unit tests
  exist for every pure-logic seam that matters:
  `TestParseCheckOutput`/`TestParseCheckOutputClean` (per-tool parsers),
  `TestCheckerFor`/`TestShellDialectFlag` (checker selection),
  `TestMissingTool` (now includes the motivating false-positive case —
  `"module 'x' not found"` — plus a genuine multi-line real-diagnostic
  case), `TestSafeSuffixBlocksInjection` (the injection fix), and
  `TestGuestScriptsNeverWaitForStdinEOF` (the stdin contract, independently
  negative-controlled above). The HTTP handler `checkFile` itself has no
  Go-level test — **this is a real, but already-tracked** gap
  (`TODO.md`: "`checkFile`/`formatFile` … have no Go handler-level test —
  only e2e coverage"). I additionally exercised the handler live via the
  API matrix above, which is the same class of coverage the codebase
  already accepts for `formatFile`.
- `internal/httpapi/format.go`, `internal/orchestrator/exec.go` — covered
  by `TestGuestScriptsNeverWaitForStdinEOF` plus the live `/format` check
  above. Per the changelog's own honest admission, unit tests could not
  have caught the original stdin/half-close bug (it was a transport issue,
  not a logic one) — that is why the live/e2e layer matters here and I
  exercised it directly rather than trusting the assertion.
- `deploy/update/styrman-update` (+ `.service`/`.timer`) — **zero automated
  test coverage** (no bats/CI harness for this repo's shell scripts at
  all). This matches the existing, accepted convention for this codebase
  (`deploy/host-setup.sh`, `deploy/backup/styrman-host-backup` are likewise
  shellcheck-only + manually verified, with no test files). Given that
  precedent, I am not failing the batch over the absence of automated shell
  tests, but flagging plainly: the only regression protection for this
  root-privileged, security-sensitive script is human diligence — mine
  today, and the two reviewers' before me. All five signature-gate
  branches and all five `reports_version` decision-table branches were
  verified by hand against real git/docker state in this pass.
- `deploy/Dockerfile.server`, `image/Dockerfile` — no "test" applies beyond
  a real build, which I ran (twice: server + runtime image), confirming
  the LABEL, the hadolint pin+checksum, and the cache-layer ordering.

### Test that would pass even if the feature were broken (explicitly requested)

**Yes — found one.** `web/e2e/run.mjs`'s "syntax check leaves no temp files
in the workspace" check (`run.mjs:320-327`) only ever creates/opens a
`.sh` file and only asserts the panel listing does not match
`/tmp\.[A-Za-z0-9]+\./`. A `__pycache__` **directory** can never match that
regex. This is exactly how the previous round's inert
`PYTHONDONTWRITEBYTECODE` fix passed the batch's own e2e gate while still
leaving litter on every `.py` check — the re-review called this out
explicitly, and it is still true today: the current
`PYTHONPYCACHEPREFIX` fix is genuinely correct (verified by hand above,
twice), but there is still no automated check that would catch a
regression back to the old, broken behavior. Unlike the other carried-over
gaps, **this one is not recorded in `TODO.md`** — it should be, as a
follow-up: extend the e2e litter check to open a `.py` file and assert
neither `tmp.` nor `__pycache__` appears in the listing.

## Cleanup performed

- Removed the throwaway `verifytest` project (API `DELETE`, confirmed
  container + network gone) and killed/removed my scratch `styrman`
  instance and its state dir.
- Removed the test containers (`vtestctr`, `pytest`, `verify-server` and
  variants) and the `styrman-server:verifytest` test image.
- Restored `internal/httpapi/check.go` from backup after the negative
  control edit; confirmed `git diff` shows no residual change and
  `go build`/`gofmt -l` are clean afterward.
- Deleted a stray file accidentally created by my own `sed` invocation
  during that edit (a literal filename `as: head -c %d > "$F"#` in the
  repo root, from a shell-quoting mistake on my part, unrelated to the
  code under test) — confirmed removed and not part of `git status`.
- Reverted the `safe.directory`/`gpg.*` entries I added to root's global
  `.gitconfig` for the scratch-repo signing tests; confirmed root's
  `.gitconfig` is back to its pre-existing state (two `/opt/devbox` entries
  that predate this session, unrelated to styrman).
- Removed `/var/lib/styrman` (my scratch state-file testing) and confirmed
  `/etc/styrman` was never created (no systemd units were installed or
  enabled, no `/opt/styrman` was created, per the task's constraints).
- Deleted all scratch directories under the session scratchpad.
- Did not start, stop, or exec into any operator project. `docker ps -a`
  before and after shows the same five pre-existing containers
  (`styrman-982ffddc…`, `styrman-36782cc5…`, `styrman-server`,
  `styrman-928e709a…`, `styrman-caddy`) untouched throughout.
- `web/dist/client/**` churn from the two `web/e2e/run.sh` runs is expected
  build-output noise per the testing README ("A run therefore rewrites the
  TRACKED web/dist/client files with fresh content-hashed asset names") —
  left as-is, consistent with the pre-existing dirty state at the start of
  this session (the batch's own last e2e run had already left it dirty
  before I started).

## Environment

Tier: local, on the actual target host (bare-metal Debian, per R9) — not a
remote box or GCP VM. `docker version`: Engine 29.7.2, containerd 2.3.3.
Kata `mvm` runtime confirmed working
(`docker run --runtime mvm --rm debian:trixie uname -r` → `6.18.28`,
differs from host `6.12.101+deb13-amd64`, per the `styrman-runtime` build
and integration suite). `nproc`=16, 62 GiB RAM, load average ~0.4
throughout. `sudo` available password-less for the root-required tag-gate
tests.
