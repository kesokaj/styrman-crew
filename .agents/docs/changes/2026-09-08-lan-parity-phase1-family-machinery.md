---
owner: main
date: 2026-09-08
---

# LAN parity Phase 1: the firewall's reconcile machinery becomes generic over rule families   (2026-09-08)

Plan: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (Phase 1, 4 of 4
boxes). ADR-066. No behaviour changes — that is the point of the phase.

## Why this came first

ADR-066 adds two more rule families (`lanhost` in `STYRMAN-IN`, `lanpub` in
`STYRMAN-FWD`) beside the existing `lan`. Every piece of the reconcile —
`RULE_RE`, `collect_desired`, `rule_args`, `scan_current`,
`delete_malformed`, `reconcile`, `count_applied` — was hardwired to one chain
and one comment tag, so adding them meant either generalizing once or
carrying three near-copies of ~120 lines. Three copies is how a
delete-by-spec or ordering fix lands in one place and is missed in the other
two; BUG-013 was exactly that class of defect. Generalize first, add
families second.

## What changed

A family is now `(tag, chain, regex, key-builder, args-builder)` in a table
at the top of the LAN section. `FAMILY_TAGS` currently holds one entry,
`lan`, so the table describes today's behaviour exactly.

- Keys gained a leading tag; the uuid stays last. The generic code addresses
  **only** those two positions (`${key%% *}`, `${key##* }`) and never parses
  the middle, so families may differ in arity — `lan` carries three data
  fields where the coming `lanhost` carries two.
- `rule_args` dispatches on the tag and sets `CHAIN` alongside `ARGS`; every
  `iptables` call in `reconcile` and `delete_malformed` now uses `${CHAIN}`
  instead of a literal.
- `scan_current` walks once per DISTINCT chain rather than once per family,
  because two families sharing a chain must agree on the first-DROP position.
- `MALFORMED` entries carry their chain, so the re-read-and-confirm-by-number
  step targets the right list.
- `WANT_COUNT`/`APPLIED_COUNT` accumulate across families, keeping `lan=`
  all-or-nothing: a project is reported ON only when every rule it wants is
  present.

Delete-by-spec is untouched: a purge still rebuilds the argv from the
regex captures and deletes by full content match, never by a position from an
earlier read.

## Verification

- `deploy/firewall/test.sh` **unmodified** — that was the phase's pass
  condition — passing **83 / 0**, byte-identical to the baseline captured
  before the first edit.
- `bash -n` and `shellcheck -S warning` clean; `go vet ./...` clean;
  `go test ./...` green, including `TestFirewallScriptInNetns` and
  `TestMinScriptVersionMatchesScript`.
- Exercised against the LIVE host chains with a scratch state dir: conf
  parsed, `lan_dev=enp114s0 lan_cidrs=192.168.86.0/24`, empty marker dir
  yielding no allowances, `STYRMAN-FWD` and `STYRMAN-IN` byte-identical
  before and after, production's own status file untouched. The first attempt
  was refused by the run-dir guard because the scratch dir was not root-owned
  — the fail-closed check working, and worth recording as such.
- `STYRMAN_FW_VERSION` stays 3. The bump belongs to Phase 2, which is why
  `test.sh`'s `version=3` assertion still holds against an unmodified file.

## One deliberate text change

The insert-failure `ERRORS` entry is now `<uuid>:<tag> insert failed`. It was
`<uuid>:<dst uuid>:insert failed`, built by stripping a fixed number of
fields off the key — an assumption the variable-arity keys break. No test
asserts this path, and the family tag is the more useful thing to know now
that there are three.

## Cold-reader round 2, folded in

Round 2 ran against the plan while this phase was being implemented and
returned three findings, all addressed in the plan rather than deferred:

- **high** — the plan's key notation had room for two data fields where `lan`
  needs three. The implementation already used variable arity; the plan text
  was wrong, not the code, and now says so explicitly.
- **medium** — `INGRESS_BRIDGE` was validated only by shape and existence,
  never tied to the real `styrman-ingress` network, so a wrong-but-valid
  value would silently leave the ingress bridge reachable. Now: `install.sh`
  derives it (`br-` + the first 12 hex of the network id — verified live) and
  `die`s on failure as it already does for `LAN_DEV`; a stale value names a
  non-existent interface and fails closed by itself; and the server, which
  unlike root knows its own ingress network, compares root's reported
  `ingress_bridge=` and surfaces a mismatch.
- **low** — the shared-chain scan is now walked through step by step in the
  plan, including why the families' regexes are disjoint by construction.

Round 2's verdict was `design needs revision`; the round-1 and round-2
reports are in `reviews/`. Phase 1 proceeded on the operator's explicit
instruction ("approve the R12 text and start phase 1") with the findings
folded in as they arrived.
