# TODO

Empty by design (operator directive 2026-08-30: "I want nothing in todo and
roadmap — start fresh with new things"). The entire previous roadmap was
either SHIPPED in v1.14.0 or RETIRED with reasons in **ADR-045**
(`.agents/docs/architecture/DECISIONS.md`) — including the standing
WON'T-DO decisions. History lives in `.agents/docs/changes/` and git.

Add new items below as they arise.

## Inbox

- [ ] Settings harness roster is permanently empty. `internal/httpapi/setup.go:230-252`
  reports the harness roster (`Skills`, `Agents`, `Commands`) from
  `s.cfg.UserHome(u.Username)` — the base/setup home. The entrypoint never
  seeds that home, because the setup VM runs with `STYRMAN_MINIMAL=1`
  (`internal/orchestrator/orchestrator.go:565` → `image/entrypoint.sh`), so
  all three fields are always empty and `omitempty` hides them. Confirmed on
  the host: `/var/lib/styrman/homes/ostling-simon/.claude/agents` does not
  exist. Consequence: the 28 skills / 6 subagents shipped in v1.15.0 can
  never surface in the UI. Pre-existing, deliberately deferred out of the
  v1.15.0 vendoring release; both the reviewer and the agent agree the
  deferral is correct because a real fix needs a product decision (point the
  lookup at a project home, or delete the three fields as dead reporting).
  Secondary: if it is ever repointed at a seeded home, `listNames`
  (`setup.go:167-178`) strips only a `.md` suffix, so harness re-seed
  backups would be listed as agents — a real example exists on the host:
  `~/.claude/agents/testing.md.bak.20260901-065226`. Filter `*.bak.*` if
  repointed. (deferred: needs a product decision, not a code fix)
- [ ] `checkFile`/`formatFile` (`internal/httpapi/check.go`,
  `internal/orchestrator/exec.go`) have no Go handler-level test — only e2e
  coverage plus the script-contract unit test
  (`TestGuestScriptsNeverWaitForStdinEOF`) exercise them. (deferred: flagged
  by the verifier during the 2026-09-01 syntax-check/auto-updater batch,
  not fixed in that pass)
- [ ] `deploy/Dockerfile.server` still embeds a committed `web/dist` with
  nothing verifying it matches current `web/src` — the same stale-asset
  hazard `web/e2e/run.sh` was fixed for on the test path earlier the same
  day (`2026-09-01-editor-pane-and-ordering.md`). (deferred: flagged by the
  verifier during the 2026-09-01 syntax-check/auto-updater batch, not fixed
  in that pass)

- [ ] Unpinned toolchain fetches in `image/Dockerfile` — `yq` uses
  `releases/latest/download`, and helm/OpenTofu install via `curl | bash`, all
  without checksums. Consequences: the runtime image is shared across every
  user's VM alongside their seeded credentials, and an unpinned tool makes the
  tag→image mapping non-deterministic, so the auto-updater's rollback does not
  restore the exact image the old tag produced. `hadolint` was pinned with a
  sha256 when it was added (2026-09-01); the pre-existing three were left alone
  to keep that change small. (security review 2026-09-01)

- [ ] `govulncheck ./...` reports 2 CALLED vulnerabilities, both in
  `github.com/docker/docker@v28.5.2+incompatible` (GO-2026-4887, GO-2026-4883),
  both **Fixed in: N/A** and both daemon-side, flagged only through `init`
  reachability. Pre-existing, no upgrade available — recheck when docker
  publishes a fix. (security review 2026-09-01)

- [ ] Carried-over low findings from the 2026-09-01 re-review of the
  syntax-check/auto-updater remediation
  (`.agents/docs/reviews/2026-09-01-review-syntax-check-and-autoupdate-rereview.md`),
  none re-argued, all still open: `deploy/update/styrman-update.timer`'s
  `Persistent=true` is inert on a monotonic (`OnBootSec`/`OnUnitActiveSec`)
  timer; `deploy/host-setup.sh` step 8c is still physically ordered before
  step 8b; `deploy()` calls `fail` (which exits) on a checkout error instead
  of `return 1` like the build/health paths, so that one path still bypasses
  the failed-tag latch; `mktemp -p .` in both `check.go` and `format.go`
  writes into the workspace root on every debounce tick instead of a
  dot-dir; the Terraform checker still reports `STYRMAN_TOOL gcc` to the UI
  (mislabelled — it runs `tofu fmt -check`, not gcc, and that's a formatting
  check sold as a syntax check); dead trap globs remain in `check.go`/
  `format.go` (`${F}.err`/`${F}.out` for files never created, plus a
  trailing `rm -f` the `EXIT` trap already covers); `checkFile`
  (`web/src/lib/api.ts`, `CodeEditor.tsx`) takes no `AbortSignal`; the lint
  gutter (`lintGutter()`/`linter()`) is still mounted unconditionally
  instead of only for supported file types; and the close-file confirm
  dialog's "Discard" button (`EditorPane.tsx:171-178`) still discards
  nothing. (deferred: all low-severity, none blocking; batched here so
  they aren't lost)

- [ ] Unsaved editor edits are still lost without a prompt when switching
  between terminals. Keying `EditorPane` per terminal (2026-09-01) fixed the
  state crosstalk — the wrong terminal being latched dirty — but not the
  underlying loss: the pane remounts on a terminal switch and the buffer
  goes with it. Opening another file and closing the editor both prompt to
  save; switching terminals does not. Fixing it properly needs either the
  draft buffer to survive per terminal, or an intercept on sidebar
  navigation. (flagged by the 2026-09-01 FINAL review, not fixed in that
  pass)

- [ ] `TerminalPane`'s focus/refit schedule sends up to four identical
  tmux resize frames per Editor→Terminal toggle — the exact cost the
  120ms debounce exists to avoid. Verified not harmful (no
  disposed-terminal or pre-WebSocket-window error observed), just
  wasteful; a single settled refit would do. (flagged by the 2026-09-01
  preflight gate, not fixed in that pass)
