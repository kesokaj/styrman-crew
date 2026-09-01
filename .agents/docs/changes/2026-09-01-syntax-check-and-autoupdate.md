# Format data-loss bug fixed, editor syntax checking, tag-gated auto-updater, second EditorPane bug

Date: 2026-09-01. Second batch of today's feature cycle, unrelated to the
morning's harness re-sync (`2026-09-01-harness-upgrade-v1.15.0.md`) and
editor-pane/ordering work (`2026-09-01-editor-pane-and-ordering.md`) — do not
conflate any of the three same-day entries.

## 1. FIXED: `/format` silently returned empty content and could destroy a file (`internal/orchestrator/exec.go`)

Pre-existing bug, present since `/format` shipped. `Exec` wrote the script's
stdin payload and then called `CloseWrite()` to signal EOF to the guest
process. A Docker hijacked exec stream is **one bidirectional connection**
carrying stdin AND the multiplexed stdout/stderr — half-closing it with
`CloseWrite()` tears down the whole stream, not just the write half, so
anything the guest printed *after* it finished reading stdin was silently
discarded.

Consequence: the shipped `/format` endpoint returned `{"content":""}` for
every single request. The editor feeds that response straight into
`setContent` on the buffer, so clicking Format blanked the open document —
one Save later, the file's contents were gone. Reproduced 3/3 against a real
project VM; instrumented guest output showed the formatting script dying
immediately after it wrote its temp file (`DBG ... size=28` logged, then
nothing further — the guest process was still running, but its stdout after
that point had nowhere to go).

Fix: never half-close. The guest-side scripts now read an **exact byte
count** (`head -c <len>`) instead of blocking on stdin EOF, so the stream
stays fully open for the guest to write its result back. Verified live:
`/format` now returns properly formatted Go source for a real file.

Guarded by a new test, `TestGuestScriptsNeverWaitForStdinEOF`, which was
negative-control tested — it fails if `cat >`/EOF-waiting is reintroduced
into either guest script. Worth recording: unit tests passed throughout the
entire life of this bug, because the defect was in the exec transport, not
in anything the parsing/formatting logic touched — a case where correct unit
coverage gave false confidence and only an end-to-end / live check caught it.

Also fixed while in this code: both the format and check scripts now write
their temp file **into the workspace** (so project-local tool config —
`.golangci.yml`-style files, `pyproject.toml`, etc. — is actually found) and
now `trap 'rm -f ...' EXIT` to clean up; leftover `tmp.*.txt`/`.out` files
from the old scripts had been observed sitting in a user's file listing.

## 2. NEW: syntax checking in the editor (operator request item 5)

`POST /api/projects/{project}/check` (`internal/httpapi/check.go`) — same
exec-in-guest shape as formatting (and the same stdin-exact-count fix from
§1 applies here too) — wired to CodeMirror's lint gutter (`@codemirror/lint`,
now a declared frontend dependency), debounced 1s since each run is a real
VM exec.

Checkers by file type: `gofmt` (Go), `shellcheck` with a `bash -n` fallback
when shellcheck is unavailable (shell), `python3 -m py_compile` (Python),
`node --check` (JS), `jq` (JSON), `yq` (YAML), `tofu fmt` (HCL/Terraform),
`hadolint` (Dockerfile).

Design decisions worth recording:
- Every tool has a genuinely different output format — measured, not
  assumed. gofmt/shellcheck are gcc-style (`file:line:col: message`),
  hadolint emits JSON, bash/python/node/jq/yq each differ from each other
  and from the gcc style. Each tool gets its own parser, unit-tested against
  real captured output, with a fallback that turns any unparseable line into
  a visible line-1 diagnostic rather than silently reporting "clean" — a
  parse failure must never look like a pass.
- Unsupported file types return `supported:false` with **HTTP 200**, not
  400, because the editor calls this endpoint automatically for whatever
  file happens to be open — an unsupported type is a normal, expected
  response, not an error.
- A missing tool binary reports `unavailable` with the tool's name rather
  than failing generically.
- `shellcheck -s bash` is passed **only when the file has no shebang**: it
  otherwise fires SC2148 ("Tips depend on target shell") on every clean
  shebang-less fragment, but passing `-s bash` unconditionally would override
  a real shebang and silently drop dialect-specific findings — measured: a
  `#!/bin/sh` script with a bashism produced SC3014 when shellcheck was
  allowed to detect its own shell, and nothing when `-s bash` was forced over
  it.

