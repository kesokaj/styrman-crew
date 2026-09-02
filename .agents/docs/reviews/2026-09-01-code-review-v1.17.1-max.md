# /code-review max — commit 36210ff (v1.17.1) — 2026-09-01

Vendor built-in `/code-review` at effort `max` (multi-agent: 10 finder angles +
a sweep, every surviving candidate adversarially verified; two claims were
partially refuted and rewritten, none refuted outright). Target: `HEAD~1..HEAD`
(clean tree, HEAD == origin/main). Ranked correctness-first. Nothing applied —
this is the record; fixes are a separate change set.

## Findings (all CONFIRMED)

1. **[data corruption] `web/src/components/FileEditor.tsx:123`** — the
   render-phase reset is keyed on `file.path` alone. Opening the same-named
   file in the OTHER scope (one click via a Recent chip) keeps the previous
   scope's buffer; Ctrl+S has no dirty guard → the other scope's file is
   silently overwritten (no backups, ADR-055). Key the reset on
   `scope.kind + path` (or key the editor on both).
2. **[data loss, pre-existing] `FileExplorer.tsx:135`** — "New file" with an
   existing name truncates it (PUT empty → O_CREATE+Truncate), no existence
   check; after this diff the truncated file also lands as the newest Recent
   chip. Reject existing names in the newFile validator and/or add a
   create-only endpoint.
3. **[unsaved-edit loss] `web/src/lib/editorPane.ts:196`** — `closeFileInPane`
   prunes recents scope-aware but the pane sweep above still matches by path
   only: deleting project:`p` clears a pane holding home:`p` and discards its
   unsaved edits without a prompt. One `matches(file, {scopeKind,path})`
   predicate for both loops, scopeKind required.
4. **[unsaved-edit loss, pre-existing] `p.$project.$term.tsx:169`** —
   EditorPane is keyed per terminal; a sidebar click to another terminal
   unmounts it and destroys the buffer with no prompt, while the new badge/dot
   advertise "parked edits". Needs a discard prompt (or buffer-in-store).
5. **[file resurrection] `EditorPane.tsx:76`** — when `readFile` rejects (stale
   Recent chip, guest-side rm) the store keeps pointing at the requested path
   while `loaded` keeps the previous file editable; `closeFileInPane` no longer
   recognises the on-screen file and a later Save recreates a deleted file.
   Revert the store to `loaded.req` on failure (or publish the loaded file).
6. **[stale overwrite, pre-existing] `FileExplorer.tsx:220` (+ drop path :301)**
   — uploads never call `closeIfOpen`; uploading over the open file leaves the
   stale buffer and the next Save overwrites the upload.
7. **[ops] `deploy/update/styrman-update.service:15`** — `DOCKER_CONFIG`
   hard-codes `/etc/styrman/update` a second time (systemd `Environment=` does
   no `$VAR` expansion) under the comment calling line 10 the ONLY pin
   (ADR-052); a hand-run `sudo … styrman-update --force` builds with root's
   `/root/.docker` (env_reset strips it) while the timer builds with an empty
   config — with a docker-container buildx builder the runtime image can land
   only in the build cache while `reports_version` still passes. Also the
   comment misdescribes `ProtectHome=yes` ("read-only"; it is inaccessible +
   empty). Fix: `export DOCKER_CONFIG="${STATE_DIR}/.docker"` in the script
   after the guard; drop the unit line.
8. **[test theatre] `web/e2e/run.mjs:304`** — the "recent strip reopens a file"
   check is vacuous: `.cm-content >> text=hi` is a lax substring match that the
   still-open Dockerfile buffer (`RUN echo hi`) already satisfies, and the
   chip's aria-current derives from the store written synchronously before any
   fetch. Assert the reopen (toolbar path `subdir/nested.txt`, or `FROM debian`
   detached); use an exact `getByLabel('Open recent file subdir/nested.txt')`.
