---
owner: bugs
date: 2026-09-09
id: BUG-025
status: fixed
area: .agents/docs/testing/styrman/integration.sh (ADR-067 network-mode checks)
---
# BUG-025: fixture re-implemented BridgeNameFor's hash wrong, and asserted an ownership claim it couldn't make

## Symptom
Two new network-mode checks failed against a correctly working feature:
"the suffix is unchanged (same project, same identity in ip link)" and "no
firewall rule names the lan- bridge".

## Repro
`bash .agents/docs/testing/styrman/integration.sh` with the ADR-067
network-mode section added; went 78/5/3 before the fix.

## Root cause
Two distinct fixture errors. (1) The suffix check recomputed the expected
bridge name as `printf 'sdb-%s' ${NET} | sha1sum | cut -c1-11`, but
`config.BridgeNameFor` (`internal/config/config.go:480`) hashes the
NETWORK NAME ALONE and prepends the mode prefix afterward. Measured on the
host: network `sdb-e441b4b6-3a6b-426c-bd89-e0da5f65ab20` has bridge
`sdb-48cabdb8e84`; hashing the bare name gives `48cabdb8e84`, hashing
`sdb-`+name gives `aac7fb87f27`. The fixture re-implemented the algorithm
and got it wrong — the same class the file's `MINVER` comment already
warns about ("a fixture that encodes a constant must READ the constant").
(2) The second check asserted no iptables rule anywhere names the lan-
bridge. FALSE BY CONSTRUCTION: Docker names every bridge in full in four
of its own rules (DOCKER, DOCKER-BRIDGE, DOCKER-CT, DOCKER-FORWARD), and
one of them — `-A DOCKER ! -i <br> -o <br> -j DROP` — IS the cross-project
isolation ADR-067 relies on. Styrman's own rules key on the `sdb-+`
WILDCARD and never name a bridge in full. The check conflated "no styrman
rule matches it" with "no rule mentions it".

## Fix
(1) Hash the bare network name, matching `BridgeNameFor`. (2) Replaced
with the narrow true claim —
`! iptables -S | grep STYRMAN | grep -q -- '-i lan-'` — plus a new
positive check that Docker's per-bridge isolation DROP for the lan- bridge
is present, both behind the existing `sudo -n` availability gate with
skips. Integration went 78/5/3 to 84/0/3.

## Prevention
A fixture must READ the constant or algorithm it checks, never
re-implement it — extends the existing MINVER lesson from literals to
derived values. Before asserting that no rule/entry mentions something,
establish WHO ELSE writes to that shared namespace; an assertion over an
externally-owned list has to name the owner it means.

## Related
BUG-021 (same ADR-067 Phase 5 suite work). Third occurrence of "a fixture
that duplicates production logic instead of reading it" alongside the
existing MINVER case — candidate for a permanent rule in
`.agents/rules/` on fixtures never re-deriving constants/algorithms.
