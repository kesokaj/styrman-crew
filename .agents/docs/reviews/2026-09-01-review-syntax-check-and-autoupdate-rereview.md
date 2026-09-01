# Review — syntax-check and auto-updater, RE-REVIEW of the remediation (2026-09-01)

Re-review of the fixes applied to the findings in
`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate.md`.
Scope: the same uncommitted working tree, re-read after remediation —
`deploy/update/styrman-update{,.service,.timer}`, `deploy/Dockerfile.server`,
`deploy/host-setup.sh` (step 8c), `internal/httpapi/{check.go,check_test.go,format.go}`,
`web/src/components/{ui.tsx,EditorPane.tsx,FileEditor.tsx,CodeEditor.tsx}`,
`web/e2e/run.mjs`, and the batch's docs (ADR-048/049/050, the changelog, `AGENTS.md`,
`.agents/docs/testing/styrman/README.md`). `web/dist/**` is build output and was not read.

## Verdict: REQUEST CHANGES

Both **high**-severity findings are genuinely and well fixed — the label-based version
check is the right root fix, it fails closed, and the empty-label guard is correct
(verified: `docker inspect --format '{{index .Config.Labels "missing"}}'` prints an empty
string with exit 0, so that guard is exactly what stands between "unlabelled image" and
"verifies anything"). Findings 2, 3, 4, 5, 6, 9 and 10 are resolved, and the
`curlimages/curl`-on-the-rollback-path dependency disappeared with the HTTP check.
What blocks is one **high**-severity regression that landed in the same file while this
re-review was being written — `deploy()`'s new `git clean -d --force` deletes
`.deployed-version` and `.failed-version`, because neither is gitignored, so the
failed-tag latch is wiped by the very rollback that writes it (verified in a scratch
repo) — plus one fix that does not work: **finding 7.** I ran it —
`PYTHONDONTWRITEBYTECODE=1 python3 -m py_compile tmp.ABCDEF.py` still writes
`__pycache__/tmp.ABCDEF.cpython-313.pyc`; that env var governs the import system, not
`py_compile`, whose entire job is to emit bytecode. The workspace litter the changelog
claims to have closed is still there for every `.py` file checked, and the e2e "leaves no
temp files" gate cannot see it (it only exercises a shell file and greps for
`tmp\.[A-Za-z0-9]+\.` in the file listing — a `__pycache__` *directory* never matches).
Behind that: the durable record now describes an implementation that no longer exists
(ADR-050 and the changelog both still say the updater verifies via `/api/version`, and the
changelog still says the ConfirmDialog bug was "fixed with a synchronous ref flag"), the
new health gate accepts `health=none` and never checks `.State.Running`, and
`docker compose up -d` never rebuilds the ingress, so a tagged release that changes
`deploy/caddy/**` ships everything except Caddy and still reports VERIFIED.

## Findings

### Regression introduced by the parallel security hardening

- **[high / confidence: high]** `deploy/update/styrman-update:175` — `git clean -d --force
  --quiet` inside `deploy()` **deletes the updater's own state files**. `cwd` is
  `${DEPLOY_DIR}`, and `.deployed-version` / `.failed-version` are written at the clone root
  as untracked files that no `.gitignore` covers (`git check-ignore` on both: no match), so
  `clean -d` without `-x` removes them. Verified in a scratch repo: both dotfiles are gone
  after one `git clean -d --force --quiet`. The damaging sequence is the rollback:
  `printf LATEST > "${FAILED}"` (line 246) → `deploy "${PREVIOUS}"` → `git clean` erases the
  latch → next timer tick sees no latch, retries the broken tag, rebuilds **both** images,
  fails, rolls back, forever. That is precisely the retry storm finding 5 was raised to
  close, reintroduced through a different door. Secondary damage: `.deployed-version` is
  deleted at the *start* of every deploy, so any early exit before the rewrite (the
  unbound-variable path below, or `fail` on a checkout) loses it, and the next run's
  `CURRENT` falls back to `git describe` on a tree that is now checked out at the **broken**
  tag — making it its own rollback target. The `git clean` itself is correct and worth
  keeping (a planted untracked file would otherwise survive into `context: ..`); the state
  files are what is in the wrong place. Fix: move them out of the work tree entirely —
  `/var/lib/styrman/update/{deployed,failed}-version` — which also matches the "the clone is
  a disposable checkout" model. If they must stay, `-e .deployed-version -e .failed-version`
  on the clean, and a comment saying why, is the minimum.

