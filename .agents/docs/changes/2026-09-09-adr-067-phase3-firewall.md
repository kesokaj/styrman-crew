---
owner: main
date: 2026-09-09
---
# ADR-067 Phase 3 — the root firewall shrinks to its one job

`deploy/firewall/` goes from **1,637 to 439 lines**. What is left does exactly
what it did before ADR-065: keep every `sdb-*` bridge off the host and off the
private ranges, re-asserted every 60 s.

| file | before | after |
|---|---|---|
| `styrman-firewall.sh` | 794 | 163 |
| `install.sh` | 355 | 119 |
| `test.sh` | 435 | 130 |
| `styrman-firewall.path` | 19 | **deleted** |
| `styrman-firewall.service` | 24 | 17 |

Gone from the script: the three rule families and their generic reconcile, the
conf `LAN_*` parsing and validation, the marker directory read, the spool
drain and its EXIT trap, the status file, the lock, the path-unit revival, and
`STYRMAN_FW_VERSION` (nothing gates on a version any more). Root now parses
**no input from the server at all**.

## The installer removes what it used to install

An upgrading host still carries the `.path` unit, the bounded spool tmpfs
mount, its tmpfiles entry, the run dir and the service drop-in. A leftover
`.path` unit watching a directory nothing writes is a unit that fails on every
boot, so `install.sh` now disables and removes all five — in that order,
because deleting a unit file while systemd still has it running leaves an
orphan. Verified by `--dry-run` against this host, which correctly identified
every artifact it carries.

`/etc/styrman/firewall.conf` is deliberately **left alone**: this script does
not edit an operator-owned file, and its now-unread keys are inert. The
installer says so rather than silently rewriting it.

Obsolete flags (`--lan-dev`, `--lan-cidr`, `--lan-allow-host`,
`--lan-allow-published-ports`, `--ingress-bridge`, `--state-dir`,
`--i-accept-sshd-passwords`) are accepted and ignored with a note, so a stale
runbook or an old `host_converge` invocation does not fail the install.

## The stale-rule cleanup, and the two bugs the test caught

The script removes any `styrman-lan:` / `styrman-lanhost:` / `styrman-lanpub:`
rule left by v1.23/v1.24. Writing it surfaced two defects, both caught by
`test.sh` rather than by review:

1. **Naive tokenizing does not survive `iptables -S`.** It quotes the comment,
   so word-splitting hands iptables a token with literal quotes and the delete
   fails silently. Worse, a comment containing spaces would let a planted rule
   inject argv. The rule is now matched by a strict whole-line regex and
   rebuilt from the captured groups, with the pre-comment args group
   forbidding `"` so no second quoted match can hide inside it.
2. **A test that read host state, not script behaviour.** `[ ! -e
   /run/styrman-firewall ]` passed or failed depending on what the host
   happened to have, since the namespace does not get a private `/run`.
   Deleted.

A third failure was a test artifact worth recording: `set -o pipefail` made
`run 2>&1 | grep -q ...` fail even when grep matched, because the script
deliberately exits 1 when it has warnings.

The cleanup still refuses to delete a `-j DROP` whatever its comment says, and
never deletes by rule number (BUG-013).

## The behavioural test kept its Go runner

`deploy/firewall/test.sh` was invoked under `go test` by
`internal/netpolicy/script_test.go`, which Phase 2 deleted — so the real
script would have stopped being exercised by the suite. The runner moved to
**`internal/config/firewall_script_test.go`**, the package that owns
`BridgeNameFor`, since the script's whole job is matching the prefix that
function produces. It brings a replacement for the deleted version handshake:
`TestFirewallScriptMatchesBridgePrefixes` asserts the script carries the
`sdb-+` jumps *and* that no line of its CODE names the `lan-` prefix. If
`BridgeNameFor`'s sandbox prefix ever changes without the script's wildcard
changing with it, every project silently loses isolation — that is now a test
failure rather than a review's responsibility.

`test.sh` itself grew the assertion that states ADR-067 at the rule level: a
`lan-<hash>` dummy interface exists in the namespace and **no rule anywhere
names it**, while the `sdb-` wildcard covers the sandbox one. 24 checks, all
passing under `unshare -Urnm`.

## Also fixed: two things that would have broken the release

- **`deploy/verify-commit.sh:162`** ran `systemd-analyze verify` on
  `./styrman-firewall.path`. Deleting that unit would have failed the release
  gate itself — found by the cold reader, not by any build.
- **`deploy/update/styrman-update`** `host_converge` installed and enabled the
  `.path` unit in five places. It now handles three files, with a comment
  saying removal is `install.sh`'s job: the updater installs what the verified
  tree carries and does not delete host state on its own.

`host-setup.sh` §5c was rewritten to describe the two-file reality.

## Verification

`bash -n` and `shellcheck -S warning` clean across every host script;
`systemd-analyze verify` accepts both remaining units; `install.sh --dry-run`
correct against this host; `deploy/firewall/test.sh` 24/24 under a namespace;
`go test ./... -race` 194 tests (192 + 2 new), 15 packages ok.

**Deferred to Phase 6 on purpose:** converging THIS host. Production runs
v1.24.1, whose server still reads `/run/styrman-firewall/status`; removing it
now would make the live server report "unavailable" for a feature nothing has
enabled. Host and server should move together at deploy time, so the plan's
live-converge box stays unticked until then.
