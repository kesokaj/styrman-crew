---
owner: main
date: 2026-09-16
---
# Harness channel Phase 1: the channel on the host (2026-09-16)

## Summary

Phase 1 of `.agents/docs/plans/2026-09-16-harness-channel.md` (ADR-073): the crew updater gained a `harness` verb that delivers the newest SSH-signed `styrman-harness` tag to `/var/lib/styrman/harness/current` on the host, with its own systemd unit pair, `styrman-ctl` setup and status, and two Go tests that drive the script end to end in test mode. Nothing consumes the channel yet: the server mount (Phase 2) and the entrypoint clause (Phase 3) follow in the same release.

## Type

Feature (phase checkpoint)

## Changed

- `deploy/update/styrman-update` — `MODE=harness`: `harness_main` (walks `vX.Y.Z` tags downward to the newest that verifies; alarms every refusal to the journal and `harness-refused`; refuses a tag below the installed `harness-version`; latches a tag that fails export or smoke in `harness-failed-version`; `--check`, `--force`; `REQUIRE_SIGNED` forced to 1 on this path), `harness_export` (git archive minus `.agents`/`.gitignore`/`.git`, `COMMIT`/`TAG`/`SHA256SUMS` in the mirror's shape, `sh-agent` built offline in the image's pinned `harness-builder` Go image), `harness_smoke` (`sh-agent list --repo` and `install --dry-run` in the deployed runtime image as uid 1000), `harness_swap` (one-rename `current` symlink, previous target kept), `harness_alarm`, `harness_test_guard` (test mode only on scratch paths), `ver_gt`; `git()` takes its `safe.directory` at call time so the harness clone gets its own; `self_install` installs the harness unit pair as optional entries; `host_converge` enables `styrman-harness-update.timer` unconditionally when unit and clone exist (BUG-035). Dispatch sits after the deploy-clone assertions and before the crew token block.
- `deploy/update/styrman-harness-update.service`, `.timer` — new: own `RuntimeDirectory`, `DOCKER_CONFIG`, `TimeoutStartSec=45min`, the paths pinned in the unit (ADR-052), timer `OnBootSec=7min`/`OnUnitActiveSec=5min`.
- `deploy/styrman-ctl` — `HARNESS_REPO_DIR` (default beside the crew clone), `HARNESS_ORIGIN`, the two units in `UNITS`, three harness variables in `UPDATER_ENV`, `--harness-token-file` on `install` and `update --setup`, `setup_harness_channel` (token root 600, content dir root 755 never a symlink, the clone under the scoped one-shot credential helper, `harness --check`, timer enabled), the `status` harness line with its DEGRADED rule and JSON object; unattended without a token the harness half is skipped with the command that does it later.
- `deploy/host-setup.sh` — installs the unit pair; creates `/var/lib/styrman/harness` root 0755 and refuses a symlink there.
- `internal/config/styrman_harness_test.go` — new: `TestStyrmanHarnessChannel` (scratch origin from `image/harness/`, scratch ed25519 keys; the walk-down over a foreign-key tag, a lightweight tag and a symlink-broken export that latches, down to the signed `v1.0.0`; state files; export shape and `sha256sum -c`; the export equals the mirror's files; `--check`; downgrade refusal; `--force`; the ungated verb refused). Runs unelevated against scratch paths; skips only without docker, git, ssh-keygen or the two pinned images.
- `internal/config/styrman_ctl_harness_test.go` — new: `TestStyrmanCtlUpdateSetupHarnessPlan` (the dry-run plan of `update --setup --harness-token-file`, ordering, the harness clone reading its own token, no production path in a scratch-prefixed plan; the skip line without a token).
- Docs (`documentation` agent, read and one sentence corrected by the main agent): `.agents/docs/specs/styrman/deploy.md` (new "Harness channel (ADR-073)" subsection), `.agents/docs/specs/styrman/versioning.md`, `deploy/README.md`, `.agents/docs/deploy/remote.md` (the harness token prerequisite), `.agents/docs/testing/styrman/README.md` (Tier 1 names the two tests), `AGENTS.md` § Setup Commands (the ctl setup line).
- `.agents/docs/plans/2026-09-16-harness-channel.md` — Phase 1 boxes and Checkpoint ticked with the evidence; `.agents/docs/reviews/2026-09-16-review-plan-harness-channel.md` — the plan's three cold-read rounds (committed with this phase).

## Verification

`verifier` agent, `.agents/docs/testing/2026-09-16-verify-harness-channel-phase-1.md`: PASS — `go build`/`go vet`/`gofmt` clean; `go test ./internal/config/ -run 'StyrmanHarness|StyrmanCtl' -count=1 -v` 18/18 PASS with `TestStyrmanHarnessChannel` a real run (docker and both pinned images present, no skip); `go test -race ./... -count=1` 15 ok; the ingress module ok; `bash -n` and `shellcheck -S warning` clean on the three scripts; `systemd-analyze verify` on both units rc 0; positive control: HEAD's updater exits 2 with "unknown flag: harness". Documented gap: `deploy/host-setup.sh`'s new lines are root-only and have no automated tier; they are read on the host under Phase 4's deploy box.

Fixes on the way (no bug record: each was a first-attempt test correction, not a defect that shipped or regressed): the ctl `HARNESS_REPO_DIR` default first leaked `/opt/styrman-harness` into scratch-prefixed plans (made relative to the crew clone); the new plan test's leak assertion matched the scratch path's own `/opt/` suffix (narrowed to `=/opt/` and ` /opt/`); its no-token assertion matched the crew updater's env, which legitimately names the harness paths (narrowed to the token install and the timer); a stray untracked `image/harness/.agents/scratch` broke the export-equals-mirror diff (removed; `.agents` excluded on both sides).

## Agents dispatched

`verifier` (the log above), `documentation` (the six files above). Review and security rows are owed once over the whole feature at Done (Phase 4), the whole diff being one change.
