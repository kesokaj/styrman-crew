---
owner: main
date: 2026-09-11
---
# Verification — styrman-ctl, Phase 4 (`uninstall`, docs)   (2026-09-11)

Written by main; the `verifier` agent runs over the committed tree after the phase commit.

| Step | Command | Result |
|---|---|---|
| Lint | `shellcheck deploy/styrman-ctl` | clean (SC2030/2031 disabled on the deliberate plan-printing subshell) |
| Unit | `go vet ./... && go test -race -count=1 -run StyrmanCtl ./internal/config/` | 13 tests green (2 new: uninstall plans per level, uninstall confirmations; the Phase 1 "uninstall is a stub" assertion removed) |
| Real, dry-run | `sudo deploy/styrman-ctl uninstall --level 3 --dry-run` on the production host | the full plan printed in order: compose `down --remove-orphans` from `/opt/styrman` at 1.27.1, `docker rm -f` of the one (stopped) project VM, `docker network rm` of its `sdb-` network, the four units disabled, the IPv4 and IPv6 chain teardown, the four unit files + daemon-reload, the two binaries, the KSM tmpfiles entry, `rm -rf -- /etc/styrman`, `rm -rf -- /opt/styrman`, `docker image rm` of every `styrman-*` image (≈100 tags on this host), `174M /var/lib/styrman` then `rm -rf -- /var/lib/styrman`; "nothing executed (the word to type for level 3 is: dave)"; afterwards the two stack containers still up and `styrman-firewall.timer` still enabled |

PASS. Not verified: a real `uninstall` at any level (this host is
production), a real `install` on a fresh host, the level-1-then-install
round trip — the plan's fresh-host e2e is the operator's (or a nested-virt
VM's), recorded when run.

Observation, not this plan's: the host carries ≈100 `styrman-*` image tags
from earlier releases and verification builds; `uninstall --level 2` would
remove them all, and a prune of the old ones is an operator housekeeping
item (`docker image prune` does not touch tagged images).

## Security round 2 (Phase 2 report, `reviews/2026-09-11-security-styrman-ctl-phase2.md`) — dispositions in this phase's commit

- F1 (medium, `awk -v` decoded `\n` in a value): RESOLVED in Phase 3 (awk `ENVIRON`, backslash refused, test).
- F2 (medium, the one-shot askpass answered any host; no https assertion; inherited helpers not cleared): FIXED — the token now reaches git the way `host-setup.sh` 8c and the updater do: `-c credential.helper=` clears every inherited helper and an inline helper scoped to `credential.https://<origin host>.helper` reads the root-only token file; `https_origin` accepts only a plain `https://host/path` origin. Tests assert the scoped helper on both clone and fetch, the absence of any askpass, and that `ext::`, `file://` and an option-shaped origin are refused before any plan.
- F3 (low, origin as a bare positional): FIXED — `clone -- "${origin}"` plus the allow-list above.
- F4 (low, `fetch --tags` offered the token to whatever remote an existing clone had): FIXED — the existing clone's origin (normalized to https) must equal the expected origin or the run dies; test plants a clone pointing elsewhere.
- F5 (low, `__` hooks reachable): RESOLVED in Phase 3 (gated on `STYRMAN_CTL_TEST=1`).
- F6 (low, secret files read regardless of mode): FIXED — a group- or world-readable `--gh-token-file` is refused with "chmod 600 it first" (test); `--allowed-signers-file` with more than one line is refused instead of silently dropping lines.
- F7 (low, temp secrets cleaned by EXIT only): FIXED — the trap covers INT, TERM and HUP.
- F8 (low, `is_signers_line` unanchored): FIXED — anchored, one optional trailing comment field.
- F9 (low, `$`/`{`/`}`/backslash accepted in secrets that compose interpolates): FIXED — `validate` refuses them for the three secret keys (tests).
- F10 (info, `docker logs` tail on a failed bring-up): accepted — it is the operator's terminal; a unit-driven install is not a supported path.

After these: shellcheck clean; 13 tests green; the real `update --setup --dry-run` on this host still prints its nine-line plan (the clone's origin matched the checkout's).
