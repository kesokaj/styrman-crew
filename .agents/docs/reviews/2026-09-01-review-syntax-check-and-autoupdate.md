# Review — syntax-check, exec stdin fix, and tag-gated auto-updater (2026-09-01)

Scope: the SECOND batch of the 2026-09-01 feature cycle in the uncommitted working tree.
Reviewed: `internal/orchestrator/exec.go`, `internal/httpapi/{check.go,check_test.go,format.go,server.go}`,
`web/src/components/{CodeEditor.tsx,FileEditor.tsx,EditorPane.tsx}`, `web/src/lib/api.ts`,
`web/package.json`, `image/Dockerfile`, `deploy/update/*`, `deploy/host-setup.sh` (step 8c),
`web/e2e/{run.sh,run.mjs}`, and the batch's docs (`DECISIONS.md` ADR-049/050,
`changes/2026-09-01-syntax-check-and-autoupdate.md`, `AGENTS.md`,
`testing/styrman/README.md`).

Explicitly NOT re-reviewed (per dispatch): the first batch — editor pane, terminal
ordering (`internal/conversation/store.go`), Dockerfile grammar matching, `FileExplorer`
extraction — except where this batch changed the same file. `web/dist/**` is generated
build output and was not read.

## Verdict: REQUEST CHANGES

The auto-updater cannot work. `reports_version()` polls `GET /api/version` with
`X-Caddy-Email: updater@localhost`, but in production compose the server runs
`STYRMAN_AUTH=header` and `authGuard` exempts **only** `/api/health` — `updater@localhost`
is not in the userstore, so every poll gets a 403 whose body contains no version string.
`reports_version` therefore times out after 120 s on **every** deploy, including a
perfectly healthy one; the script then rolls back, writes the new tag to `.failed-version`,
fails the rollback verification the same way, and exits FATAL. Net effect once enabled:
every pushed `vX.Y.Z` tag burns four image builds, is permanently blacklisted, and the
host never updates. Everything else below is smaller, but three of them are also
production-path bugs (root SSH clone, `/root/.gitconfig` growth, first-run retry storm).
The Go/frontend half of the batch is in much better shape — the stdin fix and its reasoning
are correct — but it still ships a shell-injection vector, a workspace-litter path the batch
claims to have closed, and two places where a failed check renders as a clean file.

## Findings

### Auto-updater (f)

- **[high / confidence: high]** `deploy/update/styrman-update:129-143` — `reports_version()`
  will always fail in production. `/api/version` is behind `authGuard`
  (`internal/httpapi/auth.go:28-31` exempts `/api/health` only); with
  `STYRMAN_AUTH=header` (`deploy/docker-compose.yml:83`), `userstore.IsAuthorized("updater@localhost")`
  returns false (`internal/userstore/userstore.go:117-129`) → HTTP 403, body
  `{"error":"updater@localhost is not authorized for styrman"}`, which never matches
  `grep -q "\"${want}\""`. Consequence: every healthy deploy is judged unhealthy →
  rollback → `.failed-version` latch → the tag is never retried without `--force`.
  Fix: do not go through the guarded HTTP surface. Either (a) exempt `/api/version` in
  `authGuard` the way `/api/health` is (it discloses only a version string), or (b) read
  the version out of band — `docker inspect -f '{{index .Config.Labels "org.opencontainers.image.version"}}' styrman-server`,
  or add the version to the already-exempt `/api/health` payload
  (`internal/httpapi/server.go:1154-1167`) and poll that. Whichever is chosen, this path
  MUST be exercised against a `STYRMAN_AUTH=header` server before the timer is enabled.

- **[high / confidence: high]** `deploy/update/styrman-update:131-132` — the health check
  **fails open**: `[ -n "${token}" ] || { log "WARN: …cannot verify"; return 0; }`. A
  renamed/absent `STYRMAN_INGRESS_TOKEN` silently converts "cannot verify" into "healthy",
  records the tag in `.deployed-version`, and disables rollback entirely for that release —
  the exact scenario rollback exists for. Fix: return non-zero (or better, take the
  out-of-band version path above, which needs no token at all).