New image dependencies (`image/Dockerfile`): `shellcheck` (Debian apt,
0.10.0) and `hadolint` (GitHub release binary, 2.15.1, same install pattern
already used for `yq`). Justification: they are the only real checkers for
shell scripts and Dockerfiles — the two formats the editor had zero
validation for before this.

## 3. NEW: tag-gated auto-updater (operator request item 1)

`deploy/update/styrman-update` + its `.service` + `.timer`, installed by
`deploy/host-setup.sh` step 8c. Ported from argolis-devbox's `devbox update`,
with the change the operator specifically asked for: it acts **only on
stable version tags** matching `^v[0-9]+\.[0-9]+\.[0-9]+$`, never on new
commits. Pre-release tags (`-rc1`, `-beta`, etc.) are ignored by
construction — pattern exclusion, not a separate check — so release
candidates can be pushed to the remote without ever being auto-deployed.
Among matching tags it picks the highest by `sort -V`, not the most
recently pushed by date.

Three deliberate departures from the argolis original, each worth recording:

- **(a) Dedicated clone, not the operator's working tree.** It runs from a
  dedicated clone at `/opt/styrman`, never wherever the operator happens to
  have styrman checked out — styrman's compose build reads from the repo
  root, so an automatic `git checkout <tag>` in the operator's own working
  tree would stomp in-progress, uncommitted work.
