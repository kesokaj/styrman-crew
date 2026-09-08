---
owner: reviewer
date: 2026-09-08
---
# Review — ADR-065 Phase 1: host firewall half (LAN access toggle)   (2026-09-08)

Range reviewed: `/tmp/claude-1000/-home-kesokaj-workspace-styrman-crew/721f8136-9d9a-45fb-b173-cb58bede525c/scratchpad/phase1.diff`
(`deploy/firewall/{styrman-firewall.sh,.service,.timer,.path,install.sh,test.sh}`,
`deploy/host-setup.sh`, `deploy/update/styrman-update{,.service}`,
`deploy/verify-commit.sh`, `internal/netpolicy/{netpolicy.go,script_test.go}`)

## Verdict: REQUEST CHANGES

The firewall script itself is solid — the base section really is byte-identical to
the removed heredoc, all 79 `deploy/firewall/test.sh` checks pass under
`unshare -Urnm`, `go test ./internal/netpolicy/` passes, `bash -n`, `shellcheck -S
warning`, and `systemd-analyze verify` are all clean. The one real bug is in the
updater, not the script: `styrman-update --check` — documented as
`"report what would happen, change nothing"` — now runs `host_converge()` before
the `CHECK_ONLY` branch, so a `--check` invocation can install a new root-owned
`styrman-firewall.sh`/units and restart `styrman-firewall.{service,timer,path}`
whenever the deployed tree's firewall files differ from what's on disk. This repo
has hit exactly this bug shape before (line 142's comment: an earlier `--check`
chmod'd `/tmp` itself — "review+security finding, measured") and it recurs here
in a different form.

## Findings

- **[high / confidence: high]** `deploy/update/styrman-update:618-627` — `host_converge`
  is called unconditionally before the `CHECK_ONLY` exit, but `host_converge` can
  write files (`atomic_install`) and run `systemctl daemon-reload` /
  `enable --now` / `start` as root when the deployed tree's `deploy/firewall/*`
  differs from what's installed. The script's own usage text
  (`deploy/update/styrman-update:25`) promises `--check` "change[s] nothing". Under
  normal steady state (nothing drifted) this is a no-op, but the exact scenario
  ADR-065 exists to fix — installed firewall files behind the verified commit — is
  precisely when `--check` would silently apply the fix instead of just reporting
  it. Concrete scenario: an operator runs `sudo styrman-update --check` to preview
  before deciding to deploy, on a host where `/usr/local/sbin/styrman-firewall.sh`
  predates the currently-deployed tag (e.g. `install.sh` hasn't been re-run since a
  hand edit, or a previous `host_converge` failed partway); `--check` now silently
  reinstalls the script and bounces `styrman-firewall.service`/`.timer`/`.path`
  before printing "update AVAILABLE". Fix: move the `host_converge` call to after
  the `CHECK_ONLY` exit (still ahead of the "already up to date" exit, which has no
  such promise), or gate it explicitly on `[ "${CHECK_ONLY}" = false ]`.

- **[medium / confidence: medium]** `deploy/update/styrman-update:349-353` vs
  `:618` — the pre-existing "previously FAILED, not retried" exit
  (`if [ "$(cat "${FAILED}")" = "${LATEST}" ] ...; then ... exit 0; fi`) sits
  *before* the function definitions and therefore before the new `host_converge`
  call. ADR-065's stated design is host-converge "on EVERY tick" (the script's own
  comment at `:469-471` says the same). Once a release is marked FAILED, every
  subsequent automatic tick exits at this check without ever reaching
  `host_converge`, even though the firewall half of that same tag (or of the
  rolled-back-to tag) may be unrelated to the failure. Concrete scenario: tag
  `vX.Y.Z` ships a firewall fix and an unrelated server bug that fails the health
  check; the updater rolls back and marks `vX.Y.Z` FAILED. If, after that rollback,
  something reverts or removes `/usr/local/sbin/styrman-firewall.sh` (or a
  `.service`/`.timer`/`.path` unit) on the host, no further automatic tick will
  restore it until an operator pushes a newer tag or passes `--force` — the
  "closes the drift class" claim in the ADR does not hold across a FAILED tag.
  Narrow window, but it directly contradicts the delivery goal the diff exists to
  satisfy. Fix: either move the FAILED-tag exit below `host_converge` too, or call
  `host_converge` once before that check.

- **[low / confidence: medium]** `deploy/firewall/test.sh:127-135` — test 9 ("spool
  drained on a FAILED run (no re-trigger loop)") writes its spool entry
  (`${RUN}/spool/${U}.456`) *before* the failing run starts, so it is caught by the
  script's unconditional drain-first step, not by the `on_exit` EXIT-trap drain the
  test's name and the script's comment (`styrman-firewall.sh:275-282`) claim to be
  proving. The distinct code path — an entry arriving *after* drain-first but before
  a mid-run failure — is not exercised by any test; the assertion currently passes
  regardless of whether the EXIT trap works at all. Not a live bug (drain-first
  already gives the safety property this test checks), but the test doesn't prove
  what its name claims.

- **[low / confidence: low]** `deploy/verify-commit.sh:117-119` — the new
  `shellcheck -S warning` gate covers only `deploy/firewall/*.sh`; `deploy/update/styrman-update`,
  which gained the new ~90-line `host_converge` function in this same diff, is only
  `bash -n`'d, not shellchecked, despite the gate's own stated purpose ("the
  firewall script runs as root from the verified tag via the updater's
  host_converge, so a syntax error... must fail here"). `shellcheck -S warning
  deploy/update/styrman-update` is currently clean, so this is a coverage gap, not
  an active bug.

## Criteria

1. **Dead code** — pass. The old heredoc in `host-setup.sh` §3b is fully removed
   (comment-only remnant), nothing left calling into it; no orphaned functions in
   the new files (every helper in `styrman-firewall.sh` and `install.sh` is called).
2. **Flow tracing** — mostly pass; one real break (finding 1: `--check`'s contract)
   and one gap (finding 2: FAILED-tag skips `host_converge`). Server-side markers
   are out of scope for Phase 1 (Phase 2), so `internal/netpolicy` here is
   intentionally just the version constant + the script wrapper test — consistent
   with the plan.
3. **Standards conformance** — good use of process substitution (`< <(...)`)
   everywhere state needs to survive a loop under `set -eu`, so no read-in-pipeline
   subshell bugs; deletions are by rule number, highest-first, matching the
   documented invariant; `local -a`/associative arrays used correctly; unquoted
   pattern variables in `[[ =~ ]]` (correct bash idiom). `systemd-analyze verify`
   and `shellcheck` both clean for the files they cover (finding 4 notes a gap in
   what's covered, not a violation of what is).
4. **Directory layout** — pass. `deploy/firewall/` mirrors `deploy/update/`;
   `internal/netpolicy/` is a new `internal/` package with its test alongside it.
5. **Simplicity** — pass. Pure bash + Go stdlib (`os`, `os/exec`, `regexp`,
   `runtime`, `strconv`), no new dependencies; no auth/crypto surface touched.
6. **Decoupling** — pass with a minor note: `self_install` and `host_converge`
   share the global `SELF_HEAD`/`atomic_install`/`differs` helpers, each
   overwriting `SELF_HEAD` immediately before use; safe today (never called
   concurrently) but a future third self-installing target would need the same
   discipline — worth a comment if the pattern is copied again, not a finding on
   its own.
