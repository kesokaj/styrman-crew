---
owner: security
date: 2026-09-08
---
# Security review — release gate v1.23.0 (per-project LAN access, ADR-065)   (2026-09-08)

Range reviewed: v1.22.0..bdc15b7   (209edce..bdc15b7, 13 commits, 116 files)

## Verdict: FINDINGS — 0 high, 1 medium, 5 low. Ship-able.

Nothing in this range breaks R12's unconditional clauses. I traced the composed
chain order against this host's live kernel state (read-only) and the exception
is enforced by **two independent guards**, either of which alone would hold:
the conf validator refuses any `LAN_CIDRS` overlapping 172.16/12, 169.254/16,
100.64/10, 198.18/15, 127/8, 0/8, 224/3 or any Docker bridge's connected route,
**and** every allow rule carries `-o <LAN_DEV>`, so a packet that is not leaving
the uplink can never match it. The host stays protected by a different chain
(`INPUT → STYRMAN-IN → DROP`) that the LAN rule does not touch, and
host-published ports are caught post-DNAT by the `172.16/12` drop. The design is
fail-closed at every hinge I could find: a missing conf, an invalid conf, a
missing marker dir, a stale status, an absent host half and a flushed ruleset all
*remove* access, never grant it.

The one medium is a disclosure gap, not a code defect: the admin-facing dialog
and R12 both say what a LAN-enabled project can *reach*, but nowhere is it
stated that its traffic is **SNAT'd to the host's own LAN address**
(`-A POSTROUTING -s 172.20.0.0/16 ! -o sdb-e660d58459e -j MASQUERADE`, live).
Every LAN ACL keyed on the styrman host's IP — a NAS export, a router admin
allowlist, another box's firewall — therefore extends silently to whatever the
agent runs, and the LAN peer cannot tell the two apart. That is the actual
blast radius an admin is consenting to, and one sentence fixes it.

## Findings

- **[medium / confidence: high]** `REQUIREMENTS.md:82` (R12 amendment),
  `.agents/docs/architecture/DECISIONS.md` ADR-065 part (5),
  `web/src/components/LocalNetwork.tsx:132-143` — **the SNAT identity of
  LAN-enabled project traffic is nowhere disclosed.** All three documents
  enumerate what stays blocked and the dialog correctly warns that the project
  "holds your git, SSH and cloud credentials through the broker", but none says
  that Docker's MASQUERADE rewrites the source to the host's LAN address before
  the packet leaves `enp114s0`. Exploitable in principle: a LAN device that
  grants access by *address* rather than by credential — an NFS/SMB export
  pinned to 192.168.86.58, a router admin page restricted to the server, an
  `sshd` `Match Address` block, a Home Assistant trusted-network login — treats
  the agent as the styrman host itself, with no credential needed and no way for
  the peer to distinguish them. The dialog's "any LAN machine that trusts them"
  understates this: address trust is transferred too, not only credential trust.
  Remediation: one clause in R12's exception paragraph, in ADR-065 (5), and in
  the `ConfirmDialog` body — "traffic leaves as the server's own LAN address, so
  anything on the LAN that trusts this machine by IP now trusts this project".
  No code change.

- **[low / confidence: medium]** `deploy/firewall/install.sh:119-130` —
  **BUG-009's symlink race is narrowed, not closed.** `chown -h` (line 128) is
  correct, but `chmod 0750 "${d}"` (line 129) still dereferences, and the loop's
  second iteration (`d = <state>/netpolicy/lan`) only re-checks its own leaf —
  it never re-verifies that the parent `netpolicy` is still a real directory
  after the first iteration validated it. `/var/lib/styrman` is `0755` owned by
  uid 1000 (verified: `stat -c '%u %g %a'` → `1000 1000 755`), so that uid can
  swap `netpolicy` for a symlink between iterations. Exploitable in principle:
  a root `chmod 0750` on an attacker-chosen path, and a root `mkdir` (line 122)
  that creates a directory named `lan` under an attacker-chosen parent, then
  chowns it to uid 1000. Neither is a clean escalation, and the window only
  opens while an operator is hand-running `install.sh` — it is not on the timer
  path. Remediation: hold an `O_PATH|O_NOFOLLOW` fd per level (or `openat2`
  with `RESOLVE_NO_SYMLINKS`) and operate through `/proc/self/fd/N`, or at
  minimum re-run the `-L`/`-d`/owner assertion on every component at the top of
  each iteration and drop the bare `chmod` in favour of `mkdir -m` on create
  only.