- **[medium / confidence: high]** `deploy/host-setup.sh:339-347` — the deploy clone is
  created from `git -C "${REPO_ROOT}" remote get-url origin`, which on this repo is
  `git@github.com:kesokaj/styrman-crew.git` (SSH). Step 8c runs as root; root has no
  deploy key and no known_hosts entry for github.com, so `git clone` fails, the script
  prints a single `WARN: clone failed` into a long install log, and the whole feature is
  silently absent. The updater's own `git fetch` (line 71) would fail the same way, and it
  too only WARNs — it would then deploy stale local tags forever. Fix: rewrite an
  `git@github.com:` origin to `https://github.com/…` for the clone, require an explicit
  `STYRMAN_DEPLOY_ORIGIN`/deploy-key setup step, and make the updater's fetch failure a
  hard error rather than a warning when no new tag was found.

- **[medium / confidence: high]** `deploy/update/styrman-update:65` —
  `git config --global --add safe.directory "${DEPLOY_DIR}"` runs on **every tick**, and
  `--add` never de-duplicates (verified: three invocations produce three `directory =` lines).
  At a 5-minute cadence that is ~105 k duplicate lines in `/root/.gitconfig` per year,
  parsed by every git invocation on the box. Fix: `git config --global --get-all safe.directory | grep -qxF "${DEPLOY_DIR}" || git config --global --add …`,
  or drop the global write entirely and use `git -c safe.directory="${DEPLOY_DIR}" …`.

- **[medium / confidence: high]** `deploy/update/styrman-update:145-163` — on the **first
  run with no state file**, `CURRENT` falls back to `none`. If that first deploy fails,
  the `PREVIOUS = none` branch (line 155) calls `fail` **before** `printf '%s' "${LATEST}" > "${FAILED}"`
  (line 159), so the failed tag is never latched. The 5-minute timer then rebuilds both
  images, fails, and repeats forever — precisely the churn the latch was added to prevent,
  on precisely the machine most likely to hit it (a fresh install). Fix: write `FAILED`
  immediately after the deploy/health check fails, before any `PREVIOUS` branching.

- **[medium / confidence: medium]** `deploy/update/styrman-update:135` — the health poll
  shells out to `docker run --rm --network styrman-ingress curlimages/curl`. That puts an
  **unpinned third-party Docker Hub image on the critical path of the rollback decision**:
  a Hub outage, a rate limit, or a pull failure makes `reports_version` return 1 and rolls
  back a healthy release. It is also the batch's only unjustified new runtime dependency
  (criterion 5 — stdlib/host-first). Fix: use a tool already on the host or already in an
  image you build (`docker exec styrman-server wget -qO-`, or the host's `curl` against the
  published port); if the image is kept, pin it by digest.

- **[low / confidence: high]** `deploy/update/styrman-update:115` — `deploy()` mixes error
  conventions: `git checkout … || fail "checkout ${tag} failed"` **exits the script**, while
  the build failures `return 1`. During rollback (line 160) a checkout failure therefore
  aborts with "checkout failed" instead of the intended
  "rollback … ALSO failed — manual intervention required". Also, a *successful* rollback
  exits through `fail`, so the operator sees `FATAL:` on the one path that worked. Fix:
  `return 1` from `deploy()` throughout; use a distinct non-fatal exit for a successful rollback.

- **[low / confidence: high]** `deploy/update/styrman-update:71-77` — nothing verifies tag
  provenance. Anyone able to push a `vX.Y.Z` tag to origin gets root-level code execution on
  the host within 5 minutes (the tag's `image/Dockerfile` and `deploy/docker-compose.yml`
  are both executed as root). The repo is private and single-operator, so this is a residual
  risk, not an open door — but for an unattended auto-deployer it should be recorded and
  ideally closed with `git verify-tag "${LATEST}"` against a pinned signing key.

- **[low / confidence: high]** `deploy/update/styrman-update:131` — `cut -d= -f2-` takes the
  `.env` value verbatim. If the operator ever quotes it (`STYRMAN_INGRESS_TOKEN="abc"`, which
  compose accepts), the quotes end up in the header and the health check fails → spurious
  rollback. Strip surrounding quotes, or source the file.

- **[low / confidence: high]** `deploy/update/styrman-update.timer:22` — `Persistent=true`
  has no effect on a purely monotonic timer (`OnBootSec`/`OnUnitActiveSec`); it only applies
  to `OnCalendar=`. Delete it or switch to `OnCalendar=`.

