---
owner: reviewer
date: 2026-09-11
---
# Review — styrman-ctl phase 4 (uninstall)

Range reviewed: `git diff edc4858..108c4e4 -- deploy/styrman-ctl internal/config/styrman_ctl_test.go`
Blobs: `deploy/styrman-ctl@62fb248`, `internal/config/styrman_ctl_test.go@0257cbe` (both at `108c4e4`).

## Verdict: REQUEST CHANGES

The single worst problem: the level-2/3 firewall teardown's `iptables -D`/`ip6tables -D` calls do not
match the rule specification the firewall script actually installs, so on any host where
`styrman-firewall.timer` has ever run (i.e. every real deployment), a real `uninstall --level 2` or
`--level 3` will abort mid-script under `set -e` — after the level-1 stack/container/network teardown
already ran, but before ANY of level 2's unit files, `/etc/styrman`, the deploy clone, the images, or
(at level 3) the entire `${STATE_DIR}` data tree are removed. An operator who has just typed the
irreversible confirmation word for level 3 is left with a half-torn-down host and stale firewall state,
told nothing beyond a raw `iptables: Bad rule` error. This is invisible to the test suite because every
uninstall test only exercises `--dry-run`, where `run()` never actually invokes `iptables`.

## Findings

- **[high / confidence: high]** `deploy/styrman-ctl:687-694` (uninstall_level's iptables/ip6tables block) —
  `run iptables -D DOCKER-USER -j STYRMAN-FWD` and `run iptables -D INPUT -j STYRMAN-IN` (and the `ip6tables`
  equivalents for `STYRMAN-FWD6`/`STYRMAN-IN6`) omit `-i sdb-+`. The actual rules, installed by
  `deploy/firewall/styrman-firewall.sh`'s `ensure_jump()` (`ipt -I "$1" -i "$2" -j "$3"`, called as
  `ensure_jump INPUT sdb-+ STYRMAN-IN` and `ensure_jump DOCKER-USER sdb-+ STYRMAN-FWD`, and the ip6tables
  block's literal `-I INPUT -i sdb-+ -j STYRMAN-IN6` / `-I FORWARD -i sdb-+ -j STYRMAN-FWD6`), all carry
  `-i sdb-+`. `iptables -D`/`ip6tables -D` requires an exact match of every specified condition against the
  existing rule; a delete spec with fewer match options than the live rule does not match it and exits
  non-zero ("Bad rule (does a matching rule exist in that chain?)"). Because these four `run` calls are
  bare statements under the file's `set -euo pipefail` (no `|| true`, unlike the journalctl/cmd_status
  lines a few lines away, and unlike the idempotent/`|| true` style `deploy/firewall/install.sh` uses for
  its own converge-cleanup), the first failing `-D` aborts `uninstall_level` — and therefore the whole
  `cmd_uninstall` — immediately. Concrete failure: `sudo deploy/styrman-ctl uninstall --level 3 --confirm
  <hostname>` on the production host stops and removes the stack/containers/networks (level 1), then dies
  on `iptables -D DOCKER-USER -j STYRMAN-FWD`; the firewall chains, `styrman-update`/`styrman-firewall`
  units, `/etc/styrman` (token, signers, updater state), `/opt/styrman`, the runtime/server/caddy images,
  the `.env` backup, and — despite the operator having just confirmed irreversible deletion — the entire
  `${STATE_DIR}` (every project home/workspace) are all left in place. Fix: match on rendered rule text via
  `iptables -S <chain>` the way `ensure_jump` does, or simply include `-i sdb-+` in the delete spec (and
  wrap each `-D`/`-F`/`-X` in `|| true`, consistent with the rest of the uninstall path's best-effort
  deletes, since a chain the operator already removed by hand must not abort levels 2/3 either).

- **[low / confidence: high]** `deploy/styrman-ctl:704` (`du -sh "${STATE_DIR}" 2>/dev/null || true` inside
  `uninstall_level`, level-3 branch) — this line is not wrapped in `run()`, so it executes for real even
  during the plan-preview subshell (`( DRY=1; uninstall_level "${level}" )`), i.e. before the confirmation
  word is ever asked, and again on a plain `uninstall --level 3 --dry-run` that the operator only intended
  to inspect. It is read-only so it changes nothing, but it contradicts the file's own stated contract
  ("Every side effect goes through run(), so --dry-run prints the exact plan … and executes nothing") and
  on a large real `${STATE_DIR}` (many project homes/workspaces) can add a real, possibly slow I/O scan to
  a command the operator expected to be a no-op preview. Wrap it in `run()` (dry-run then just prints the
  `du` line) or drop it from the preview path.

## Not examined

Did not trace `internal/orchestrator`'s own container/dockerd teardown semantics (whether `docker rm -f`
on a Kata-backed `styrman-<uuid>` container cleanly releases its loop-mounted `~/.dockerd/<uuid>.img`,
vs. what the orchestrator's own stop path does) — out of scope of this diff's lines and not named in the
dispatch's focus list. Did not re-verify `deploy/host-setup.sh`'s full inventory beyond the paths the
dispatch named (confirmed units, `styrman-update` binary, `styrman-firewall.sh`, the KSM tmpfiles entry,
`/etc/styrman` match what `uninstall_level` removes); the `containerd-shim-mvm-v2` symlink host-setup.sh
installs into `/usr/local/bin` is untouched by any uninstall level, but that is consistent with the menu
text's promise to keep "Docker and Kata" at every level, so not flagged. Did not run the test suite live.

## Criteria

1. Dead code — pass; no unused code introduced, the old askpass path is fully replaced, not left beside the new helper.
2. Flow tracing — fail: see high finding above (`uninstall_level`'s firewall-chain callees don't match their installer's actual rule shape, and the failure aborts every later step in the same function on the real path, which the test suite structurally cannot exercise).
3. Standards conformance — mostly pass; the credential-helper rewrite matches `host-setup.sh`'s and `styrman-update`'s existing scoped-helper idiom exactly (quoting verified character-by-character), and the origin/token validation additions are sound. The `-D` calls break with the file's own "best-effort, non-aborting" convention used elsewhere in the same function.
4. Directory layout — pass; test additions live in the existing `internal/config/styrman_ctl_test.go`, no new files.
5. Simplicity (DIY-except-security) — pass; wraps existing host-setup/firewall/compose scripts, no new dependency, credential handling stays in the vetted git-credential-helper idiom already used elsewhere (no hand-rolled auth).
6. Decoupling — pass; `uninstall_level`/`cmd_uninstall`/`rm_tree`/`guard_path` are small, single-purpose, and `guard_path`'s three-tree allowlist is respected by every recursive delete.