- **[low / confidence: medium]** `deploy/firewall/styrman-firewall.sh:74,76` vs
  `:82,99` — **the legacy migration deletes run before the replacement jumps,
  contradicting the file's own promise at lines 12-13** ("every rule is
  INSERT-IF-MISSING, never delete-then-insert, so the 60 s timer re-runs open no
  unprotected window") and ADR-036's "never delete a live rule". Line 74
  unconditionally deletes `INPUT -i sdb-+ -j DROP` and lines 75-77 delete the
  four legacy `DOCKER-USER` drops; the equivalent coverage is only restored at
  lines 82 and 99, roughly ten `iptables` calls later. With `-w 5` on every call
  and `TimeoutStartSec=30s` in the unit, a run under xtables-lock contention can
  be SIGTERM'd inside that window (`trap 'exit 143' TERM`), leaving a host with
  no project-bridge isolation until the next tick. Exposure is one first
  migration run per host — on an already-migrated host (this one: `-A INPUT -i
  sdb-+ -j STYRMAN-IN` present, no direct DROP) both are no-ops. Pre-existing
  (moved verbatim from `host-setup.sh`), but this range re-asserted the claim
  while relocating the code. Remediation: move the two migration deletes to
  *after* the `ensure_jump` calls, or delete them outright — every live host is
  long past that migration — and raise `TimeoutStartSec` above
  `5s × <iptables call count>`.

- **[low / confidence: high]** `internal/config/config.go:160` +
  `cmd/styrman/main.go:353-376` — **`FirewallRunDir` defaults to the production
  `/run/styrman-firewall` in code**, the single ADR-052 "no production-path
  default" exception in this range (the working tree already carries an
  uncommitted comment weighing it; a comment is not a guard). A host-process
  dev or test run on the production machine with default env reads root's real
  status file and writes trigger files into the real spool. Worse in
  combination: `wantLAN` is built from `forEachProject`, which logs and
  `continue`s past any user whose `projects.json` fails to load
  (`main.go:368-371`), and `Dir.Sync` then **removes the markers of every
  project it did not see** — so a transient store read error, or a stale binary
  that cannot parse a record, silently revokes a live LAN allowance and root
  closes the rule within 60 s. Fail-closed (it revokes, never grants), which is
  why this is low. Remediation: have `Sync` refuse to run its delete pass when
  `forEachProject` reported any per-user failure (propagate an error instead of
  swallowing it), and consider requiring `STYRMAN_FIREWALL_DIR` explicitly the
  way the root-run script and the updater already require their paths.

