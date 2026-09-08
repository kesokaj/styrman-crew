---
owner: main
date: 2026-09-08
---

# LAN parity Phase 2: the host itself becomes reachable, behind a conf opt-in   (2026-09-08)

Plan: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (Phase 2, 6 of 6
boxes). ADR-066 parts (1), (2) and the first half of (3).

## What it does

A second rule family, `lanhost`, in chain `STYRMAN-IN`:

```
-A STYRMAN-IN -s <bridge subnet> -d <host addr>/32 -i sdb-<hash> \
   -m comment --comment "styrman-lanhost:<uuid>" -j RETURN
```

Inserted at the head, ahead of the isolation DROP. It carries no `-o`,
because a packet addressed to the host is delivered locally and never
forwarded, so there is no output interface to pin; `-s` is still pinned to
the bridge subnet so a guest with `NET_ADMIN` cannot spoof past the DROP
under loose `rp_filter`.

Gated on `LAN_ALLOW_HOST` in the root-owned conf. **Absent or `0` keeps the
pre-ADR-066 behaviour exactly**, so converging an existing host to script
version 4 grants nothing; only an explicit `1` opts in. The family is
registered in the table unconditionally, which is what makes turning the key
back off *remove* the rules rather than orphan them.

Host addresses are discovered from the uplink's global addresses and
restricted to the configured prefixes — an address outside `LAN_CIDRS` is
skipped with a note, because the allowance is defined by `LAN_CIDRS` and a
second global address on another subnet is not the local network the operator
named.

`STYRMAN_FW_VERSION` 3 → 4 with `netpolicy.MinScriptVersion`, so a host still
on 3 refuses to enable rather than silently under-applying what the server
promises.

## install.sh, and a correction to the plan

The plan said `install.sh` writes `LAN_ALLOW_HOST=1`. It writes **0**.
Writing 1 on a fresh install would grant host reach to every new deployment
by default, contradicting ADR-066 part (2), which the plan itself quotes.
`--lan-allow-host 0|1` is the opt-in, and it is the one thing this installer
will change in an existing conf — the conf is otherwise never rewritten once
present, so the operator would have had to hand-edit it. Without the flag on
an existing conf, install.sh prints a note saying the host is not reachable
and how to opt in.

## Security findings from the Phase 1 review, fixed here

`reviews/2026-09-08-security-phase1-refactor.md`, no highs; all four
preservation properties passed. Three findings, all fixed in this commit:

1. **medium/high — `for chain in $(chains)` swallowed the helper's failure.**
   Reproduced in isolation: under `set -e` with `inherit_errexit`, a command
   substitution whose command fails yields an empty word list and rc 0, and
   execution continues. The scan is the sole producer of `PURGE` and
   `MALFORMED` while reconcile's insert pass iterates `DESIRED`
   independently — so that failure would revoke nothing and still install
   everything. Loosening without tightening, on the one construct gating all
   revocation. Replaced with an array built once from the family table, an
   empty table now being a hard error rather than an empty loop.
2. **medium/medium — the numbered delete could in principle remove a DROP.**
   Not reachable today (no base rule carries a comment, and the delete is
   filtered on our comment prefix), but it is the single delete that names a
   rule by POSITION, and it sits beside the isolation itself. It now refuses
   any line ending `-j DROP` and records a warn. Section 20 of `test.sh`
   plants exactly such a rule and asserts the refusal.
3. **low/high — `rule_args` built a function name from key data** with only
   `set -u` on the following array lookup standing between an unknown tag and
   an abort with no status written. The tag is now checked against
   `FAMILY_TAGS` explicitly, `ARGS`/`CHAIN` are reset, and callers skip the
   key instead of proceeding.

The reviewer also suggested re-running the base pass at the end of the LAN
section so a concurrently-removed base rule is repaired at once rather than
within 60 s. Not done here: it is a real improvement but independent of this
phase, and it wants the base block extracted into a function. Carried to
`TODO.md`.

## Verification

- `deploy/firewall/test.sh` **102 / 0**, up from 83 at the Phase 1 baseline:
  +19 checks across three new sections (host rule head placement and
  idempotence, the DROP-delete refusal, and off/invalid/marker-removed fail
  closed).
- `go vet ./...`, `go test ./...`, and the vendored ingress module: clean.

## Test fixtures now track the constant

Bumping `MinScriptVersion` failed four `httpapi` tests and two `netpolicy`
tests that wrote `version=3` into fake status files. That is the gate working,
not a regression — but the fixtures are now built from
`netpolicy.MinScriptVersion` rather than a literal, and the "outdated" cases
from `MinScriptVersion - 1`, so the next bump is one edit instead of a hunt.
`TestParseStatus` already asserted an unknown `future_key=` is ignored, which
independently confirms the forward-compatibility this plan relied on.

## Two self-inflicted detours, recorded rather than hidden

Both cost a cycle and neither reached a commit. A python assertion checking
that a rejected shell construct was gone tripped on the explanatory comment
that quotes it. And the first run of the new tests failed six checks, all
from two bugs in the test itself: a counter that matched the deliberately
malformed rule it was supposed to ignore, and a cleanup deleting rule number
1 after the reconcile had refilled that position with the real rule. The
script was correct in both cases.
