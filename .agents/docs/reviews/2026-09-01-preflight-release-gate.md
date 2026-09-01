# Review — PREFLIGHT release gate, v1.15.0..6b41979   (2026-09-01)

Scope: the full release diff `git diff v1.15.0..HEAD` — one commit, 85 files,
+5285/-478. Prior artifacts read first and NOT re-argued: the three review rounds
(`2026-09-01-review-editor-pane-and-ordering.md`,
`…-review-syntax-check-and-autoupdate{,-rereview,-final}.md`), the security pass
(`…-security-editor-check-and-autoupdate.md`), and the three verifier logs.
Effort was concentrated on the delta made AFTER the final APPROVE, which no
reviewer had seen: the `EditorPane` `key`, the `ssh-keyscan`/`sed` known_hosts
seeding in `deploy/host-setup.sh`, the `image/Dockerfile` comment, the
`contains` removal in `check_test.go`, the e2e count reconciliation, the ADR-050
footer merge, and changelog §5. `image/harness/**` out of scope (ADR-046,
unchanged here). `web/dist/**` inspected for commit coherence only.

Gates re-run by me on the committed tree, not taken on report:
`gofmt -l internal cmd` clean · `go vet ./...` clean · `go build ./...` clean ·
`go test ./...` all pass · `(cd deploy/caddy/ingress && go test ./...)` ok ·
`npx tsc -b` exit 0 · `shellcheck -S warning deploy/host-setup.sh
deploy/update/styrman-update` clean.

## Verdict: APPROVE WITH NITS — nothing found is SHIP-blocking

The worst thing I found is not in the shipped runtime: it is that the
`known_hosts` remediation added after the final APPROVE does not deliver the
property its own comment claims, and is not idempotent. `deploy/host-setup.sh`
seeds `/etc/styrman/known_hosts` with an **unverified `ssh-keyscan`** — the
exact mechanism the security review explicitly rejected ("populated from
GitHub's published SSH key fingerprints, verified against
`https://api.github.com/meta`, **not** from an unverified `ssh-keyscan`") —
while `deploy/update/styrman-update:88-91` asserts that its
`StrictHostKeyChecking=yes` closes the trust-on-first-use window. It does not
close it; it relocates it to setup time. Worse, the seeding sits inside the
`if [ ! -d "${STYRMAN_DEPLOY_DIR}/.git" ]` block, so on any host where the
deploy clone already exists the file is never created — and the updater's own
fetch-failure message still blames only the deploy key, which is the second
half of the review finding that prompted this change and was not done. None of
this blocks the tag: the updater ships DISABLED, auto-enable additionally
requires the operator to hand-place `deploy/.env` in the clone, and the control
that actually matters — signed-tag verification, failing closed on both
lightweight and unsigned tags — is present and correct.

Second: the changelog and a load-bearing code comment both claim the
`EditorPane` `key` fixed the unsaved-edit data loss on a terminal switch. It
did not. The key fixes state crosstalk; the buffer still dies with the unmount.
`TODO.md:80` states this correctly, so the project record is not wholly wrong —
but the append-only changelog is the record of what shipped, and it overstates.

Everything the previous rounds flagged as must-fix-before-commit checks out:
`web/dist` is fully coherent (every asset `_shell.html` references is tracked
and present; I crawled the chunk graph — 45 assets, zero orphans, zero dangling
references, `git status --porcelain web/dist` empty). ADR-050 has exactly one
footer. `check_test.go`'s `contains` alias is gone. The `image/Dockerfile`
contradictory comment is gone and hadolint is checksum-pinned. e2e counts
reconcile: `run.mjs` has 49 `await check(` calls, +1 client-error gate = 50 =
48 pass + 2 auto-SKIP, matching AGENTS.md and the testing README.
`deploy/Dockerfile.server`'s `ARG`/`LABEL` contract is wired end-to-end
(`docker-compose.yml:68` passes `STYRMAN_VERSION`), so `reports_version()` will
actually match — the failure mode that would have rolled back every healthy
release.

## Findings

- **[medium / confidence: high]** `deploy/host-setup.sh:339-343` and
  `deploy/update/styrman-update:88-91` — the "pinned" host key is learned over
  an unauthenticated channel. `ssh-keyscan "${host}" > /etc/styrman/known_hosts`
  is trust-on-first-use by definition; the updater then treats that file as an
  authority with `StrictHostKeyChecking=yes`. Net effect: the TOFU window moved
  from the updater's first root fetch to host-setup's keyscan — it was not
  eliminated. The updater's comment ("a first-contact TOFU window would let an
  attacker pin their own host key permanently (security review). host-setup.sh
  seeds /etc/styrman/known_hosts via ssh-keyscan") is self-contradictory and
  should not stand as written: a comment that claims a security property the
  code does not have is how the next engineer stops checking. Concrete fix:
  seed from GitHub's published fingerprints — fetch `https://api.github.com/meta`
  over TLS and write `.ssh_keys` entries, or have the operator paste them — and
  either way correct the comment to state which channel the trust came from.
  Not ship-blocking: the timer ships disabled, and the signed-tag check is the
  control that bounds the blast radius.

- **[medium / confidence: high]** `deploy/host-setup.sh:336-350` — the seeding
  is **not idempotent**, contradicting the script's own header ("Idempotent:
  safe to re-run; every step checks before it changes anything"). It is nested
  inside `if [ ! -d "${STYRMAN_DEPLOY_DIR}/.git" ]`, so a host whose deploy
  clone already exists (any host that ran step 8c before this commit) can never
  obtain `/etc/styrman/known_hosts` by re-running host-setup. The updater then
  dies on **every** tick in `git fetch`, and `styrman-update:103` still says
  only "install a read-only deploy key at /etc/styrman/deploy-key" — the
  misleading-message half of the original review finding was never applied. Fix:
  hoist the `ORIGIN`/keyscan seeding out of the clone-exists guard into its own
  `if [ ! -s /etc/styrman/known_hosts ]` step, and append "or
  /etc/styrman/known_hosts is missing/stale" to the fetch-failure message. (Not
  live on this host today — `/opt/styrman` and `/etc/styrman` do not exist and
  the timer is `not-found` — so this is a latent upgrade-path defect, not a
  current outage.)