- **[low / confidence: medium]** `deploy/update/styrman-update:488-491` —
  **`host_converge` stops converging silently whenever `HEAD` ≠ the recorded
  deployed SHA**, which contradicts its own comment ("on EVERY tick — before the
  'already up to date' exit as well as after a deploy or rollback"). Any hand
  `git checkout` in `/opt/styrman`, or a failed deploy whose rollback checkout
  also failed, pins HEAD away from `${STATE_SHA}` and the host then runs the old
  root firewall script indefinitely, with only a per-tick `log WARN` in the
  journal — exactly the silent-drift class ADR-065 part (6) exists to close (the
  2026-08-30 vs 2026-09-02 four-versus-six-DROP drift). The gate itself is
  correct and fail-closed: `REQUIRE_SIGNED=1`, `trusted_dir` on both targets,
  the basename pin, a 40-hex recorded SHA, and reads via `git show
  "${SELF_HEAD}:…"` from the verified object rather than the mutable worktree —
  and I confirmed `--check` returns before `rm -f *.new.*`, `atomic_install` and
  every `systemctl` call (BUG-008 properly closed), with nothing state-changing
  ahead of that guard inside the function. Remediation: escalate a repeated
  skip — surface it in the status the operator already reads, or refuse to
  report the release as converged — rather than emitting a WARN nobody watches.

- **[low / confidence: high]** `web/dist/client/_shell.html` (offset 7417) —
  **a raw NUL byte makes a `go:embed`ed, browser-served asset invisible in every
  diff.** `git diff --numstat` reports `-` in both columns and `file(1)` says
  `data`, so this range's UI phase (03fedc8) and its reviewers saw only "Binary
  files differ" for a file shipped to every user's browser —
  `.agents/rules/writing-files.md` says that is not a review. This is **not**
  BUG-007 recurring: the NUL is generated by the TanStack build
  (`{i:"__root__\x00"` in the router serialization) and traces back through at
  least fifteen commits, well before v1.22.0. I decoded both revisions and
  diffed them myself: the only changes are asset content hashes and a build
  timestamp — no new script source, no external URL, no inline handler, no
  credential. Remediation: strip the NUL in the build step, or add a
  `.gitattributes` entry so the file diffs as text; until then a reviewer of
  this path must decode it explicitly.

## What was NOT checked

- **The live host half was never installed or exercised.** Phase 5 is open; this
  host still runs the pre-ADR-065 script (`STYRMAN-FWD` carries four DROPs, not
  six, and no `styrman-lan:` rule exists). All chain-order reasoning is from
  read-only `iptables -S` / `ip -4 route` plus the script's source. I did not
  run the reconciler, install anything, or touch `/var/lib/styrman`,
  `/etc/styrman` or `/run/styrman-firewall`.
- **No exploit was built** for any finding; the install.sh race and the
  migration window are reasoned from the code and from verified directory
  ownership, not demonstrated.
- **IPv6 was reviewed but not tested.** `STYRMAN-FWD6` drops only `fc00::/7` and
  `fe80::/10` — global unicast is not dropped — mitigated by `disable_ipv6=1`
  per bridge. Unchanged by this range (moved verbatim); flagged here only so it
  is on the record.
- **The vendored Caddy ingress** (`deploy/caddy/ingress`) is untouched by this
  range; I ran its govulncheck but did not read its code or run its tests.
- **Browser e2e / integration suites** were not run (`web/e2e/run.sh`,
  `integration.sh` need a live Kata host and a built SPA).
- **`web/dist/` bundles** were not audited byte-by-byte beyond `_shell.html`;
  they are generated output.
- **Working tree is dirty**: another agent left uncommitted edits to
  `internal/config/config.go` (comment only), `internal/netpolicy/dir.go`
  (removes the now-unused `MarkerDir()`) and a testing README count, plus its
  own review file. Reviewed for impact — none — but they are outside the
  committed range and were not part of this gate.
- **Cross-user `PUT /api/projects/{id}/network`** is covered by
  `network_test.go:128-133`, but `isolation_test.go` only added the **GET** path
  to its cross-user list. Correct by construction (`getProjectFor` resolves in
  the caller's own store), so not a finding — noted so the gap in the canonical
  isolation test is on the record.

## Checked

- **Secrets** — clean. `git diff v1.22.0..HEAD | grep -nE 'BEGIN |AKIA|AIza|ya29\.|token=|password=|PRIVATE KEY'` returned nothing; a wider sweep (`secret|passphrase|api_key|bearer|ssh-rsa|ssh-ed25519|gh[pousr]_|glpat-|xox[baprs]-|sk-`) matched only prose in `.agents/docs/**`. No base64-shaped literal ≥32 chars in added non-doc code. `git ls-files` finds exactly one secret-shaped tracked path in the whole tree — `deploy/.env.example`, a template — and no `.env`, `.pem`, `.key`, `.tfstate`, `.tfvars` or credential JSON entered the tree in this range. `/etc/styrman/firewall.conf` holds only `LAN_DEV`/`LAN_CIDRS`; the status file is `root:<state-gid> 0640` and the marker dir `0750`, so no other local uid enumerates LAN-enabled projects.
- **Injection** — no path found from a lower-privileged uid into an iptables argv, a shell word or a `find -delete` target. The only server-controlled input root parses is a **directory entry name**, gated by `^[0-9a-f]{8}-…-[0-9a-f]{12}$` + `-f` + `! -L` (`styrman-firewall.sh:305`); the interface is recomputed by root from that uuid (golden vector pinned on both sides — `internal/config/bridgename_test.go` and `deploy/firewall/test.sh`), the subnet from `ip route`, and the device and prefixes from the root-owned conf, so every argv component is root-derived or regex-bounded. `write_status` sanitizes through `tr -c 'A-Za-z0-9 ._:/=\n-' '_'` and never echoes untrusted rule text. `find -P … -delete` on the spool never interprets names; its root-owned `0755` parent prevents swapping `spool` for a symlink, and `[ -L ]` catches it anyway. A 44-bit truncated-SHA1 interface collision is reachable in theory but grants the *victim* project access, not the attacker's — no gain. `deploy/firewall/test.sh`'s hostile cases run for real here: `TestFirewallScriptInNetns` **PASSED (5.63s)**, not skipped.
- **Authz/authn** — correct. Both new routes sit on the same mux behind the existing guard. `getProjectFor` resolves the project in the **caller's own** store, so another user's project 404s before any authz signal; enable additionally requires `requireAdmin` and an admin has no path into another tenant's project, so a non-admin can never obtain LAN access and can always revoke it. Enable is refused 503 whenever root's word is absent, stale, future-dated, outdated (`version < MinScriptVersion`) or unconfigured; `Derive` reports from root's status, never the request lifecycle, and a nil status with the record on reports `error`, not `off` (fail-closed). Tenant-facing messages are genericized (`network.go:70-84`); `host` and `settings.hostFirewall` are admin-only.
- **Crypto** — none introduced. No hand-rolled auth, hashing, session or JWT/OIDC code in the range. SHA-1 appears only as a non-security interface-name derivation (an 11-hex truncation for the 15-char IFNAMSIZ limit), not as an integrity or authentication primitive. Release trust rests on `git verify-tag` with SSH signatures pinned to a root-owned `allowed_signers`, with PGP explicitly refused because it would not consult the allowlist — a vetted-tooling path, correctly reasoned.
- **Dependencies** — no change: `git diff v1.22.0..HEAD -- go.mod go.sum web/package*.json deploy/caddy/ingress/go.{mod,sum}` is empty. `bash deploy/vulncheck.sh` → exactly the accepted baseline and nothing else: **GO-2026-4887** and **GO-2026-4883**, both `github.com/docker/docker@v28.5.2+incompatible`, both `Fixed in: N/A`, in the root module and the ingress module; `image/harness` clean. `npm audit` and `npm audit --omit=dev` in `web/` → **0 vulnerabilities**. `go vet ./...` clean; `go test` green for `netpolicy`, `httpapi`, `project`, `config`.
- **GCP surface** — not applicable and none added. This is a self-hosted deployment (`.agents/docs/deploy/gcp.md` says so); the range introduces no IAM binding, service account, key file, `--allow-unauthenticated`, Secret Manager reference or Artifact Registry change. The only credential-on-disk in the delivery path is the fine-grained GitHub token at `${GH_TOKEN_FILE}`, asserted root-owned and `0600` and bound to a `https://github.com`-scoped credential helper — unchanged by this range.
