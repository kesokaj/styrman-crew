---
owner: reviewer
date: 2026-09-08
---
# Review — cold-reader check: LAN full parity plan   (2026-09-08)

Range reviewed: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (plan document, not a diff), cross-checked against `deploy/firewall/styrman-firewall.sh`, `internal/netpolicy/status.go`, `internal/netpolicy/dir.go`, `internal/httpapi/network.go`, `internal/httpapi/server.go`, `internal/credbroker/broker.go`, `internal/credstore/credstore.go`, `cmd/styrman/main.go`, `deploy/firewall/install.sh`, `deploy/docker-compose.yml`.

## Verdict: DESIGN NEEDS REVISION / IMPLEMENTATION NOT READY

The plan is unusually well grounded — most of its factual claims about the current code (RULE_RE at line 199, the `status.go:60` no-default-case claim, `kickLAN`'s shape at `network.go:182`, `main.go:104`/`180` ordering, `credstore` file-path format) check out exactly against the live source. The one place it is not concrete enough to implement is the section it was flagged for: the generic key grammar proposed for the "Rule-family machinery" generalization cannot represent the existing `lan` family it is supposed to absorb first, which stops Phase 1's very first task cold. There is also a real, unaddressed security-relevant gap in how `INGRESS_BRIDGE` gets its value and how its identity is verified, which matters because the whole point of excluding it is to keep the ingress bridge out of the LAN-reachable set — the plan checks it is *a* bridge, never that it is *the* ingress bridge.

## Findings

- **[high / confidence: high]** `.agents/docs/plans/2026-09-08-lan-full-parity.md:150` vs `deploy/firewall/styrman-firewall.sh:199,315` — the proposed generic key `"<tag> <iface> <f1> <f2> <uuid>"` has room for exactly two data fields besides `iface`. The existing `lan` family's key (built at `styrman-firewall.sh:315`: `"${iface} ${LAN_DEV} ${subnet} ${c} ${name}"`) and its `RULE_RE` (line 199, five captures: src, dst, iface, dev, uuid) need **three** data fields (`dev`, `src`, `dst`). Phase 1's first bullet ("port the existing `lan` family onto it as the only family") cannot be completed as specified: either the template needs a third field (and the table at lines 144–148 needs to say so explicitly for `lan` vs. `lanhost`/`lanpub`), or the `dev`/`-o` field is dropped from the key for `lan` and the implementer has to invent how. Given Risk 4's own standard — this is security-critical delete-by-spec code, and a mis-specified key is exactly BUG-013's failure mode (a rule the code thinks it deleted/matched is not actually the rule it built) — this is not something a first-pass implementer should resolve by guessing.

- **[medium / confidence: medium]** `.agents/docs/plans/2026-09-08-lan-full-parity.md:127-131,186-188` vs `deploy/firewall/install.sh:130-141` and `deploy/docker-compose.yml:153` — `INGRESS_BRIDGE` validation is "`br-[0-9a-f]{12}`, must exist, must not be `sdb-*`" (plan line 187). Nothing ties the value to the actual `styrman-ingress` compose network; a stale or mistyped-but-syntactically-valid value leaves the *real* ingress bridge inside "every Docker bridge that is neither `sdb-*` nor `INGRESS_BRIDGE`" (plan line 127-129) — i.e. eligible for the `lanpub` allow rule, the opposite of the exclusion's purpose. `install.sh`'s existing `LAN_DEV`/`LAN_CIDRS` auto-discovery (`install.sh:130-141`) `die`s when it can't determine a safe value; the plan's Scope line 82 says `install.sh` will write `INGRESS_BRIDGE=` (blank) by default, with no stated fallback (auto-discover from the `styrman-ingress` docker network, or refuse to enable `LAN_ALLOW_PUBLISHED_PORTS=1` without an explicit value). This needs a decision before Phase 3/`install.sh` work starts, not during it.

- **[low / confidence: medium]** `.agents/docs/plans/2026-09-08-lan-full-parity.md:333-353` — `scan_current`'s per-line "is this rule ours" pretest (`styrman-firewall.sh:339`, a single-tag substring test) and `count_applied` (line 401-417, listed at line 135 as needing generalization but never walked through in the machinery description at lines 150-166) are both left to inference for the multi-tag-per-chain case (`STYRMAN-FWD` will carry both `lan` and `lanpub`). Plausible to infer correctly, but the plan's own bar for this section ("one function, a parameter, not a twin") is not met for these two pieces the way it is for `rule_args`/`delete_malformed`.

## Answers

**(1) Gaps** — see Findings above. Summarized: (a) the generic key template is under-specified for the family it must absorb first; (b) `INGRESS_BRIDGE` provenance/verification is undecided; (c) the multi-tag scan/malformed and `count_applied` generalization is asserted but not spelled out. No misunderstanding of the existing code was found — every specific claim the plan makes about current line numbers, function shapes, and behavior (`RULE_RE`, `status.go` unknown-key handling, `kickLAN`, `main.go` build order, `credstore.FileCred.Path` format) was verified correct against the source.

**(2) Readiness** — a first-pass implementer could not ship Phase 1 without asking:
1. What is the key grammar for a family (like `lan`) whose ARGS need three data fields beyond `iface`, not two — extend the template, or drop a field?
2. How does `install.sh` obtain and verify `INGRESS_BRIDGE` — required flag that `die`s like `LAN_DEV` on failure, or auto-derived from the `styrman-ingress` network, and either way, is its identity re-checked against the actual ingress network on every firewall run or only at install time?
3. When `STYRMAN-FWD` carries both `lan` and `lanpub` tags, what is the exact algorithm for classifying a line as "family X, well-formed" vs. "carries one of our tags but malformed" vs. "not ours, ignore"?
4. Does `count_applied` become a second per-chain post-reconcile scan parallel to `scan_current`, confirmed still separate (not merged, since one runs before reconcile and one after)?

Zero gaps is not met (three items above) and zero questions is not met (four above).

design needs revision
implementation not ready

## Criteria
1. Dead code — n/a (plan document, not a diff).
2. Flow tracing — the Go-side flow (`LanExposed` → `credbroker.Broker.files` → `isHostReachingFile`) traces cleanly end to end against the real `space(user)`, `netpol.Status`, and `credstore.FileCred.Path` shapes; verified by reading the actual code, not just the plan's claims.
3. Standards conformance — n/a (no diff to check against language skills); the plan's own internal standard (every regex/claim "VERIFIED... on this host") is not met for the `lanpub` rendering order or the multi-tag scan algorithm (see findings).
4. Directory layout — n/a.
5. Simplicity — the "one generic implementation parameterized by family" decision is the right call (three near-copies would be worse); the finding is that the parameterization as written doesn't fit its own first data point.
6. Decoupling — n/a for a single-file shell script; the Go-side seam (`LanChecker` interface, implemented by `*httpapi.Server`, consumed by `credbroker`) is small and consumer-defined, appropriately.
