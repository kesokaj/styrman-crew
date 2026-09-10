---
owner: main
date: 2026-09-10
---
# Live test — credential-broker reachability from a LAN project   (2026-09-10)

The test `reviews/2026-09-09-security-adr-067.md` § "The one test that
settles it" specified, run on the **compose deployment** (production,
`styrman-server:1.27.0`, `STYRMAN_SELF_CONTAINER=styrman-server`) by main
with the operator's authority. Owed since 2026-09-09; closed here.

## Setup

- Two DIFFERENT users: A = the operator (`ostling-simon`, project
  `32c61123…`, the one from the LAN walk-through); B = a throwaway
  allowlist entry `broker-test-b@example.com` added through
  `POST /api/settings/users` and given one project (`9ec1de47…`) through
  the server's own API, reached directly on the ingress bridge with the
  ingress token from `deploy/.env` and `X-Caddy-Email`, exactly as the
  Caddy ingress does. Both running; the server attached to both bridges.
- A switched to LAN through `PUT /api/projects/{id}/network` (rebuilt:
  true → bridge `lan-e64fe297000`, subnet 172.16.1.0/24); B sandboxed
  (bridge `sdb-658b9af49ad`, subnet **172.16.0.0/24 — the first network
  drawn from the `default-address-pools` host-setup pinned minutes
  earlier**; existing networks kept their 172.19/16 subnets).
- Addresses: A's broker (the server's endpoint on A's network)
  172.16.1.254; B's broker 172.16.0.254; B's gateway on the host
  172.16.0.1; host uplink 192.168.5.10.

## Probes from A's guest (`docker exec … curl -s --max-time 5`)

| # | probe | expected | observed |
|---|---|---|---|
| 1 | `http://172.16.1.254:8091/env` | 200, A's own user's map | **200**, a map with A's own git identity and `GH_TOKEN` (values not reproduced here) |
| 2 | `http://172.16.0.254:8091/healthz` | timeout (Docker's per-bridge DROP) | **timeout** after 5000 ms |
| 3 | `http://172.16.0.1:8091/healthz` | connection refused | **connection refused** (0 ms) |
| 4 | `http://192.168.5.10:8091/healthz` | connection refused | **connection refused** (0 ms) |
| 4b | same URL from the HOST (substitute for "a non-styrman machine on 192.168.5.0/24" — no second machine was at hand) | connection refused | **connection refused** |
| ctl | `http://192.168.5.10:22` from A | reaches the LAN (LAN mode is real) | sshd answered (HTTP/0.9 error = a banner came back) |

Every row matches the expected column. No 200 anywhere but the baseline.
The broker is reachable only at the caller's own bridge endpoint; the
per-bridge DROP keeps A off B's endpoint even with A on the LAN; nothing
listens on the host addresses.

## Teardown

A back to sandbox (`rebuilt: true`, back on `sdb-e64fe297000`, now
172.16.1.0/24 from the pool); B's project deleted (container and network
gone, verified with `docker ps -a` / `docker network ls`); B removed from
the allowlist (200). The server keeps a removed user's metadata directory
(`users/broker-test-b/`, an empty project store) and any home it created —
by design (removal is an allowlist change, not a wipe); it holds nothing
of value and no agent touches `/var/lib/styrman` (ADR-052). The operator
may `rm -r` those two directories at leisure.

## Not examined

- The probe from a real second machine on 192.168.5.0/24 (4b is the
  host-side substitute). The operator can run
  `curl -s --max-time 5 http://192.168.5.10:8091/healthz` from a laptop;
  "connection refused" is the expected answer.
- Source-address forgery (the review itself says no forgery step belongs
  in this test).
