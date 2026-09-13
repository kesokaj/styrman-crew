---
owner: bugs
date: 2026-09-13
id: BUG-054
status: fixed
area: image/configs (login-shell credential snippet) / internal/orchestrator
---
# BUG-054: login-shell credentials silently unset since v1.27.1 (/24 bridges, /16 recomputation)

## Symptom
Since 2026-09-10 (v1.27.1), every project's login shell has GH_TOKEN/NPM/
CLAUDE env unset, no gcloud ADC, no ssh key; HTTPS git auth falls through.
Silent — the fetch is `curl ... || true`.

## Repro
On the live host: project network `sdb-32c61123` is `172.16.1.0/24`,
container env `STYRMAN_CRED_BROKER=172.16.1.254`, but
`image/configs/styrman-credentials.sh:25-31` recomputes it from the first
two octets assuming a /16 and produces `172.16.255.254`, where nothing
listens. Measured live 2026-09-13.

## Root cause
`STYRMAN_CRED_BROKER` is computed correctly server-side
(`internal/orchestrator/orchestrator.go:475 brokerIPForSubnet`, real mask)
but the guest snippet independently re-derives the same value assuming a
hard-coded /16 (added 2026-08-29, a74cc1b, for a recreate-changes-subnet
worry that was itself moot — Docker won't remove a network with an attached
container, and every recreate path re-injects a fresh env anyway). On
2026-09-10 `deploy/host-setup.sh` pinned Docker's default-address-pools to
`/24` bridges, breaking the guest's /16 assumption. Two independent
derivations of one value, no test tying them together.
The 2026-09-10 broker reachability test curl'd the injected env var
directly, never a real login shell, so the divergence was invisible.

## Fix
Delete the recomputation in `image/configs/styrman-credentials.sh:25-31`;
trust the injected `STYRMAN_CRED_BROKER` as-is. Go test pins that the
snippet contains no octet arithmetic and reads the env var directly. MOTD
reports when the broker did not answer.

## Prevention
A network parameter the server computes must be injected into the guest,
never re-derived there. Any change to the Docker address-pool shape must
grep every consumer of that shape (mechanism-reaches-every-caller family,
BUG-035). A "reachability" test must exercise the path users actually take
(a login shell), not the raw variable in isolation (BUG-040 family).

## Related
BUG-035, BUG-040. `.agents/docs/reviews/2026-09-13-audit-project.md`,
`.agents/docs/reviews/2026-09-13-audit-security-credentials-broker.md`,
`.agents/docs/testing/2026-09-10-broker-reachability-live.md`.
