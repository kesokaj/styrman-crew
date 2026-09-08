---
owner: reviewer
date: 2026-09-08
---
# Review — release gate v1.23.0 (whole range)   (2026-09-08)

Range reviewed: v1.22.0..bdc15b7

## Verdict: APPROVE WITH NITS

Nothing in this range is broken in a way that reaches a user or a tenant. The
cross-phase seams the gate exists to check all hold: the bridge-interface
derivation is byte-identical in shell and Go and pinned by a shared golden
vector on both sides; `netpolicy.MinScriptVersion` (3) equals the script's
`STYRMAN_FW_VERSION` (3) with a repo-level drift test enforcing it; the
`key=value` status grammar the script writes is exactly what
`internal/netpolicy/status.go` parses, including the `pending=<uuid>:<reason>`
and `error=<uuid>:<text>` sub-grammars and the `lan=` "all rules present"
rule; and the Phase 2 `internal/httpapi` additions and the earlier exec-seam
commit touch the same files (`server.go`, `explorer.go`) without contradiction
— the seam's `s.guest` substitution and Phase 2's `lanFields`/`hostFirewall`
additions are independent edits in the same functions and both compile and
test clean. The worst thing I found is a documentation source-of-truth that
now contradicts itself about how many e2e checks exist, which is exactly the
signal a verifier is told to trust; the rest is one dead method and one
in-code default that breaks the ADR-052 shape the rest of this same range
established.

## Findings

- **[medium / confidence: high]** `.agents/docs/testing/styrman/README.md:240`
  vs `:328` — the same file now states two different totals for Tier 4.
  Line 240 (updated by Phase 3, 03fedc8) says "Playwright, 86 checks + a final
  client-error gate; firewall-hardened hosts run 84 + 2 auto-SKIP"; line 328,
  in the Tier 4 body, still says "82 checks + the client-error gate (80 pass on
  firewall-hardened hosts, plus 2 auto-SKIP)" — the pre-ADR-065 number, which
  both places carried at v1.22.0. A static count of `await check("` in
  `web/e2e/run.mjs` gives 84 call sites (3 of them new in this range), so
  neither stated total is obviously the right one. `AGENTS.md` names this
  README as the source of truth for check counts twice, and line 240 itself
  ends with "re-measure after any further check additions before citing a pass
  count as current". Concrete failure: the next verifier or release gate reads
  the Tier 4 body (line 328), runs `web/e2e/run.sh` on this firewall-hardened
  host, sees 84 passes against an expected 80, and either reports a phantom
  regression or — the dangerous direction — a later run that silently stops
  executing four checks lands exactly on the stale "80 pass" number and is
  waved through. Note this is the same defect class commit 2ef1e51 in this very
  range had to fix once already ("a stale test count"). Fix: re-measure once
  and write the one number into both places, or make line 328 point at line 240
  instead of restating a count.