- **[medium / confidence: high]** `deploy/host-setup.sh:346` — the auto-clone
  still passes `StrictHostKeyChecking=accept-new` while pointing
  `UserKnownHostsFile` at the file the updater later trusts strictly. The
  keyscan's failure is swallowed twice (`2>/dev/null` and `|| true`) and the
  redirection truncates the file **before** `ssh-keyscan` runs, so a failed scan
  leaves an empty file and the clone then TOFU-appends whatever key it is
  offered *into the pinned file*. That is a write path into the updater's trust
  anchor from an unauthenticated clone. Fix: use `StrictHostKeyChecking=yes` in
  the auto-clone too and refuse to clone when `[ ! -s /etc/styrman/known_hosts ]`.

- **[medium / confidence: high]**
  `.agents/docs/changes/2026-09-01-syntax-check-and-autoupdate.md` §5 and
  `web/src/routes/p.$project.$term.tsx:146-152` — both state that without the
  `key` "the previous terminal's unsaved edits were wiped with no prompt",
  implying the key fixed it. Traced: with the key, a terminal switch **unmounts**
  `EditorPane`, which destroys `loaded` and `useFileEditorState`'s `text`; on
  return the store's `file` is re-fetched from disk and the publish effect sets
  `dirty:false`. The edits are still silently lost — the mechanism changed, the
  outcome did not. What the key genuinely fixed is real and worth keeping (no
  cross-terminal buffer reuse, no `setPending` confirm popping up on the wrong
  terminal, no wrong-key `setDirty`). `TODO.md:80-88` describes this accurately;
  the changelog and the comment do not. Fix (two-line docs edit, worth doing
  before the tag): reword both to "fixed the state crosstalk; the underlying
  loss is still open — TODO.md".

- **[low / confidence: high]** `web/src/components/TerminalPane.tsx:71-90` — the
  final review's low (four identical `{"type":"resize"}` frames per
  Editor→Terminal toggle, each a full tmux resize + TUI redraw in the microVM)
  was neither fixed nor recorded in `TODO.md`, unlike every other carried-over
  low. Either track the last-sent `cols`/`rows` in the effect scope and skip
  unchanged sends, or add the row to `TODO.md` so it is not lost.