9. **[security surface] `styrman-update.service:15`** — pointing
   `DOCKER_CONFIG` into the trust-asserted state dir adds root-executed inputs
   (`config.json` credsStore/credHelpers/cliPluginsExtraDirs/proxies,
   `cli-plugins/`) the `-maxdepth 0` owner/mode assertions never cover, and
   buildx localstate accretes under /etc with no GC (9 files after two
   deploys). Fix: `RuntimeDirectory=styrman-update`,
   `RuntimeDirectoryMode=0700`, `Environment=DOCKER_CONFIG=%t/styrman-update/docker`
   (tmpfs, wiped after the oneshot; `%t` verified to expand in `Environment=`).
10. **[validation] `editorPane.ts:128`** — the recents validator runs only on
    read and partially: `noteRecent` persists unvalidated (a >512-char path
    shows this session and vanishes after reload), non-canonical paths
    (`subdir/`, `./a.txt`, `a//b`) pass, the type predicate over `any[]`
    narrows nothing, foreign properties survive. One
    `sanitizeRecents(unknown): RecentFile[]` (canonicalise, first-wins dedup,
    cap) used by both paths.
11. **[UX] `FileExplorer.tsx:265`** — chips labelled by bare basename (`~/`
    glued onto the basename for home scope): same-basename files in different
    dirs render identical chips; a nested home file shows a `~/<basename>` path
    that does not exist. Disambiguate on collision (append parent dir), prefix
    `~/` to the path.
12. **[a11y contrast] `p.$project.$term.tsx:126`, `FileExplorer.tsx:334/368`**
    — the inactive-toggle badge (text-dim + opacity-75) measures 3.51:1 light;
    the highlighted row keeps text-dim size label/icon on bg-accent-soft
    (3.82:1 dark). Drop opacity-75; switch the row's descendants to
    text-on-accent-soft.
13. **[a11y] `p.$project.$term.tsx:110`** — the aria-label pin makes the badge
    and the unsaved dot AT-invisible (accname 2C returns before descendants).
    Keep the label; add `aria-describedby` to a child span carrying the
    basename + ", unsaved changes" when dirty (exact-name locators stay valid).
14. **[process — Definition of Done] `changes/2026-09-01-editor-ux-and-updater-fixes.md:179`**
    — the trail for HEAD does not satisfy the DoD: both `testing/` logs read
    FAIL (round 2: passed=48) and the final 51/0/2 run exists only inline in
    the changelog (self-verified, no round-3 verifier log); the
    `styrman-update.service` change has no review/verification artifact (round
    1's log even asserts no `deploy/` files are in the diff); the new prune
    branch and validator have zero covering tests (mutants that leave the suite
    at 51/0/2: flip `!==` to `===` at `editorPane.ts:213`; delete
    `.slice(0, RECENTS_CAP)` at :143) while the changelog claims a "both chips"
    assertion the e2e does not make; `guide.md:86` says "the last 5 of up to 8"
    while all 8 render; the prior security review's premise "the docker CLI
    needs neither setuid nor /home" is falsified by this fix with no security
    findings file. Rules broken verbatim: verifier log before any completion
    claim; never self-verify; changed code without covering tests = FAIL;
    never restate a failure as success.
15. **[UX] `FileExplorer.tsx:233`** — the Recent strip mounts ABOVE the listing
    in the same commit as the first file open, shifting the listing 33.5px
    (0.93 of a row) under a stationary pointer: a double-click's second
    mousedown hits the row that was above (verified by hit-testing the built
    CSS in Chromium; invisible to e2e, which re-resolves per click). Reserve
    the slot or render the strip below the listing.

Cut at the cap (all confirmed, low): requested-vs-shown store altitude
(`editorPane.ts:89`); cross-tab last-writer-wins on recents; the explorer's
whole-state subscription re-rendering on view toggles; the dotted
`styrman.recentFiles.<project>` key vs the `styrman-` convention; `basename()`
in the store module beside CodeEditor's own copy; micro-duplications (Scope
ternary, `isOpen` ×3 per row, prefix predicate duplicated in `closeFileInPane`).
The updater's "does not reinstall itself" gap was dropped as already TODO'd.