- **[low / confidence: high]** `internal/netpolicy/dir.go:81` —
  `func (d *Dir) MarkerDir() string` is dead. Zero callers anywhere in the
  repo, including tests; `cmd/styrman/main.go:46` creates the directory from
  `cfg.NetPolicyDir()` directly, so its own doc comment ("created by main at
  boot") describes a caller that does not exist. It is an exported method on a
  type whose whole contract is "the server may only say on/off per project",
  and it widens that surface to hand out a filesystem path for no consumer.
  Concrete failure: it is an invitation — the next change that wants the
  marker path reaches for this accessor and starts doing path work outside
  `netpolicy`, which is precisely the coupling `netpolicy.Dir` was built to
  prevent. Delete it.

- **[low / confidence: medium]** `internal/config/config.go:160` —
  `FirewallRunDir: envOr("STYRMAN_FIREWALL_DIR", "/run/styrman-firewall")`
  gives the server an in-code default that aliases the production root-owned
  run dir. Every other new path in this same range deliberately refuses that
  shape: `deploy/firewall/styrman-firewall.sh:27-31` states "Paths come from
  the environment with NO defaults (ADR-052)" and applies base rules only when
  they are unset, and `deploy/update/styrman-update.service:15` adds
  `STYRMAN_FW_INSTALL_BIN` with the comment "same ADR-052 shape: no in-script
  default". Phase 2 then introduced a default for the same family of path, and
  Phase 4 had to paper over it with a warning in
  `.agents/docs/deploy/local.md:29` telling the operator to point it at a
  scratch dir. Concrete failure: this repo lives on the production host, and
  `AGENTS.md`'s documented local run is `STYRMAN_IMAGE=styrman-runtime:dev go
  run ./cmd/styrman/` with no `STYRMAN_FIREWALL_DIR`. That dev server — whose
  loopback dev door resolves to the first allowed email, i.e. an admin — reads
  the live `/run/styrman-firewall/status` and renders the operator's real
  `lan_cidrs`, `lan_dev` and the count of LAN-enabled projects in Settings,
  and drops trigger files into root's live spool on any project PUT. Both
  `web/e2e/run.sh:53` and `integration.sh:66` already have to set the variable
  explicitly to avoid it, which is the tell. Fix: default it to empty (feature
  off unless configured), leaving `deploy/docker-compose.yml:111` as the only
  place the production path is named.

Not findings, checked and clean (recording them so the gate does not have to
re-derive them): the bridge name — `printf 'sdb-%s' "$uuid" | sha1sum | cut
-c1-11` in `styrman-firewall.sh:308` and `test.sh:47` versus
`config.BridgeNameFor` in `config.go:472`, both pinned to `sdb-e660d58459e`
by `internal/config/bridgename_test.go` and `deploy/firewall/test.sh`;
`MinScriptVersion`/`STYRMAN_FW_VERSION` with `TestMinScriptVersionMatchesScript`
as the guard; the status grammar round-trip including the `tr -c` sanitiser in
`write_status` (its allowed set keeps `= . : / -` and space, so no line the
script emits can trip `ParseStatus`'s "line without '='"); the marker-dir mode
0750 agreeing across `install.sh:120`, `main.go:46` and `dir.go:89`; the
container uid/gid (`docker-compose.yml:75`) matching the state dir owner
`install.sh` derives the spool and status group from; the `host_converge`
ordering in `deploy/update/styrman-update` — `STATE_SHA` is written before the
post-deploy `host_converge` and left untouched on the rollback path, so the
HEAD-equals-verified-sha gate holds in both branches; and `s.netpol.Kick`/
`KickThrottled` tags (project uuids, `boot`, `adopt`) all matching
`kickTagRe`.

## Criteria

1. **Dead code** — one finding: `netpolicy.Dir.MarkerDir()` has no callers.
   Nothing else orphaned; the old inline firewall heredoc was properly deleted
   from `host-setup.sh` rather than left commented, and no removed exec call
   site was left behind by the seam change.
2. **Flow tracing** — pass. Traced record → marker → spool → root → status →
   `Derive` → `projectView`/`lanView` → `lanChip`/`lanLine`, both directions,
   for every state. Enable is gated on `Availability` under `netMu`, the store
   write rolls back to the exact prior `(value, timestamp)` pair when
   `SetMarker` fails, delete removes the marker under the same mutex before the
   record, and the boot/2-minute `Sync` is fail-closed on an unreadable store
   and self-heals on the next tick. `pending=no-bridge` for a stopped project
   maps correctly to "applies when the project next starts". Errors are wrapped
   or logged on every path; no discarded `err`.
3. **Standards conformance** — pass. `go build`, `go vet`, gofmt,
   `go test -race`, the ingress module, `npm run check`/`lint`/`test` (80/80),
   `shellcheck -S warning`, and `unshare -Urnm bash deploy/firewall/test.sh`
   (83/83) are all green (I ran the first eight myself before the cut-off; the
   coordinator vouches for the full set on this tree). No hand-rolled auth,
   crypto or session code anywhere in the range.
4. **Directory layout** — pass. `internal/netpolicy/` is a new package in the
   right place with tests beside the code; the shell half is confined to
   `deploy/firewall/`; UI stays inside `web/`; the pure LAN derivations sit in
   `web/src/lib/lan.ts` with `lan.test.ts` next to them.
5. **Simplicity (DIY-except-security)** — pass. No new Go or npm dependency.
   The privileged half is a root systemd-owned shell script and the server
   only ever writes a filename, which is the simplest split that keeps the
   server unprivileged. `guestRuntime` is a two-method interface with a real
   test double, not speculative.
6. **Decoupling** — pass, and improved. `guestRuntime` gives `internal/httpapi`
   a testable exec seam without importing orchestrator internals, and
   `netpolicy.Dir` is the only thing that knows the marker/spool/status paths.
   The one leak in that boundary is the dead `MarkerDir()` accessor above.

## Text/encoding integrity

`git diff --numstat v1.22.0..HEAD` reports exactly one path with `-` in both
columns: `web/dist/client/_shell.html`. It was already binary at v1.22.0
(`git show v1.22.0:web/dist/client/_shell.html | file -` → `data`), it is
generated build output under `web/dist/client/**`, and it is therefore the
known pre-existing case named in the dispatch — **not** a new BUG-007-class
regression. No source file in the range is binary to git.

## What I did NOT check

The gate report must carry these as gaps, not imply coverage:

- **Browser e2e was not run.** `bash web/e2e/run.sh` (Playwright) was never
  executed in this review. That means the three new ADR-065 e2e checks are
  unverified by me, and the check-count contradiction in finding 1 is
  established from the README's own two statements plus a static count of
  `await check("` call sites in `web/e2e/run.mjs` — I did not measure the
  runtime total, so I cannot say which of 82/84/86 is correct, only that the
  document cannot be internally right.
- **`integration.sh` was not run** (Tier 2, real Kata). Its 91 new lines were
  read, not executed. A static count of `check "` in the file gives 86
  invocations against the 82 claimed in `AGENTS.md:58` and README line 9;
  because several are inside `if`/`else` arms that resolve at runtime I did not
  raise it as a finding, but the Tier 2 number is also unconfirmed. The opt-in
  `STYRMAN_IT_LIVE_LAN=1` block was not exercised.
- **Nothing was exercised against the live host firewall.** Per the dispatch I
  did not touch `/var/lib/styrman`, `/etc/styrman` or `/run/styrman-firewall`,
  did not run as root, and built no Docker image. So the end-to-end claim —
  toggle on, rule inserted at the head of `STYRMAN-FWD`, guest reaches the LAN
  gateway, host still unreachable — rests entirely on the namespaced
  `deploy/firewall/test.sh` and the manual log in
  `.agents/docs/testing/2026-09-08-verify-lan-access.md`, neither of which I
  re-verified.
- **`deploy/firewall/install.sh` and `host_converge` were read, never run**,
  not even `--dry-run`. The tmpfiles entry, the spool tmpfs mount unit and the
  `firewall.conf` discovery path are unproven by execution; Phase 5 (host
  install) is explicitly still open per handoff bdc15b7.
- **`deploy/verify-commit.sh` was not run by me** on any commit (the
  coordinator vouches for it on 3aa7acf, not on HEAD/bdc15b7).
- **No `govulncheck`** (`deploy/vulncheck.sh`) and no dependency review; the
  range adds no dependency, but I did not confirm that from `go.mod`/`go.sum`
  diffs.
- **`web/dist/client/**` generated assets were not audited** beyond the binary
  check above — I did not confirm the committed bundle matches a fresh build of
  `web/src` at bdc15b7.
- **The five bug records BUG-014..BUG-019** claimed as fixed by the phase
  commits were read only as titles; I did not verify each recorded defect is
  actually absent from the final tree.
- The four non-LAN commits that ride along (`4ce9244`, `48cb698`, `dc9f6ae`,
  `2ef1e51`) were reviewed only for coherence with the Phase 2 changes to the
  same package, which is what the dispatch asked; the exec seam's own 720-line
  `guestexec_test.go` was not read line by line.