- **[low / confidence: high]** `deploy/host-setup.sh`, step 8c heredoc — two
  operator-facing text defects: prerequisite 2 reads "That clone's OWN deploy/.env
  (gitignored, so a fresh clone has none) (gitignored, so a fresh clone has
  none)" — the parenthetical is duplicated verbatim; and "Three prerequisites,
  **all root-only**" is followed by item 3 ("SIGN your release tags"), which is
  neither root-only nor performed on this host. Also inconsistent: the printed
  clone command uses `${ORIGIN:-<origin>}` but the printed keyscan hardcodes
  `github.com`.

- **[low / confidence: medium]** `deploy/host-setup.sh:341` — the sed host
  extraction `s/^[^@]*@\([^:/]*\).*/\1/p` drops a non-default SSH port. For
  `ssh://git@host:2222/owner/repo.git` it yields `host`, so `ssh-keyscan` scans
  port 22 and writes a `host` entry while ssh looks for `[host]:2222` — the
  updater's strict check then fails permanently. Correct for the
  `git@github.com:owner/repo.git` and `https://…@host/…` shapes I traced, and
  correctly a no-op (guarded by `[ -n "${host}" ]`) for a plain `https://` origin.
  One-line fix if `ssh://` origins are to be supported; otherwise say so.

- **[low / confidence: medium]** `REQUIREMENTS.md` R4 vs. the shipped editor.
  R4 reads "Terminal-first, no embedded IDE … A lightweight file viewer is the
  ceiling." What shipped is a co-equal main-pane view with a `Terminal | Editor`
  toggle, per-terminal state, a real-time lint gutter, and eight CLI checkers
  executing in the guest. It is emphatically not code-server, the terminal
  remains the default view and is never unmounted, and ADR-048 records that the
  operator asked for it — so I am **not** calling this a blocking requirements
  violation. But the requirement text now disagrees with the product, and the
  next increment (autocomplete, LSP, multi-file tabs) has no line to stop at.
  Only the operator may amend `REQUIREMENTS.md`; the cheap alternative is one
  paragraph in ADR-048 stating where the R4 ceiling now sits.

- **[low / confidence: medium]** `web/src/lib/editorPane.ts:79-98` —
  `closeFileInPane(project, path)` matches on path only and ignores `scope`, so
  deleting home-scope `~/.bashrc` also closes a project-scope `.bashrc` open in
  a pane (and vice versa). It errs in the safe direction (closing too much
  cannot resurrect a deleted file), so this is a correctness nit, not a hazard.
  Include `scope.kind` in the match.

- **[low / confidence: low]** `deploy/update/styrman-update.service` —
  `ProtectHome=yes` hides `/root`, so the docker CLI cannot read
  `/root/.docker/{config.json,contexts}`. Harmless in the current deployment
  (local socket, local builds, no registry auth) but it means the updater's
  `docker build`/`docker compose` can silently use a different context or
  credential set than the same command run by hand as root. Worth one sentence
  in `deploy.md` rather than a code change.

- **[low / confidence: high]** `.agents/docs/testing/styrman/README.md:79-80` —
  "49 checks + the client-error gate (48 pass on firewall-hardened hosts, plus 2
  auto-SKIP) plus a final no-client-errors gate" counts the gate twice in one
  sentence. The arithmetic is right (49 + 1 = 50 = 48 + 2, and `run.mjs` really
  does have 49 `await check(` calls); only the prose is doubled.

- **[low / confidence: high]** `web/src/components/CodeEditor.tsx:262-265` —
  pre-existing, not a regression, recorded for completeness: `lang?.load().then(…)`
  is a floating promise with no `catch` and no destroyed-view guard, so a failed
  lazy grammar import becomes an unhandled rejection and a resolve after unmount
  dispatches into a torn-down `EditorView`. The line was only touched to pass
  `base` instead of `filename`. `void`+`.catch` and a `viewRef.current === view`
  guard close it.

## Carried over, verified still open, and correctly recorded

All of the following are in `TODO.md`, so I am not re-filing them: `deploy()`
`fail`s (exits) on a checkout error instead of `return 1`, bypassing the
failed-tag latch on that one path; `Persistent=true` is inert on a monotonic
timer; step 8c is physically ordered before step 8b; `mktemp -p .` writes into
the workspace root; the Terraform checker reports `STYRMAN_TOOL gcc`; the dead
`${F}.out`/`${F}.err` trap globs; `checkFile` takes no `AbortSignal`; the lint
gutter mounts unconditionally; the close-file "Discard" button discards nothing;
and the two `docker/docker@v28.5.2+incompatible` govulncheck hits with no fixed
version on the `+incompatible` line.