- **[low / confidence: high]** `deploy/host-setup.sh:330,372` — the new step **8c is inserted
  physically before step 8b**. Cosmetic, but this file is read top-to-bottom by an operator
  during a fresh install; move it after 8b or renumber.

### Guest exec / stdin contract (a) — reasoning is correct, one gap left

The diagnosis and the fix are right. The hijacked exec connection is a single bidirectional
stream, `CloseWrite()` tears it down, and only these two callers pass `Stdin`
(`format.go:95`, `check.go:328`) — both now use `head -c <len>`, so nothing else depends on
stdin EOF. `head -c N` counts **bytes**, and `len(body.Content)` is the Go byte length of the
same payload, so multi-byte UTF-8 and binary content are handled correctly. The write
goroutine (`exec.go:106-108`) cannot leak: if the guest never reads, `Write` blocks on the
socket buffer until the deferred `att.Close()` returns an error to it, and `net.Conn` is safe
for concurrent Write/Close. `TestGuestScriptsNeverWaitForStdinEOF` is a good guard and the
negative-control test is the right diligence.

- **[medium / confidence: medium]** `internal/httpapi/format.go:118-123`,
  `internal/httpapi/check.go:370-377` — `head -c N` **succeeds on a short read**. If the
  stdin write fails partway or dockerd closes the exec stdin early, `head` exits with a
  truncated file, the formatter runs on it, and `/format` returns a *partial* document that
  `FileEditor.doFormat` pushes straight into the buffer via `setContent`
  (`FileEditor.tsx:204`) — the same data-loss class this batch exists to close, just smaller.
  Fix, one line: after `head`, `[ "$(wc -c <"$F")" -eq N ] || { printf 'STYRMAN_ERR\ntruncated stdin'; exit 0; }`.
  Worth pairing with a client-side guard in `doFormat` (`if (!content && text) { toast(...); return; }`) —
  that alone would have caught the original bug at the UI.

### Syntax checking (b)

