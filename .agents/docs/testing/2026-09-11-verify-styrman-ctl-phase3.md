---
owner: main
date: 2026-09-11
---
# Verification — styrman-ctl, Phase 3 (`edit`, `update`, `--redeploy`)   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after the phase commit.

| Step | Command | Result |
|---|---|---|
| Lint | `shellcheck deploy/styrman-ctl` | clean |
| Unit | `go vet ./... && go test -race -count=1 -run StyrmanCtl ./internal/config/` | 11 tests green (2 new: edit plan, update plan; the Phase 2 test's "plain update is a stub" assertion removed) |
| Real, dry-run | `sudo deploy/styrman-ctl edit --dry-run STYRMAN_IDLE_TIMEOUT=9h` on the production host | plan: `env_set /opt/styrman/deploy/.env STYRMAN_IDLE_TIMEOUT` (the checkout has no `.env` here, so one copy), then `env STYRMAN_VERSION=1.27.1 docker compose -f /opt/styrman/deploy/docker-compose.yml --env-file /opt/styrman/deploy/.env up -d`, the healthy wait, `status`; nothing changed |
| Real, dry-run | `sudo deploy/styrman-ctl update --dry-run` and `update --redeploy --dry-run` | plans: `systemctl start styrman-update.service` + `journalctl -u styrman-update.service -n 20`; `env STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update STYRMAN_DEPLOY_DIR=/opt/styrman /usr/local/bin/styrman-update --force` + journal; nothing changed |

PASS. Not verified: a real `edit` (the production `.env` is not to be
changed for a test) and a real updater tick (nothing newer than v1.27.1 to
deploy); both are the operator's when wanted.

## Review dispositions folded into this phase's commit

- Security, Phase 1 (`reviews/2026-09-11-security-styrman-ctl-phase1.md`): HIGH — `env_set`'s replace branch passed the value through `awk -v`, which decodes escapes, so a literal `\n` smuggled a second assignment past the newline refusal → FIXED: the value reaches awk through the environment (`ENVIRON`), and a backslash is refused outright; test plants `x\nSUPERADMINS=evil@x.y` and asserts nothing was smuggled. MEDIUM — `env_get` interpolated the key into a sed program → FIXED: the key is validated and matched literally as an awk variable; test passes a key carrying a sed program. MEDIUM — the `__` hooks were an unconfined root file mutator → FIXED: gated on `STYRMAN_CTL_TEST=1` (set only by the Go harness); test asserts they are refused without it; `ask`'s precedence/tty defects were already fixed in Phase 2. Also fixed: `guard_path` now requires the path to be canonical as given (no symlink component, no `..`) and an existing directory, and compares literally against the configured trees; `json_str` has a python3-free fallback; every value that a file, git or a container label contributes to `status` is stripped of control bytes. Accepted: the env-supplied allowlist (root's own configuration).
- Reviewer, Phase 2 (`reviews/2026-09-11-review-styrman-ctl-phase2.md`, REQUEST CHANGES): HIGH — the setup's `styrman-update --check` named the state and deploy dirs but not the token/signers files, which default to `/etc/styrman` inside the updater → FIXED: `UPDATER_ENV` names all four on every hand invocation (`--check`, `--force`); test asserts them. LOW — dry-run wrote the secrets to temp files → FIXED: under `--dry-run` nothing touches the disk and the plan shows `<tmpdir>`; test asserts the placeholder.
- Verifier, Phase 2: PASS (`testing/2026-09-11-verifier-styrman-ctl-phase2.md`).

Re-run after these fixes: shellcheck clean; 11 tests green; the real `update --setup --dry-run` on this host shows the four-variable `--check` line and `<tmpdir>` placeholders.