### Remediation that did not take

- **[medium / confidence: high]** `internal/httpapi/check.go:113` — the Python litter fix
  is inert. `PYTHONDONTWRITEBYTECODE=1 python3 -m py_compile "$F"` still writes
  `__pycache__/<tmpname>.cpython-3NN.pyc` next to the source, i.e. into the workspace root
  (`WorkDir=ws`); `sys.dont_write_bytecode` is consulted by the import machinery, never by
  `py_compile.compile()`. Reproduced here on Python 3.13.5: exit 0, `__pycache__/` created.
  The `trap` removes only `$F`, `${F}.err`, `${F}.out`, so the directory survives — under
  R13 every workspace is a git repo, so it also shows up in `git status` and the git panel.
  Two fixes, both verified clean in this session: `PYTHONPYCACHEPREFIX=/tmp/styrman-pyc`
  (smallest change, keeps `py_compile`'s error format so `rePyLine`/`rePyErr` still match),
  or `python3 -c 'import ast,sys;ast.parse(open(sys.argv[1]).read(), sys.argv[1])' "$F"`
  (note: its traceback carries extra `File "…ast.py", line 50` lines that `rePyLine` will
  also match, so prefer the env var).

- **[medium / confidence: high]** `.agents/docs/architecture/DECISIONS.md` (ADR-050) and
  `.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md:106` — the durable record
  documents the removed design. ADR-050 states the updater "rolls back to the previous tag
  if the newly deployed version fails to report itself healthy **via `/api/version`**" and
  that this path "was exercised … before shipping"; the changelog repeats it, and its §4
  still says the ConfirmDialog bug was "Fixed with a synchronous ref flag rather than
  state" — the ref was deleted in favour of `onCancel`. Neither mentions the
  `styrman.version` LABEL, which is the load-bearing part: **`deploy/Dockerfile.server`'s
  `LABEL` and `styrman-update`'s `reports_version` are now a cross-file contract, and
  deleting the LABEL rolls back every future release.** That coupling exists only as a
  comment in two files. ADR-048 already established the right shape for this — add the same
  "**Correction (same-day review remediation)**" block to ADR-050, and rewrite the
  changelog's updater and §4 paragraphs plus its Verification section to cover the
  re-verification actually performed (label decision table, `TestSafeSuffixBlocksInjection`,
  `shellcheck`, e2e 47/0/2).

### New problems introduced or exposed by the fixes

- **[medium / confidence: high]** `deploy/update/styrman-update:205-217` — the new gate is
  "container named `styrman-server` carries label X **and** health is `healthy` *or*
  `none`", and `.State.Running` is never consulted. Today `deploy/Dockerfile.server:28`
  defines a `HEALTHCHECK`, so `.State.Health` is non-nil and the `none` arm is unreachable —
  but that arm is a fail-open branch whose only guard is a line in a *different* file, in
  the *deployed tag's* Dockerfile. Remove or break that HEALTHCHECK in some future release
  and the updater blesses it on the label alone, which proves the image was built, not that
  the server runs. Fix: add `{{.State.Running}}` to the inspect and require `true`, and make
  `none` a logged failure ("the deployed image defines no healthcheck — cannot verify")
  rather than a pass. Cheap, and it restores the property the HTTP check had.

- **[medium / confidence: high]** `deploy/update/styrman-update:182-184` — the updater
  rebuilds the runtime image and the server, then runs `docker compose up -d`, which does
  **not** rebuild an image that already exists. `styrman-caddy:latest` has a `build:` stanza
  pointing at in-repo sources (`deploy/caddy/`), so a tagged release that changes the
  vendored ingress deploys everything *except* the ingress — and `reports_version` still
  reports VERIFIED, because the server's label is correct. The header comment says "Both
  images are rebuilt on every tag (operator's choice): the server, and the runtime image",
  which reads as a deliberate list but silently excludes a third image that lives in the
  same repo and moves with the same tags. Either build caddy too
  (`docker compose build styrman caddy`) or state explicitly, in the script and in ADR-050,
  that ingress changes require a manual rebuild.

- **[low / confidence: high]** `deploy/Dockerfile.server:21-23` — `ARG STYRMAN_VERSION` +
  `LABEL` are placed **before** `RUN apt-get update && install …`, so every new version
  value is a cache miss for the apt layer. Verified locally with BuildKit: with `LABEL`
  before the `RUN`, changing the build-arg re-ran it; with `LABEL` after, the same change
  was `CACHED`. Consequence beyond slowness: every release now depends on Debian mirrors
  during an unattended build, and a transient apt failure makes `deploy()` return 1 →
  rollback → the tag is latched as FAILED and needs `--force`. Move the `ARG`/`LABEL` pair
  to the end of the final stage (after `COPY`/`HEALTHCHECK`); the label lands on the image
  either way.

- **[low / confidence: high]** `deploy/update/styrman-update:201,221` — `local want="$1"
  deadline got health` leaves `got`/`health` **unset**, and line 221 reads `"${got}"` under
  `set -u`. If the `while` body never executes (`STYRMAN_UPDATE_HEALTH_TIMEOUT=0`, or a
  backwards clock step), bash aborts with `got: unbound variable` — confirmed in a scratch
  shell — and the script dies *after* deploying but *before* the `.failed-version` latch at
  line 200, reinstating exactly the retry storm fix #5 closed. One-word fix:
  `local want="$1" deadline got="" health=""`.

- **[low / confidence: high]** `internal/httpapi/check.go:53-63` — the orphaned doc comment
  is not fixed, it moved. The three-line `// checkerFor maps a file extension to its
  checker…` paragraph now sits directly above `// safeSuffix reduces…` with no blank line,
  so godoc renders one merged comment on `safeSuffix` beginning "checkerFor maps…". Put the
  paragraph back on `checkerFor`.

- **[low / confidence: medium]** `internal/httpapi/check.go:298-306` + `check_test.go:164` —
  `missingTool` rejects output with `strings.Count(t,"\n") > 1`, i.e. it still accepts a
  **two-line** output, while its own comment and the remediation note say "a single-line
  output". A two-line tool message whose last line ends in `no such file or directory` would
  still have its diagnostics discarded. More importantly `TestMissingTool` does not cover
  the regression this fix exists for: add the motivating case (a real diagnostic containing
  "not found", e.g. `a.py:3:1: error: module not found`) and a multi-line case. Also
  `A || B && C` on lines 303-305 relies on precedence to mean `A || (B && C)`; parenthesise.

- **[low / confidence: high]** `internal/httpapi/check_test.go:173-184` — hand-rolled
  `contains`/`indexOf` in a file that already imports `strings`. Delete both and call
  `strings.Contains` (criterion 5, stdlib-first).

- **[low / confidence: high]** `web/e2e/run.mjs:320-327` — the litter gate is still
  shell-only and still matches on `tmp\.[A-Za-z0-9]+\.` in the file listing, so it cannot
  detect a `__pycache__` **directory**. This is precisely why the broken Python fix passed
  the batch's own verification. Extend it: create a `.py` file, open it, wait for the
  debounced check, and assert the listing contains neither `tmp.` nor `__pycache__`.

### Carried over, still open (all previously recorded as low, none re-argued)

- **[low / confidence: high]** `deploy/update/styrman-update:170` — `deploy()` still calls
  `fail` (which exits) on a checkout failure while build failures `return 1`. Fix #5 is
  therefore complete only for the build and health paths: a failed checkout still exits
  before the latch at line 200, and a *successful* rollback still exits through `fail`, so
  the one path that worked prints `FATAL:`. Make `deploy()` `return 1` throughout.
- **[low / confidence: high]** `deploy/update/styrman-update.timer:22` — `Persistent=true`
  is inert on a monotonic (`OnBootSec`/`OnUnitActiveSec`) timer.
- **[low / confidence: high]** `deploy/host-setup.sh` — step **8c is still physically before
  step 8b**.
- **[medium / confidence: medium]** `internal/httpapi/{check.go:404, format.go:120}` —
  `mktemp -p .` still lands in the **workspace root** on every 1 s debounce tick. Unchanged
  since the first review; the `$ws/.styrman/tmp/` recommendation stands (config discovery
  walks up, Go tooling ignores dot-dirs, `.git/info/exclude` keeps `git status` clean).
- **[low / confidence: high]** `internal/httpapi/check.go:404-411`, `format.go:120-126` —
  dead trap globs (`${F}.err` in check, `${F}.out` in format) and a trailing `rm -f` the
  `EXIT` trap already covers.
- **[low / confidence: high]** `internal/httpapi/check.go:121` — the Terraform checker still
  reports `STYRMAN_TOOL gcc` to the UI, and `tofu fmt -check` is still a formatting check
  sold as a syntax check.
- **[low / confidence: high]** `web/src/lib/api.ts` / `CodeEditor.tsx:219` — `checkFile`
  still takes no `AbortSignal`; `lintGutter()`/`linter()` are still added unconditionally.
- **[low / confidence: medium]** `image/Dockerfile:48` — `hadolint` still unpinned
  (`releases/latest`, no checksum) while the changelog names a version. Consistent with the
  pre-existing `yq`/`go` lines, so consistency, not regression.
- **[low / confidence: high]** `web/src/components/EditorPane.tsx:171-178` — the close-file
  dialog's "Discard" button still discards nothing.
- **[low / confidence: high]** `AGENTS.md:98`, `.agents/docs/testing/styrman/README.md:49,75`
  — still "45 checks / 43 + 2 auto-SKIP" while the changelog says 47/0/2 and `run.mjs` has
  48 `await check(` calls. The README declares itself the source of truth and is the one
  that is wrong. Three numbers, three values, none reconciled.

## What is genuinely fixed

1. **Rollback of every healthy release** — fixed at the root. `reports_version` reads
   `docker inspect --format '{{index .Config.Labels "styrman.version"}}'` plus health; no
   HTTP, no auth, no ingress token, no widened auth surface. Declining to exempt
   `/api/version` from `authGuard` was the right call.
2. **Fail-open on a missing token** — gone with the token. `[ -n "${got}" ] && [ "${got}" =
   "${want}" ]` fails closed, and the guard is load-bearing: I confirmed a missing label key
   yields an empty string, not an error.
3. **Root SSH clone** — `git fetch` failure is now `fail` with an actionable message,
   `GIT_SSH_COMMAND` uses `/etc/styrman/deploy-key` in both scripts, the "not enabled"
   heredoc is a complete copy-pasteable runbook, and the timer enables only when the clone
   **and** its `.env` exist. `${ORIGIN:-<origin>}` is correctly defaulted for the branch
   where `ORIGIN` was never assigned.
4. **`/root/.gitconfig` growth** — the `git() { command git -c safe.directory=… "$@"; }`
   wrapper is correct. `command` bypasses function lookup so there is no recursion (tested),
   `"$@"` is properly quoted, and `git -c` composes fine with the later `--no-pager`.
   Nothing global is mutated.
5. **First-run retry storm** — the latch is written at line 246, before the
   `PREVIOUS = none` branch at 247. Correct as written — but neutered in practice by the `git clean` regression above,
   which deletes the latch during the rollback deploy, plus the two escape hatches noted
   (`fail` on checkout, the unbound-variable path).
6. **Shell injection via `suffix`** — `safeSuffix` is correct (alnum only, ≤16, `txt`
   fallback) and applied on both paths. `TestSafeSuffixBlocksInjection` is a good test: it
   asserts the character class, the end-to-end script shape for `Dockerfile.$(id)`, and
   pass-through of ordinary extensions.
8. **Failed check rendering as clean** — `missingTool` no longer substring-matches, and
   `FileEditor.runCheck`'s catch now sets a visible note and distinguishes the
   "start the project" 409. The code and its comment finally agree.
9. **Short read** — both scripts assert `[ "$(wc -c < "$F")" -eq N ]` and emit
   `STYRMAN_ERR`; the format path surfaces it as a 422 toast, so a truncated document can no
   longer reach `setContent`.
10. **`ConfirmDialog.onCancel`** — correct and non-breaking. `onCancel?.()` runs before
    `onClose()` on the Cancel button and on `Dialog`'s `onClose` (backdrop/Escape), never
    after confirm. I re-checked all five other callers (`FileExplorer.tsx`,
    `SidePanelGit.tsx`, `TerminalList.tsx` ×2, `SetupBanner.tsx`): none passes `onCancel`,
    so their behaviour is byte-identical. The one thing worth naming, because it is not
    obvious: `Dialog` now receives a **fresh closure every render**, which would have
    re-run the a11y mount effect (stealing focus, re-stacking the modal) — it is safe only
    because `useModalA11y` routes `onClose` through `onCloseRef` with an empty-dep effect
    (`ui.tsx:100-110`). That guard is what makes this change free; it should not be removed.
    `EditorPane`'s `discardingRef` is gone.

## Criteria

1. **Dead code — pass with nits.** Nothing orphaned by the remediation; the removed
   `discardingRef` and the entire token/`curlimages` path are gone cleanly. Remaining:
   hand-rolled `contains`/`indexOf` in `check_test.go`, the trap globs for files never
   created, and the trailing `rm -f` the trap already covers.
2. **Flow tracing — pass with one gap.** The seam that broke last time is now traced
   end to end and is sound: `Dockerfile.server` LABEL → image → container `Config.Labels` →
   `docker inspect` → `reports_version` → deploy/rollback. Verified the missing-key and
   empty-label behaviours directly. The untraced hop is `docker compose up -d` → the
   `caddy` service, which is never rebuilt, and the `deploy()`→`fail` path that bypasses the
   new latch. Go and frontend flows re-trace clean: `checkScript`/`formatScript` both honour
   the exact-byte-count contract, the STYRMAN_ERR short-read path propagates to a 500/422
   and then to a user-visible note, and `onCancel`/`onConfirm`/`onClose` ordering is
   consistent at every call site.
3. **Standards conformance — pass.** `gofmt -l`, `go build ./...`, `go vet ./...` and
   `go test ./internal/{httpapi,conversation,orchestrator}` all re-run clean here.
   `shellcheck` on `deploy/update/styrman-update` is clean at **default** severity, not just
   `-S error` (the only hit in `deploy/` is a pre-existing SC2015 info at
   `host-setup.sh:260`, untouched by this batch). Go nits: the doc comment now attached to
   the wrong function, the unparenthesised `||`/`&&`, hand-rolled `strings.Contains`. TS
   nits unchanged: `(e as Error)` casts instead of narrowing `unknown`, no `AbortSignal`.
4. **Directory layout — pass.** No files moved; `deploy/update/` still mirrors
   `deploy/backup/`, tests still sit beside their code. Only wart remains step 8c above 8b.
5. **Simplicity (DIY-except-security) — pass, improved.** The remediation *removed* a
   dependency: `curlimages/curl` is no longer pulled from Docker Hub inside the rollback
   decision, and the ingress token no longer needs parsing out of `.env`. Reading a build
   label with the Docker CLI already required by the script is the simplest correct answer.
   No hand-rolled auth, crypto, hashing, sessions, or JWT/OIDC anywhere; declining to widen
   `authGuard` for `/api/version` kept the auth surface unchanged. The one remaining
   stdlib-first miss is the test-local `contains`/`indexOf`.
6. **Decoupling — pass, improved.** `ConfirmDialog`'s optional `onCancel` removes the
   invisible call-ordering contract that `EditorPane` was reaching into; the seam is now
   explicit in the type. Counter-movement: the updater gained a new *implicit* contract with
   `deploy/Dockerfile.server`'s `LABEL` that is recorded only in code comments and in
   neither the ADR nor the changelog — that is the coupling most likely to be broken by
   someone who has not read either file.

## Assessment of the verification run

`go build`/`vet`/`test -race`, the ingress module, `gofmt -l`, `tsc -b`, `oxlint`,
`shellcheck -S error` on all three scripts, e2e 47/0/2, and the label decision table on a
real container (VERIFIED / REFUSED ×3) are the right set, and the label table in particular
is exactly the evidence the previous review said was missing. Two gaps remain, and they are
the same shape as last time — the automated gate does not cover the thing that broke:

- The **Python litter fix was never exercised**. The e2e litter check still runs only a
  shell file and matches only a `tmp.*` filename pattern, so it cannot fail on
  `__pycache__/`. One 20-second manual `py_compile` run in a scratch directory falsifies the
  fix; the fix should not be re-declared done until the e2e check covers `.py`.
- **No verification that the deployed stack matches the tag as a whole** — the caddy image
  is never rebuilt, and the label check cannot see that, because it only inspects the
  server. A "deploy tag → assert all three images carry the tag's content" step would close
  both this and the label-contract coupling.