- **[medium / confidence: high]** `internal/httpapi/check.go:302-319` — **shell injection**
  via `suffix`. `suffix` derives from `filepath.Ext(body.Path)` and is interpolated
  **unquoted** into `F=$(mktemp -p . --suffix=.%s)`. For every other file type the `switch`
  in `checkerFor` allowlists `ext`, but the Dockerfile branch matches on
  `strings.HasPrefix(strings.ToLower(base), "dockerfile.")` (line 64), which accepts an
  arbitrary extension. Verified: `path = "Dockerfile.$(id)"` → `base = "Dockerfile.$(id)"`,
  `ext = "$(id)"`, `checkerFor` returns the hadolint checker, and the generated script is
  `F=$(mktemp -p . --suffix=.$(id))` — command substitution inside `bash -c`. Same for
  backticks and `;`. No privilege boundary is crossed (it runs as the guest user in the
  requester's own VM), but "click a file in the explorer" becomes "execute code" — a repo
  containing a file named `Dockerfile.$(...)` is a drive-by. It also falsifies the comment
  at `format.go:24-26` / `check.go:313` claiming no shell interpolation. Fix: clamp —
  `if !regexp.MustCompile(`^[a-z0-9]{1,16}$`).MatchString(suffix) { suffix = "txt" }`, or
  carry the suffix on the `checker` struct instead of deriving it from user input.
  (`format.go` is currently safe only because `formatInPlace` allowlists `ext`; that
  invariant is implicit — a future basename-matched formatter reintroduces the hole.)

- **[medium / confidence: high]** `internal/httpapi/check.go:89` — `python3 -m py_compile "$F"`
  writes the compiled output to `__pycache__/tmp.XXXXXX.cpython-3NN.pyc` **next to the source**,
  i.e. into the workspace root. The `trap` only removes `$F`, `${F}.err`, `${F}.out`, so
  checking a Python file leaves permanent litter — the exact problem section (c) of the
  changelog claims to have fixed. The e2e litter check only exercises a shell file
  (`run.mjs`, "syntax check leaves no temp files"), so it does not catch this. Fix: use
  `python3 -c 'import ast,sys;ast.parse(open(sys.argv[1]).read(), sys.argv[1])' "$F"`
  (traceback output still matches `rePyLine`/`rePyErr`), or add `__pycache__` cleanup for
  the generated name to the trap.

- **[medium / confidence: medium]** `internal/httpapi/check.go:268-271` — `missingTool()`
  substring-matches `"not found"` against the **entire tool output**, including legitimate
  diagnostics. When it fires, `checkFile` returns `unavailable:true` with an **empty**
  `diagnostics` array (lines 350-356), so real findings are discarded and the gutter reads
  clean. This is the "failed check presents as clean" hazard the design was supposed to
  avoid. Fix: have the guest report it explicitly instead of inferring it — the shell branch
  already does exactly the right thing with `command -v shellcheck`; generalise that to
  `command -v <tool> >/dev/null || { printf 'STYRMAN_MISSING %s\n' <tool>; exit 0; }` and
  parse that marker.

- **[low / confidence: high]** `internal/httpapi/check.go:96-97` — the Terraform checker
  reports `STYRMAN_TOOL gcc`, so the JSON `tool` field returned to the UI literally says
  `"gcc"` for `.tf` files (while the `unavailable` branch reports `"tofu"` — inconsistent).
  The tool name and the parser selector are being conflated. Fix: add a `parser` field to
  `checker` and keep `tool` truthful. Separately, `tofu fmt -check` is a *formatting* check,
  not a syntax check — a valid-but-unformatted file yields exit 3 with its output sent to
  `/dev/null`, i.e. silently "clean"; `tofu validate` or `tofu fmt` without `-check` would
  be more honest about what is being reported.

- **[low / confidence: high]** `internal/httpapi/check.go:53-59` — `checkerFor`'s doc comment
  is orphaned: the three-line paragraph beginning "checkerFor maps a file extension to its
  checker" sits immediately above `func hasShebang`, so godoc attributes it to `hasShebang`.
  Move it back onto `checkerFor`.

- **[low / confidence: high]** `internal/httpapi/format.go:119` and `check.go:372` — dead
  cleanup entries and duplicated work. The format trap removes `${F}.out`, which format never
  creates; the check trap removes `${F}.err`, which check never creates; and **both** scripts
  still `rm -f` explicitly on the last line, which the trap already covers. Copy-paste
  residue — delete the stale globs and the trailing `rm -f`.

### Temp-file placement (c)

- **[medium / confidence: medium]** `internal/httpapi/{format.go:118, check.go:371}` —
  `mktemp -p .` with `WorkDir=ws` puts the temp file in the **workspace root**, and the lint
  gutter fires it on a 1 s debounce for the whole time the operator types. Two concrete
  consequences: (1) R13 makes every workspace a git repository, so a transient
  `tmp.XXXXXX.go` shows up in `git status` and in the git side panel, and a `git add -A`
  racing a check can commit it; (2) a stray `.go` file with a mismatched package clause in a
  module root intermittently breaks `go build ./...`, `go vet`, and any watcher the operator
  has running in the terminal alongside the editor. The trap also does not cover a hard
  teardown — bash runs the `EXIT` trap on normal exit but not on an untrapped fatal signal,
  so a killed exec still leaks the file.
  Recommended fix that keeps config discovery working: create the temp file in a
  dot-prefixed subdirectory of the workspace, e.g. `$ws/.styrman/tmp/`. Tool config lookup
  walks *up*, so `.golangci.yml` / `pyproject.toml` / `.hadolint.yaml` are still found; Go
  tooling ignores directories starting with `.` or `_`, so the module-root problem
  disappears; and adding the path to `.git/info/exclude` (not the user's `.gitignore`) keeps
  `git status` clean.

### Frontend (d)

- **[medium / confidence: high]** `web/src/components/FileEditor.tsx:180-183` — the `catch`
  returns `[]` and clears `checkNote`, so a failed check is **indistinguishable from a clean
  file**: the gutter is empty and the toolbar says nothing. The comment three lines above
  ("The ONE thing it must never do is claim a clean file when it does not know") describes
  the opposite of what the code does. `@codemirror/lint` will also swallow a rejected source
  via `logException` and leave the previous (possibly empty) diagnostic set in place, so
  there is no second line of defence. Fix: in the catch, `setCheckNote("syntax check unavailable")`
  (and consider distinguishing the 409 "start the project" case) before returning `[]`.

- **[low / confidence: high]** `web/src/components/CodeEditor.tsx:216-241` — the position
  clamping itself is correct: `lineNo` is clamped into `[1, doc.lines]`, `from` is clamped to
  `line.to`, and `from <= to` always holds, so `doc.line()` cannot throw and CodeMirror cannot
  get an out-of-range range. `d.col ?? 1` correctly handles the server's `omitempty` on `col`.
  No change needed — recorded because the dispatch asked.

- **[low / confidence: high]** `web/src/lib/api.ts:443-447` / `CodeEditor.tsx:217` —
  `checkFile` takes no `AbortSignal`, so a check outliving the editor (file switch, pane
  close, unmount) still completes and still calls `setCheckNote`. Harmless under React 18,
  but it violates the TypeScript review rule that an `AbortSignal` reaches long operations
  that can outlive their component, and it allows a slow response to overwrite a newer one.

- **[low / confidence: medium]** `web/src/components/CodeEditor.tsx:215-242` — `lintGutter()`
  and `linter()` are added unconditionally, including when `check` is undefined (the read-only
  Settings credential viewer, `settings.tsx:695`). The linter correctly returns `[]`, but the
  gutter column is still rendered. Gate both on `check` being supplied.

- **[low / confidence: medium]** `image/Dockerfile:48` — `hadolint` is fetched from
  `releases/latest` with no version pin and no checksum. This matches the existing `yq` line,
  so it is a consistency issue rather than a regression, but the changelog names a specific
  version ("2.15.1") that the Dockerfile does not actually pin — the recorded version and the
  built version can silently diverge.

### ConfirmDialog / discard-and-open (e)

The diagnosis is right (`ui.tsx:242-245` calls `onConfirm()` then `onClose()` unconditionally)
and the ref is correct as written — `discardingRef` is only ever true for one synchronous span,
and both the Escape/backdrop path and the Cancel button reach `keepEditing` with the flag
false. I checked the other five callers (`FileExplorer.tsx:375`, `SidePanelGit.tsx:230`,
`TerminalList.tsx:172,182`, `SetupBanner.tsx:88`): all use `onClose` as a pure dismissal
(`setState(null)`/`setState(false)`), so none is affected by the ordering and none would break
if the contract changed.

- **[low / confidence: high]** `web/src/components/ui.tsx:219-252` and
  `web/src/components/EditorPane.tsx:96-124` — the ref works, but it exists only because
  `onClose` is overloaded to mean both "dismiss" and "cancel", and EditorPane is the one
  caller whose cancel path has a side effect (re-pointing the store at `loaded.req`). The
  cleaner fix is in `ConfirmDialog`, and it is non-breaking: add an optional
  `onCancel?: () => void` used by the Cancel button and by `Dialog`'s `onClose`, defaulting to
  `onClose`. EditorPane then passes `onConfirm={discardAndOpen} onCancel={keepEditing} onClose={() => setPending(null)}`
  and the mutable ref, the comment explaining it, and the ordering coupling all disappear.
  Four lines in `ui.tsx`, zero churn at the other five call sites. Prefer this over the ref.

- **[low / confidence: high]** `web/src/components/EditorPane.tsx:178-190` — the "Discard"
  button on the close-file dialog does not discard anything: it calls `onShowTerminal()` while
  `loaded` and the dirty buffer stay in place, so toggling back to Editor shows the same
  unsaved edits. Nothing is lost, but the label promises something it does not do. Either
  rename it ("Leave open") or actually reset the buffer.

### Documentation

- **[low / confidence: high]** `AGENTS.md:90-98` and
  `.agents/docs/testing/styrman/README.md:49,71-72` — the e2e counts are stale for this batch.
  Both say "45 checks / 43 + 2 auto-SKIP", the changelog says "47 passed / 0 failed / 2 skipped",
  and `web/e2e/run.mjs` actually contains 48 `await check(` calls. The README declares itself
  "the source of truth" and is the one that is wrong. Reconcile all three against a real run.

## Criteria

1. **Dead code — pass with nits.** No orphaned files, no commented-out blocks, no unreferenced
   exports; `check.go`'s regexes, `missingTool`, `hasShebang`, `atoi`, `firstLines` all have
   callers. The only dead material is inside the guest scripts: trap globs for files that are
   never created (`${F}.out` in format, `${F}.err` in check) and the trailing `rm -f` that the
   trap already covers — delete both.
2. **Flow tracing — fail.** The break is at a package seam the batch did not trace:
   `styrman-update` → `/api/version` → `authGuard`, where only `/api/health` is exempt and the
   synthetic identity is unauthorized. Two more error paths swallow information rather than
   propagating it — `missingTool` discards real diagnostics, and `FileEditor.runCheck`'s catch
   converts every failure into "clean". Inside Go the tracing is good: `Stdin` has exactly two
   callers and both honour the new contract, `Exec`'s goroutine has an exit path via the
   deferred `att.Close()`, and every `err` is wrapped or surfaced.
3. **Standards conformance — mostly pass.** `gofmt -l`, `go vet ./...`, `go build ./...`, and
   `go test ./internal/httpapi/` re-run clean here; `shellcheck` on `deploy/update/styrman-update`
   is clean at default severity, not just `-S error`. Go nits: the orphaned `checkerFor` doc
   comment, and `check.go` mixing tool identity with parser selection. TypeScript nit: no
   `AbortSignal` on a fetch that outlives its component.
4. **Directory layout — pass.** `check.go`/`check_test.go` sit beside `format.go` in
   `internal/httpapi/`; `deploy/update/` mirrors the existing `deploy/backup/` convention;
   `EditorPane.tsx`/`FileEditor.tsx`/`editorPane.ts` are in the established `web/src/components`
   and `web/src/lib`. Only wart: step 8c placed above step 8b in `host-setup.sh`.
5. **Simplicity (DIY-except-security) — pass with one exception.** No hand-rolled auth, crypto,
   hashing, sessions, or JWT/OIDC anywhere in this batch; the ingress-token comparison it reuses
   is the existing `crypto/subtle` path. `@codemirror/lint` is justified (already transitive,
   and the gutter is the feature). `shellcheck`/`hadolint` in the image are justified — they are
   the only real checkers for the two formats that had none. The unjustified dependency is
   `curlimages/curl` pulled from Docker Hub *inside the rollback decision*: it buys nothing the
   host or the server container cannot already do, and it can cause a spurious rollback.
   Per-tool parsers are the right call, not over-abstraction — the tools genuinely disagree.
6. **Decoupling — pass.** The `useFileEditorState` / `FileEditorToolbar` / `FileEditorBody` split
   is a good seam: `EditorPane` and the Settings viewer consume different subsets without either
   knowing about the other, and `notifyFileSaved` keeps the explorer and the pane siblings rather
   than parent/child. `checker` is a small, consumer-shaped struct. The one coupling defect is
   `EditorPane`'s `discardingRef`, which reaches into `ConfirmDialog`'s *call ordering* — an
   invisible contract nobody else can see. Fixing it in `ConfirmDialog` (optional `onCancel`)
   restores the seam.

## Assessment of the verification already run

Sufficient for (a), (b), (d), (e): `go build`/`vet`/`test -race`, `gofmt -l`, `tsc -b`, `oxlint`,
`shellcheck -S error`, integration 57/0/3, e2e with four new checks, and live API testing against
a real project. The negative-control test on `TestGuestScriptsNeverWaitForStdinEOF` is exactly the
right rigour for a transport bug that unit tests could not see.

Three gaps:

- **(f) is effectively unverified where it matters.** Tag selection, `sort -V` ordering, the
  failed-tag latch, missing-clone, and the flock race were all exercised — but the health check,
  which is the single branch that decides deploy-vs-rollback, was never run against a
  `STYRMAN_AUTH=header` server. That is where the high-severity bug lives. A test against a
  production-mode server (or the out-of-band version read) is required before the timer is enabled.
- **The temp-litter e2e check only covers a shell file**, so it cannot see the Python
  `__pycache__` leak. Extend it to run a check on a `.py` file and assert no `__pycache__`.
- **No handler-level Go test for `checkFile`** (already recorded in `TODO.md` by the verifier).
  A table test over `checkerFor` + `checkScript` would have caught the `Dockerfile.$(…)` suffix
  injection cheaply.
