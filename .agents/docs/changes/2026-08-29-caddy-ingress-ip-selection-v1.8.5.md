# Caddy proxied crew to a dead project-bridge IP (v1.8.5)

Date: 2026-08-29. Operator report: `crew.styrman.xyz` sent them to a blank page
and downloaded a file named `document.txt` (persisted in incognito — a real
server-side bug, not a cache artifact).

## Symptom → cause

The browser was saving Caddy's **502 error body** as `document.txt` (the error
response had no HTML content-type / filename, so the browser downloaded it).

Caddy was proxying `crew.styrman.xyz` to `172.19.255.254:8080` — a **project
bridge** broker IP (subnet-broadcast-1 on `sdb-45b71295…`), which the ingress
can't reach → `dial tcp … i/o timeout` → 502. The correct upstream is the
server's **ingress** IP `172.18.0.2` (on `styrman-ingress`).

Root cause in `deploy/caddy/ingress/watcher.go` → `extractContainerIP`: it
iterated the container's networks map and returned the **first** IP. Go map
iteration is random-order, and the styrman server is attached to **three**
networks (ingress + two project bridges, for the broker and the preview proxy).
It happened to return the ingress IP until the **v1.8.3 network recreation**
(firewall fix) reattached the server to freshly-created project bridges and the
next re-discovery picked a project-bridge IP instead. Latent since the server
first became multi-homed; the recreation just reshuffled the map.

## Fix

`extractContainerIP` now selects deterministically:
1. Prefer the `styrman-ingress` network if present (the server's ingress IP).
2. Else prefer any non-project network (project bridges are `sdb-*`).
3. Else fall back to whatever's there — a **preview VM** has only its own
   `sdb-*` network, and that IP is the correct target for it.

Regression test `watcher_ip_test.go` (`TestExtractContainerIP`) locks it in:
a server on ingress + two `sdb-*` bridges resolves to the ingress IP on every
one of 100 iterations; a preview VM on a single `sdb-*` bridge resolves to its
own IP; host-network → `127.0.0.1`; no networks → empty.

## Verification (live)

- Rebuilt `styrman-caddy:latest` (xcaddy; `caddy list-modules` still lists the
  `ingress` plugin), `docker compose up -d caddy`.
- After re-discovery, Caddy's generated config proxies crew to `172.18.0.2:8080`
  (was `172.19.255.254:8080`).
- `curl -L https://crew.styrman.xyz/` → **HTTP 200, `text/html`** (was a 502
  saved as `document.txt`).

## Release

Caddy-image-only change (no styrman-server rebuild). Tagged v1.8.5.
