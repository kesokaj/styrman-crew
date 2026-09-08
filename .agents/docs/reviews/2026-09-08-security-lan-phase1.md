---
owner: security
date: 2026-09-08
---
# Security review — LAN access Phase 1 (host firewall, installer, updater converge)   (2026-09-08)

Range reviewed: dbe66a8..working-tree, restricted to the 12 files in the supplied
`phase1.diff` (deploy/firewall/*, deploy/host-setup.sh, deploy/update/styrman-update{,.service},
deploy/verify-commit.sh, internal/netpolicy/*). Blast radius followed outward to
`deploy/docker-compose.yml`, `internal/config/config.go`, `internal/project/store.go`
(all three carry uncommitted Phase 2 changes in the same working tree and are NOT part of
this diff — flagged below, not reviewed as such), `internal/config` bridge naming,
the live host's `STYRMAN-FWD`/`INPUT` chains and routing table, and R12 / ADR-065.

## Verdict: FINDINGS

The root installer follows a symlink through a directory a non-root uid owns:
`deploy/firewall/install.sh:97-99` does `mkdir -p "${STATE_DIR}/netpolicy/lan"` and then
`chown`/`chmod` on both levels. `/var/lib/styrman` is 0755 owned by the operator uid (uid 1000
on this host) — the same uid the server container runs as, with that tree bind-mounted rw —
so that uid can pre-create `netpolicy`, or after the first install replace `lan`, as a symlink.
`chown` and `chmod` dereference; the next root run of `install.sh` (host-setup.sh:210, or the
hand-run its own WARN prescribes) hands the symlink's target to that uid. `ln -s /etc
/var/lib/styrman/netpolicy` yields `chown 1000:1000 /etc; chmod 0755 /etc`, and ownership of
`/etc` is root (unlink and recreate `/etc/passwd`, `/etc/sudoers.d/*`, `/etc/ld.so.preload`).
Mechanism verified in scratch: `mkdir -p` creates through the symlink and `chmod` on the
symlink changed the target's mode to 755. Everything else in the diff held up: the conf
grammar is genuinely fail-closed, no lower-privilege input reaches an iptables argv, the
overlap and host-bits arithmetic is correct, `host_converge`'s gate is sound, and the
namespaced test passes 79/79 without touching the host's real chains (re-read before/after).

## Findings

- **[high / confidence: high]** `deploy/firewall/install.sh:97-99` — root `mkdir -p` +
  `chown` + `chmod` on a path whose components are created inside a directory owned by the
  server's uid. Both `chown` and `chmod` follow symlinks, and `mkdir -p` happily traverses a
  symlink-to-directory, so the owning uid picks the target: `ln -s /etc
  /var/lib/styrman/netpolicy` before any run of `install.sh`, or `rm -rf lan; ln -s /etc lan`
  between two runs, gives that uid ownership and 0755 on the target. Scenario: an RCE in the
  styrman server (uid 1000, `/var/lib/styrman` bind-mounted rw per
  `deploy/docker-compose.yml:126`) plants the link and waits for the next `sudo bash
  deploy/host-setup.sh` or `sudo bash deploy/firewall/install.sh` — both of which the diff's
  own docs and its failure message tell the operator to run. Mitigating context, stated
  plainly so nobody over- or under-reacts: that same server container already holds
  `/var/run/docker.sock` and the docker group (`deploy/docker-compose.yml:76,122`), so on the
  deployed topology the actor is already root-equivalent, and on this host uid 1000 also has
  passwordless sudo. That does not make a new root escalation primitive acceptable — it is
  exactly the class `.agents/rules/guest-writable-paths.md` and this script's own comments
  ("the server's uid cannot swap `spool` for a symlink") claim to defend, and it becomes a
  clean escalation the moment the socket mount goes or a second, less-privileged uid owns the
  state dir. Remediation: refuse rather than retarget — `[ -L "${STATE_DIR}/netpolicy" ] &&
  die`, same for `lan`, create each level with plain `mkdir` and assert `stat -c %u` on each
  before chowning; `chown -h` alone is not the fix, because the goal is to detect the attack,
  not to relocate it. `deploy/host-setup.sh:170-171` has the identical shape for
  `homes`/`projects` (pre-existing) — fix both in one pass.

- **[medium / confidence: high]** `deploy/firewall/install.sh:124` —
  `install -d -m 0700 -o root -g root "${ETC_DIR}"` re-modes and re-owns an *existing*
  directory (verified: `install -d -m 0700` on an existing 0755 dir yields 0700). With the
  documented `--etc-dir` override (usage block, line 25), a root invocation silently applies
  `chmod 0700; chown root:root` to whatever it names: `--etc-dir /etc` makes the host
  unusable for every non-root process (`/etc/passwd`, `/etc/nsswitch.conf`, `/etc/resolv.conf`
  become unreadable). This is a verbatim repeat of a finding already recorded and fixed in
  `deploy/update/styrman-update` — see its comment at lines 139-145, "that turned a root
  `--check` against $(mktemp -d) into chmod 0700 root:root of /tmp itself (review+security
  finding, measured)". Remediation: copy the fix that already exists twenty lines away —
  `[ -d "${ETC_DIR}" ] || install -d ...`, then *assert* ownership and mode
  (styrman-update:148-152) instead of forcing them.

- **[medium / confidence: high]** `deploy/firewall/styrman-firewall.sh:84-113` ordered ahead
  of the base section at 115-176; `deploy/firewall/styrman-firewall.path:14-16`;
  `deploy/firewall/styrman-firewall.service:10` — the spool is an unbounded directory on the
  root tmpfs (`/run`, 6.3 GB here) that the server's uid writes with no entry cap and no size
  cap, and everything that touches it runs *before* the R12 base rules. Two exploitable
  shapes. (a) Fill `/run`: `mktemp "${RUN}/.status.XXXXXX"` then fails, `set -eu` aborts the
  run, and the whole host loses `/run` (systemd, journald, containerd, the docker socket's
  directory all live there). (b) Drive root: `StartLimitIntervalSec=0` removes the service's
  start-limit entirely, and the path unit's `TriggerLimitBurst=100/10s` brake, once tripped,
  is released *by the script itself* at lines 402-406 on the next healthy timer run — so a uid
  that keeps the spool non-empty runs a root oneshot at run-duration cadence, re-armable every
  60 s, each pass issuing ~20 iptables/nft transactions that contend with dockerd's own.
  Remediation: cap the spool (one entry is as informative as a million — refuse and warn above
  N), move the lock/drain *after* the base section so nothing a lower-privileged uid controls
  can delay R12 enforcement, and back the revive off instead of re-arming unconditionally.

- **[medium / confidence: medium]** `deploy/firewall/styrman-firewall.sh:299-334` — rules are
  selected by NUMBER in one `iptables -S` (line 320) and deleted by number in separate
  `iptables -D STYRMAN-FWD "${n}"` calls (line 331). `-w 5` serializes each invocation, not
  the read-then-delete pair, and the script's flock only excludes other runs of this script.
  Any other root writer of `STYRMAN-FWD` in between — an operator hand-run, an
  `iptables-restore`, a firewall-manager reload — shifts the numbering, and the delete then
  removes the neighbour. At the head of this chain the neighbour is a base `-j DROP`, so
  cross-project / LAN / metadata reachability opens until the next 60 s tick, and it opens
  *silently*: the delete returns 0, so nothing lands in `ERRORS`. Every well-formed tagged
  rule is fully reconstructible from `RULE_RE`'s five captures, so delete it by full spec
  (`-D STYRMAN-FWD -s .. -d .. -i .. -o .. -m comment --comment .. -j RETURN`), which cannot
  hit the wrong rule; keep the number path only for genuinely malformed lines, and re-read to
  confirm the line still matches before firing it.

- **[low / confidence: high]** `deploy/firewall/styrman-firewall.sh:279-282` — the marker
  directory gets a `-L` check and nothing else, while `${RUN}` two functions earlier (84-96)
  is asserted absolute, non-symlink, root-owned and not group/world-writable. `netpolicy`
  itself is never checked, and the `-L` on `lan` is a TOCTOU against the glob on the next
  line. Impact is bounded — root reads only NAMES here, and the owning uid can create those
  names directly, so nothing is gained — but the asymmetry is the gap that grows on the next
  change. Assert the owner uid and that no component is a symlink, and read through a
  directory fd rather than by path.

- **[low / confidence: high]** `deploy/firewall/test.sh:23-28` — the only barrier between
  `sudo bash deploy/firewall/test.sh` (the shebang and the executable bit invite it) and the
  host's real chains is the heuristic `ls /sys/class/net` equalling `lo `. The `mount -t sysfs
  sysfs /sys` on line 25 runs *before* that check, so a real-root run stacks a second sysfs on
  the host's `/sys` and never unmounts it (the `trap ... rm -rf` is installed later and does
  not cover the mount). ADR-052's rule is that a privileged test must not be able to alias
  production. Make the first assertion definitive and put it before the mount:
  `[ "$(readlink /proc/self/ns/net)" != "$(readlink /proc/1/ns/net)" ] || exit 2`.

- **[low / confidence: medium]** `deploy/firewall/styrman-firewall.sh:126-153` — the base
  section's iptables calls carry no `-w`, while every LAN-section call carries `-w 5`. On the
  nft backend a concurrent dockerd transaction can fail one of them; under `set -eu` that
  aborts the run at the base section, so the LAN reconcile at 178-400 never executes and a
  project whose marker was just *deleted* keeps its RETURN rule. A stale `applied_at` does
  surface it to the server, so it fails visible rather than silent — but add `-w 5` to the
  base calls for consistency.

- **[low / confidence: high]** `deploy/firewall/styrman-firewall.sh:369-380` — the status file
  is 0644 in `/run` and enumerates every LAN-enabled project UUID plus `lan_dev` and
  `lan_cidrs`. Readable by every local uid and by anything that mounts the run dir. 0640
  root:<operator gid> still satisfies the server's read.

- **[low / confidence: high]** `deploy/firewall/styrman-firewall.sh:222` / ADR-065 §5 — the
  ADR keeps the host closed partly because "the project holds its owner's SSH key through the
  broker", and then does not follow that one hop further: a LAN-enabled project holds that key
  *and* can now reach every host on 192.168.86.0/24, several of which can SSH back to this
  machine, where the operator has passwordless sudo. `STYRMAN-IN` correctly blocks the direct
  path (verified live: `-A INPUT -i sdb-+ -j STYRMAN-IN`, chain is a bare DROP), and DNAT'd
  published ports are correctly out of reach because the `-d` match sees the post-DNAT
  address. The transitive path is an accepted-risk framing rather than a code defect, but it
  belongs in the operator-facing text for the toggle; consider narrowing what the credential
  broker hands a LAN-enabled project.

- **[low / confidence: low]** `deploy/firewall/styrman-firewall.sh:222-236` — the overlap
  check against Docker bridges only sees bridges that exist at validation time. What actually
  stops a later project bridge from being allocated inside an approved LAN prefix is Docker's
  own refusal to pick a pool that collides with an existing host route, not this script.
  Pinning `default-address-pools` to 172.16/12 in `daemon.json` would make it structural
  rather than dependent on Docker's behaviour.

- **[low / confidence: high]** working-tree hygiene — `deploy/docker-compose.yml`,
  `internal/config/config.go` and `internal/project/store.go` carry uncommitted Phase 2
  changes that are *not* in `phase1.diff` but are in the same tree, including the rw bind of
  `/run/styrman-firewall` into the server container. They were read for blast radius (the bind
  matches the design: root 0755 dir, uid-owned 0700 spool, root 0644 status) but not reviewed.
  Do not let them ride in on Phase 1's commit unreviewed.

## Checked

- **Secrets** — none introduced. `/etc/styrman/firewall.conf` holds only `LAN_DEV` and
  `LAN_CIDRS`; `install.sh` echoes it to stdout, which is correct (no secret) and deliberate.
  The status file leaks project UUIDs and topology at 0644 (low finding above). No credential,
  token or key appears in the diff, and nothing echoes one into a log or an error.
- **Injection** — no lower-privilege input reaches an iptables argv, a filename, a shell word
  or the `find -delete`. Marker names pass an anchored `^[0-9a-f]{8}-...$` and are then only
  hashed (`BridgeNameFor` parity confirmed against `internal/config/config.go:471` and the
  shared golden vector `sdb-e660d58459e`); the bridge subnet passes an anchored CIDR regex;
  spool entry names are never interpreted. The conf grammar is empirically fail-closed —
  `LAN_DEV=$(reboot)` is rejected, and `;`, `\`, `$`, quotes and newlines all fall outside the
  value character class (tested directly). `write_status`'s `tr -c` sanitizes the status file
  to a fixed charset. `find -P ... -delete` does not follow symlinks (covered by test 8).
- **Authz/authn** — no HTTP surface in this diff. `host_converge`'s gate is sound: it refuses
  unless `REQUIRE_SIGNED=1`, pins the target basename, requires `trusted_dir` on both
  destinations, requires `HEAD` to equal the SHA this host recorded when the deployed tag
  verified, and reads content only from `git show <sha>:path` — never the mutable worktree.
  The pre-existing `.git` ownership and mode assertions (styrman-update:165-175) still guard
  the object store it resolves against. The `RETURN` allowance is correctly scoped: `-i` pins
  the bridge (a guest with NET_ADMIN cannot change which host bridge its packets arrive on),
  `-s` pins its own subnet so a spoofed LAN source does not match, `-o` pins the uplink so a
  destination that routes to any other bridge falls through to the DROPs, and the host itself
  stays on the INPUT path behind `STYRMAN-IN`.
- **Crypto** — nothing hand-rolled. `sha1sum` is used only as a non-security interface-name
  derivation, matching `internal/config.BridgeNameFor`; the trust decision in the updater
  rests on `git verify-tag` with an SSH allowed-signers file, unchanged by this diff.
- **Dependencies** — no dependency change (`go.mod`, `go.sum`, `web/package*.json` untouched).
  `bash deploy/vulncheck.sh` run for all three modules: `image/harness` clean; the root module
  and `deploy/caddy/ingress` each report GO-2026-4887 and GO-2026-4883 in
  `github.com/docker/docker` (v28.5.2 / v27.5.1, both "Fixed in: N/A"), reached via `init`
  traces from the docker client. Pre-existing, no upstream fix available, not touched or
  worsened by this diff. `npm audit` not run: the diff contains no frontend change and
  `web/package*.json` is unmodified.
- **GCP surface** — not applicable; styrman is self-hosted (`.agents/docs/deploy/gcp.md` says
  so). No IAM binding, service account, `--allow-unauthenticated`, Secret Manager path or
  service-account key file appears in the diff.
- **Tests run** — `unshare -Urnm bash deploy/firewall/test.sh`: 79 passed, 0 failed;
  `go test ./internal/netpolicy/`: ok; `go vet` clean on the three touched packages. The
  host's `STYRMAN-FWD` was read before and after and is byte-identical (still the pre-ADR-065
  four-DROP script from 2026-08-30 — the drift ADR-065 exists to fix is live right now).
- **BUG-007** — `git diff --numstat` on the whole change set is fully numeric; no source file
  in this diff is classified binary, and this findings file contains no control bytes.