- **(b) Rollback + failed-tag latch.** The updater verifies a deploy by
  reading a build-time container label, not by polling an HTTP endpoint: an
  `/api/version` health poll was the original design but was abandoned
  before shipping — production runs `STYRMAN_AUTH=header` and `authGuard`
  exempts only `/api/health`, so the updater's synthetic HTTP identity got
  403 on every call, which meant every healthy release would have timed
  out, rolled back, and latched itself as failed. What shipped instead:
  `deploy/Dockerfile.server` stamps `LABEL
  styrman.version="${STYRMAN_VERSION}"` (the `ARG`/`LABEL` pair re-declared
  in the final stage, placed **last** so a version bump doesn't bust the
  apt-layer cache), and the updater reads it with `docker inspect` together
  with the container's `.State.Running` and health status — no HTTP, no
  auth, no ingress token, and `authGuard` was deliberately not widened. If
  the label/health/running check fails, the updater rolls back to the
  previous tag and records the failed tag so the 5-minute timer does not
  keep rebuilding a known-broken release forever. **This is a load-bearing
  cross-file contract**: deleting the `styrman.version` LABEL from
  `Dockerfile.server` makes the updater roll back every future release (see
  ADR-050's correction).
- **(c) `flock`.** A lock file prevents overlapping runs — the argolis
  timer could stack invocations if a run took longer than the timer
  interval; this one can't.

Both images (server + runtime) rebuild on every matching tag, per the
operator's explicit choice — including the runtime image, which re-seeds
every project's agent harness on its next boot (the ADR-046 delivery path).

**Not yet installed or enabled on this host.** The files exist in the repo
and `host-setup.sh` will enable the timer, but only once the deploy clone at
`/opt/styrman` has its own `deploy/.env` (gitignored, so a brand-new clone
has none by default) — this host has not had that step run yet.

Tested against a real origin: pushing `v1.1.0-rc1` is ignored; pushing
`v1.1.0` afterward is picked up; version ordering is correct (`v1.9.3` sorts
below `v1.10.0`); the failed-tag guard, already-up-to-date, missing-clone,
and concurrent-lock (two invocations racing on the same flock) paths were
all exercised directly.

### Security hardening applied after review

A `security` review
(`.agents/docs/reviews/2026-09-01-security-editor-check-and-autoupdate.md`)
found the updater's real exposure was not the shell-injection surface
(which is a complete fix) but its trust model: it checks out any `vX.Y.Z`
tag on the remote and builds/runs it **as root**, and
`docker-compose.yml` itself lives inside that checkout, so a hostile
revision only needs to add a bind mount to own the host. Fixes, now
shipped (see new ADR-051):

- **Signed tags are required.** The updater rejects lightweight tags
  (`git cat-file -t`, since a lightweight tag cannot carry a signature)
  and runs `git verify-tag`, failing closed. Opt-out only via root-owned
  `/etc/styrman/update.env` (`STYRMAN_UPDATE_REQUIRE_SIGNED=0`), which
  logs a warning when it fires. Operator-visible consequence: tags
  `v1.14.0`–`v1.15.0` are annotated-but-unsigned or lightweight, so the
  updater would refuse them as-is — releases must be signed from now on
  (`git tag -s`).
- Root's `git fetch` now pins the GitHub host key
  (`StrictHostKeyChecking=yes` + `/etc/styrman/known_hosts`) instead of
  `accept-new` — a root fetch whose output is built as root must not
  trust-on-first-use.
- The deploy clone's ownership is asserted with `stat -c %u` before
  `-c safe.directory` disables git's ownership guard.
- `git clean -d --force` runs after checkout so untracked files can't
  survive into the build context (`context: ..`). `-x` is deliberately
  omitted: `deploy/.env` is gitignored and must not be deleted.
- State-file values (`CURRENT`/`PREVIOUS`) are validated against the tag
  regex before reaching git argv — a leading `-` would otherwise be read
  as an option.
- `hadolint` is now pinned to `2.15.1` with a sha256 checksum — an
  unpinned tool made the tag→image mapping non-deterministic, so a
  rollback would not reliably restore the image the old tag produced.
- systemd unit hardening (`NoNewPrivileges`, `ProtectHome`, …) added, with
  the honest caveat that it buys little on its own: a hostile tag's
  payload runs in the docker daemon, not in this unit's cgroup — the
  signature check is the control that actually matters.

### Second round of review fixes

A re-review
(`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-rereview.md`)
of the hardening above caught a regression it introduced and one prior fix
that never worked:

- **State-file regression.** The updater's `.deployed-version` and
  `.failed-version` files lived at the clone root and were being deleted
  by the `git clean -d --force` added for the security fix above (neither
  is gitignored), which erased the failed-tag latch on every rollback and
  reinstated exactly the retry storm that latch exists to prevent
  (reproduced in a scratch repo, then fixed). They now live outside the
  work tree entirely, at `/var/lib/styrman/update/`.
- **Python litter fix was inert.** `PYTHONDONTWRITEBYTECODE=1` governs the
  *import system*, not `py_compile`, whose entire job is emitting
  bytecode — measured directly (`python3 -m py_compile` still wrote
  `__pycache__/*.pyc` into the workspace with that env var set). Replaced
  with `PYTHONPYCACHEPREFIX`, verified to leave no `__pycache__` in the
  workspace.
- **Health check hardened further.** The version check now also requires
  `.State.Running=true`; previously a `health=none` verdict alone could
  verify a dead container.
- **Compose build now covers all services.** `docker compose up -d` alone
  never rebuilt `styrman-caddy` (it only builds images that don't already
  exist), so a tag that changed `deploy/caddy/**` shipped everything
  except the ingress while still reporting VERIFIED. The updater now
  builds all services explicitly.

## 4. Second UI bug found by the new e2e coverage (`web/src/components/EditorPane.tsx`)

`ConfirmDialog` invoked `onConfirm()` and then `onClose()` from the same
click handler. For the "you have unsaved changes, discard and open the new
file?" flow, this meant the discard-and-open confirm handler ran, and was
immediately followed by the dialog's own close path — which reverted the
editor store back to the ORIGINAL file. Net effect: choosing "Discard and
open" reloaded the first file instead of the one the operator had just
clicked. First fixed with a synchronous ref flag in `EditorPane`, but that
was removed during remediation in favor of the real fix: `ConfirmDialog`
gained an optional `onCancel` that fires only on a genuine dismissal (the
Cancel button, the backdrop, or Escape) and never after `onConfirm`, so
callers no longer need to distinguish "closed because confirmed" from
"closed because cancelled" via a side-channel ref — `EditorPane` calls
`onConfirm` to discard-and-open and simply never wires a revert into
`onCancel`. All other `ConfirmDialog` callers (`FileExplorer.tsx`,
`SidePanelGit.tsx`, `TerminalList.tsx` ×2, `SetupBanner.tsx`) don't pass
`onCancel` and are unaffected.

## Verification

- `go build ./...`, `go vet ./...`, `go test -race ./...` — all ok,
  including the `deploy/caddy/ingress` module.
- `gofmt -l` — clean.
- `tsc -b` and `oxlint` — clean.
- `shellcheck -S error` — clean on `deploy/update/styrman-update`,
  `deploy/host-setup.sh`, and `web/e2e/run.sh`.
- `integration.sh` — 57 passed / 0 failed / 3 skipped.
- e2e (`web/e2e/run.mjs`) — **48 passed / 0 failed / 2 skipped**, up from 34
  at the start of the day. Two checks were added after this section was
  first written: terminal width is preserved across a Terminal→Editor→
  Terminal round-trip, and the litter check was extended to a Python file
  asserting no `__pycache__` is left behind. Both were negative-control
  tested — each confirmed to FAIL with its bug reintroduced, then pass
  again with the fix restored.
- Live API verification against a throwaway project:
  - `/format` returns properly formatted content (the bug in §1 is closed).
  - `/check` on a broken shell script returns 4 shellcheck errors on the
    correct lines; on a clean shell script, clean; on a Dockerfile, a DL3006
    warning plus a DL3009 info; on broken JSON, a jq parse error at line 1
    col 9; on clean Go, clean; on an unsupported `.txt` file,
    `supported:false` with HTTP 200.

Re-verified after the security hardening and second review-fix rounds
above: `go build`/`vet`/`test -race` (incl. the `internal/httpapi`,
`internal/conversation`, `internal/orchestrator` packages) and `gofmt -l`
clean; `tsc -b`/`oxlint` clean; `shellcheck` clean on
`deploy/update/styrman-update` at **default** severity (not just `-S
error`); `TestSafeSuffixBlocksInjection` passing; and a label
decision table exercised against a real container — VERIFIED when the
`styrman.version` label matches and the container is running and healthy,
REFUSED (×3: label mismatch, container not running, missing label) —
confirming the update-verification rewrite in §3 works end to end. e2e
re-run at 48 passed / 0 failed / 2 skipped, unchanged by the fixes above.

## 5. Late fixes from the FINAL review (post-rereview)

The FINAL review
(`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-final.md`,
verdict APPROVE) surfaced one more UI defect and several smaller items,
all fixed in this same batch:

- **UI defect found by looking at a screenshot, not by a test**
  (`web/src/components/TerminalPane.tsx`). After a Terminal→Editor→Terminal
  round-trip, the terminal came back rendering at roughly 12 columns.
  `TerminalPane`'s `onResize` called `fit.fit()` **before** its `cols < 10`
  guard, so a hidden (`display:none`) pane collapsed the local xterm before
  the guard could reject the bad size — the guard correctly stopped that bad
  size from reaching tmux, but recovery then had to wait on a 120ms
  debounce. Fixed by exposing the resize handler and calling it on the
  existing focus schedule when the pane is shown again. Worth recording:
  every automated check passed the entire time this was broken, because
  typing still worked through the collapsed pane — the render was what
  caught it, not a test.
- Unsaved-editor state lifted into the pane store and surfaced as a dot on
  the Editor tab — previously the only dirty cue was a Save button inside
  the pane that had just been hidden.
- `EditorPane` is now keyed per terminal. Without the key it was reused
  across a terminal switch: the loaded buffer was cleared and the previous
  terminal's unsaved edits were wiped with no prompt, while `setDirty(false)`
  landed on the NEW terminal's key and left the old one latched dirty.
- Toolbar: the syntax-check note was `shrink-0 truncate` — contradictory,
  since it could not shrink and pushed the action buttons off a narrow
  pane — now `min-w-0 truncate`. `EditorPane`'s two divergent "no file open"
  presentations were unified into one.
- `deploy/host-setup.sh` now seeds `/etc/styrman/known_hosts` with
  `ssh-keyscan` during the auto-clone, and the signing instructions now
  cover the VERIFYING side too: `git verify-tag` runs as root, so root needs
  the operator's PUBLIC key (a GPG keyring entry, or a root-readable
  `allowed_signers` file for SSH signing) — signing the tag alone is not
  enough for verification to succeed.

## Follow-ups

See ADR-049 (exec stdin contract — never half-close), ADR-050
(tag-gated auto-update model, corrected same-day — see its "Correction"
block for the label-based health check that actually shipped), and the
new ADR-051 (signed tags required for auto-update) in
`architecture/DECISIONS.md`. TODO.md carries the coverage gaps and low
findings from both the original review and the 2026-09-01 re-review that
remain open (state-file/latch fix aside, which is closed above).
