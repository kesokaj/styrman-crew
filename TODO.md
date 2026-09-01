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

- [ ] 🟡 Cross-tenant deletion via symlink in `sweepMaterializedFile`
  (`internal/httpapi/credentials.go:217`), fix with an `os.Root`-confined
  `files.Remove`. (2026-09-01 whole-project audit)
- [ ] 🟡 `usernameFromEmail` duplicates `identity.DeriveUsername` on a
  false import-cycle justification, live on two different auth paths —
  divergence resolves one email to two state dirs
  (`internal/config/config.go:392`). (2026-09-01 whole-project audit)
- [ ] 🟡 Project egress deny-list omits `100.64.0.0/10` (Tailscale) and
  `198.18.0.0/15` (`deploy/host-setup.sh:149-206`). (2026-09-01
  whole-project audit)
- [ ] 🟡 `STYRMAN_RUNTIME` unvalidated while escape-capable caps are
  granted (`internal/orchestrator/orchestrator.go:759`). (2026-09-01
  whole-project audit)
- [ ] 🟡 `formatInPlace` has no test coverage, unlike its analog
  `checkerFor`. (2026-09-01 whole-project audit)
- [ ] 🟡 Add `deploy/caddy/ingress` to `/preflight`'s verification — no
  gate scans it today. (2026-09-01 whole-project audit)
- [ ] 🟢 Dead code: `LastActivity`, `Project.Agent`, `Project.GitInit`.
  (2026-09-01 whole-project audit)
- [ ] 🟢 Coverage: zero frontend unit tests (8,254 LOC);
  `internal/identity` untested despite owning the tenancy key;
  `orchestrator` 3.0%; `terminal` 8.5%. (2026-09-01 whole-project audit)

- [ ] 🟡 Caddy base images pinned by tag only, not digest, and the
  builder's own Go toolchain is unasserted (`deploy/caddy/Dockerfile`) —
  `caddy:2.11.4-builder`/`caddy:2.11.4` still float upstream, so
  "tag → binary" stays non-deterministic without a digest pin; add a
  `go version -m` assertion alongside the existing `caddy list-modules`
  check. (security review 2026-09-01,
  `.agents/docs/reviews/2026-09-01-security-redfix.md`)
- [ ] 🟢 `STYRMAN_UPDATE_STATE_DIR=/etc` or `=/` passes the ADR-052 guard
  (root-owned, mode 755 has no group/world-write bits) — a one-component
  typo off the recommended `/etc/styrman/update` silently writes
  deploy-state files into a shared system dir, and the host then looks
  "never deployed" to the timer. Require the canonicalized path to have
  at least two components and refuse a bare `/`, `/etc`, `/var`, `/tmp`,
  `/opt`, `/home`. (round-3 re-review,
  `.agents/docs/reviews/2026-09-01-review-redfix-round2.md`)
- [ ] 🟢 Ingress admin `origins` directive is browser-only defense-in-depth,
  not `/load` authentication — the comment/test now say so correctly, but
  the loopback guard still accepts `localhost` by name (resolution-dependent)
  rather than restricting to IP literals; neither is a live hole today
  (loopback bind + ordering luck), just worth tightening if the guard is
  ever reused elsewhere. (`deploy/caddy/ingress/generator.go`,
  `loopback.go`; `.agents/docs/reviews/2026-09-01-review-redfix.md`,
  `.agents/docs/reviews/2026-09-01-security-redfix.md`)

- [x] Editor/file-explorer UX pass (operator, 2026-09-01: "just some
  optimisation" — current behavior works fine, this is polish, not a fix).
  Highlight the currently-open file in the file explorer; show the open
  file's name at the top of the editor pane; a recent-files affordance.
  SHIPPED 2026-09-01 — see
  `.agents/docs/changes/2026-09-01-editor-ux-and-updater-fixes.md`. The
  fourth item scoped here, a per-project "open until closed" notion (the
  active file stays active across explorer navigation until explicitly
  closed, rather than tracking explorer selection), was not part of what
  shipped — re-raise separately if still wanted.
- [ ] 🟡 The updater does not reinstall ITSELF: a release that changes
  `deploy/update/*` deploys the stack but leaves the OLD
  `/usr/local/bin/styrman-update` + units installed (bit twice today: the
  token-auth script and the `DOCKER_CONFIG` unit fix each needed a manual
  `install`). Fix idea: `deploy()` installs the checkout's updater
  script+units after a successful health check (takes effect next run).
- [ ] 🟢 lows carried from `reviews/2026-09-01-review-editor-ux.md` round 2
  worth tracking: badge/dot invisible to screen readers under the
  `aria-label` pin (sr-only sibling or `aria-describedby`); `FileEditor`'s
  during-render reset keyed on path only, not scope (cross-scope same-path
  open now one click away via chips); open row/chip lose hover affordance.
- [ ] 🟢 Bearer-token possession-binding trade, accepted risk recorded in
  ADR-056: `/etc/styrman/gh-token` grants full repo-read from any machine
  that reads it once, with no host-key/client-cert/IP anchor the way the
  SSH deploy key it replaced had. Accepted (expiry + one-click GitHub-side
  revocation outweigh it for a repo-scoped read-only credential) — listed
  here so the trade isn't only visible inside the ADR. (security review
  2026-09-01, `.agents/docs/reviews/2026-09-01-security-no-backups.md`)
- [ ] 🟡 Operator actions from the no-backups/token-auth change: (1) mint
  the fine-grained GitHub token (this repo only, Contents: Read-only) and
  install it at `/etc/styrman/gh-token` (root:root, `600`) — **DONE
  2026-09-01**, and the first real timer-driven auto-deploy has since run
  successfully end-to-end (see
  `.agents/docs/changes/2026-09-01-editor-ux-and-updater-fixes.md`); (2) delete the GCS bucket
  `styrman-crew-backups-driven-eft-h4it` and its backup service account in
  project `driven-eft-h4it` (no `gcloud` on this host — operator-run); (3)
  shred `/var/lib/styrman/backups/` on the host — an orphaned, unencrypted
  archive containing at least one project's home (ssh keys, gcloud ADC,
  claude/agy tokens) with no code path left to list or delete it; no agent
  touches `/var/lib/styrman` (ADR-052).
  (`.agents/docs/changes/2026-09-01-remove-backups-and-token-auth.md`)
