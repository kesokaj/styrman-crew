---
owner: main
date: 2026-09-11
---
# Verification — styrman-ctl, Phase 2 (`install`, `update --setup`)   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after the phase commit.

| Step | Command | Result |
|---|---|---|
| Lint | `shellcheck deploy/styrman-ctl` | clean |
| Unit | `go vet ./internal/config/ && go test -race -count=1 -run StyrmanCtl ./internal/config/` | 9 tests green (3 new: install plan, install refusals, update --setup plan) |
| Real, dry-run | `sudo deploy/styrman-ctl install --dry-run --version 1.27.1 --skip-auto-update` on the production host | the plan: `host-setup.sh --user kesokaj`, the six prompts (would write `deploy/.env`), `docker build styrman-runtime:1.27.1`, `docker compose -f <repo>/deploy/docker-compose.yml --env-file <repo>/deploy/.env up -d --build`, the healthy wait; nothing executed (`git status` and `systemctl is-enabled styrman-update.timer` unchanged) |
| Real, dry-run | `sudo deploy/styrman-ctl update --setup --dry-run --gh-token-file <scratch> --allowed-signers-file <scratch>` | the plan: `install -d /etc/styrman`, the token install from a root-only temp file, `fetch --tags` (a clone exists) under `GIT_ASKPASS`, `chown -R root:root /opt/styrman`, the `.env` copy, `allowed_signers` install, `styrman-update --check` with `STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update`, `enable --now` the timer, `start` the service; the token string absent from the output; nothing executed |

PASS. Defects the tests caught before commit: `ask` read `/dev/tty` on the
strength of a permission bit (now it opens the device to decide); `ask`'s own
local `v` shadowed the caller's target (`printf -v` assigns by name — locals
now carry underscores); the unattended error named the variable, not the
key; `is_loopback_addr` lost `BASH_REMATCH` to its helpers' own matches
(groups copied first). The Phase 1 reviewer's medium (the checkout `.env`
path had no scratch override) is closed by `STYRMAN_ENV_FILE`; its low
(unbounded octets/port in the loopback rule) is closed too.

Not verified: a real `install` (this host is production; the fresh-host run
is the operator's or a nested-virt VM's); a real `update --setup` (the host
already has one).