## Criteria

1. **Dead code — pass.** No unused Go symbols in the changed packages: I
   enumerated every new/changed identifier (`firstLines`, `firstLine`,
   `hasShebang`, `safeSuffix`, `checkScript`, `formatScript`, `parseCheckOutput`,
   `missingTool`, `checkerFor`, `naturalLess`, `isDigit`) and each has a
   non-test caller. The `contains` alias is gone. Every new web export
   (`EditorPane`, `openFileInPane`, `closeFileInPane`, `notifyFileSaved`,
   `useFileSavedTick`, `useEditorPane`, `FileEditorBody`, `FileEditorToolbar`,
   `useFileEditorState`, `CheckResult`, `CheckDiagnostic`) has a consumer; the
   382-line modal viewer is deleted with no dangling reference. The
   contradictory hadolint comment is removed. Build output is clean too: I
   crawled `_shell.html` → chunk graph and found zero unreachable assets among
   the 45 tracked files (the three that look orphaned — `dockerfile-*`,
   `shell-*`, `toml-*` — are dynamic imports from `CodeEditor-DEY06m8V.js`; the
   woff2 set is referenced from CSS).
2. **Flow tracing — pass, with one broken flow already documented.** Traced
   `Exec` → `CloseWrite` removal → both guest scripts: exactly two `Stdin:`
   callers exist (`format.go:97`, `check.go:363`) and both now read
   `head -c <n>` with a `wc -c` equality assertion, so the contract holds at
   every hop; the stdin goroutine cannot leak because `defer att.Close()`
   unblocks a stalled `Write`. Traced `STYRMAN_TOOL`/`STYRMAN_OUT` framing
   through `checkScript` → `strings.Cut` → `parseCheckOutput` — the tool banner
   is inside the redirected block, so the cut order is right. Traced
   `EditorPane`'s new `key` through mount/unmount, the store's `set()` no-op
   bail, and `useSyncExternalStore` — no render loop, and the crosstalk is
   genuinely gone; the surviving buffer loss is finding 4. Traced
   `Dockerfile.server` `ARG`/`LABEL` → compose `args` → `reports_version()` —
   intact. Error paths: `runCheck`'s catch cannot present a failed check as a
   clean file, and Go errors in the changed handlers are all wrapped or
   surfaced.
3. **Standards conformance — pass.** All gates re-run by me on the committed
   tree (listed above) are clean, including the ingress module's separate
   `go test`. Go is stdlib-only in the diff; the one new npm dependency
   (`@codemirror/lint`) is first-party CodeMirror and justified in the changelog.
   `internal/conversation/store.go` deliberately hand-rolls natural sort rather
   than pulling `x/text/collate` and documents the stopping point — the right
   stdlib-first call, and I checked `naturalLess` is a consistent ordering
   (digit-run equivalence falls through to the ID tie-break, which is exactly
   why the two-way comparison in `List` is there).
4. **Directory layout — pass.** Updater under `deploy/update/`, state under
   `/var/lib/styrman/update/` (outside the disposable checkout), Go tests beside
   their code, frontend entirely under `web/`, new ADRs appended not rewritten.
5. **Simplicity / DIY-except-security — pass.** Nothing security-sensitive is
   hand-rolled: tag trust is `git verify-tag`, host-key trust is OpenSSH,
   randomness is `openssl rand`, the ingress token comparison is untouched
   `crypto/subtle`. hadolint is now checksum-pinned, which also makes the
   tag→image mapping deterministic for rollback. The one substantive complaint
   is finding 1 — not hand-rolled crypto, but a trust anchor sourced from an
   unauthenticated channel while claiming otherwise.
6. **Decoupling — pass.** The pane store is a genuine seam: the explorer and the
   editor pane never reference each other, and `notifyFileSaved` /
   `closeFileInPane` / `useFileSavedTick` are the entire coupling surface.
   `onResizeRef` keeps the fit addon inside `TerminalPane`. `FileEditor.tsx`
   holds the shared editor logic once, with `EditorPane` as the only wrapper.
   The one cross-file, shell-level contract that cannot be type-checked
   (`Dockerfile.server` LABEL ↔ `reports_version()`) is documented on both sides
   and in ADR-050.
