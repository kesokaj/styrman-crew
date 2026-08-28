# Credential-service P1, Increment 2 (slice 1) — the broker + git

Date: 2026-08-28. Second increment of ADR-030. Delivers the credential
BROKER and the first per-tool adapter (git/GitHub), proven end-to-end.

## What shipped

- **Broker store** (`internal/credstore`): per-user, server-owned, 0600,
  atomic writes, under `StateDir/credentials/<user>/`. The single source of
  truth; projects never touch it. `GitCred{host,username,token}` for slice 1.
- **Broker HTTP** (`internal/credbroker`): a credential-ONLY listener
  (`STYRMAN_BROKER_ADDR`, default `:8091`), bound on all interfaces so a
  project reaches it on its OWN bridge, but NOT published to the host — a
  bridge-only surface separate from the ingress-gated main API. `GET /git?
  host=` returns the stored credential. Caller identified by network
  position: `orchestrator.ResolveProjectByIP` maps the source IP to the
  sdb-<uuid> bridge subnet → project → owner (the metadata-server pattern;
  composes with ADR-029). Since all a user's projects share credentials in
  P1, this identity is audit/scoping, not a security boundary.
- **Broker URL hint**: the server writes `~/.config/styrman/cred-broker`
  (= `http://<server bridge IP>:8091`) into the home after it attaches to
  the project bridge (file write, not exec — the home is mounted). Refreshed
  on every terminal attach so it tracks the server's current bridge IP.
- **In-guest git helper** (`image/configs/styrman-git-credential`, wired as
  the SYSTEM `credential.helper` with `useHttpPath=false`): on git `get`,
  curls the broker and returns username/password. No token is stored in the
  project (`store`/`erase` are no-ops).
- **Onboarding** (`POST /api/settings/credentials/git`): extracts the
  CURRENT gh token from a running project's keyring (`gh auth token` via
  exec — gh keeps its token in the guest keyring, not a file, confirmed on
  this box) and writes it to the broker store, once. Thereafter every
  project uses it.

## Proven end-to-end (live prod project)

Seeded the store, then from inside the project: the broker served the
credential over the bridge, `git credential fill` returned it via the
helper, and NO token was persisted in the project (`~/.git-credentials`
absent). This is the operator's "credential service" working: pull the
current credential on demand, secret stays in the broker, refresh
propagates by updating the one store.

## Key finding

gh (like agy) stores its token in the gnome-keyring here, not `hosts.yml`
(no `oauth_token` in the file). So the broker's store is broker-native and
onboarding EXTRACTS from the keyring once — the "read a file" shortcut
doesn't exist. This is the correct target design anyway.

## Not yet (Increment 2 continues, then Inc 3)

- More adapters: ssh agent-socket, docker/gcloud credential helpers, gh/npm
  env injection, claude `apiKeyHelper`, agy keyring-seed. Each attaches to
  the same broker.
- Refresh write-back for file/keyring-only tools; short-lived tokens (P2).
- Inc 3: per-project home volumes + the guarded, backed-up migration of the
  existing shared home (gated on operator go-ahead).

## Review fixes (reviewer subagent, same session)

One blocking finding, fixed: the original broker-URL delivery wrote a
per-project bridge IP into a SINGLE per-user file (the shared home) — so
with ≥2 running projects the last one won and every other project's git
helper hit an unreachable bridge IP (R12 isolates bridges), stalling then
falling through. My single-project test missed it.

Fix (the metadata-server pattern done right): the broker now answers at a
FIXED per-bridge address — the subnet's broadcast-1 (e.g. 172.19.255.254) —
which the server binds as a static IP when it attaches each bridge, and
which is injected into each project as the `STYRMAN_CRED_BROKER` container
env. No per-project hint file at all. This fixes the blocker AND the
stale-hint-after-restart gap AND the shared-file problem in one stroke;
`writeBrokerHint`/`SelfBridgeIP` were removed. **Proven with two projects
running simultaneously**: each got a working broker at its own bridge's
fixed IP (172.19.255.254 / 172.18.255.254) and `git credential fill`
returned the credential in both.

Also fixed: the onboarding `host` is validated (`^[A-Za-z0-9.-]+$`) before
it reaches a login-shell command (was a `bash -lc` injection surface); the
git helper validates `host` before the broker query; `ResolveProjectByIP`
skips `sdb-sys-*` setup networks, guards an empty user label, and caches
IP→(user,project) for 30s to bound Docker API calls under repeated `/git`;
the broker isn't started in host-mode dev (SelfContainer empty). Noted for
follow-up: source-IP identity relies on Docker's return-path isolation as
the anti-spoof defense (projects have NET_ADMIN) — acceptable for P1 since
all a user's projects share credentials; a two-project spoof test is a
nice-to-have.

## Verification

go test -race (incl. new broker/store tests) + e2e; the multi-project broker
proven live on prod (two projects, each git credential fill via its own
bridge). Broker not published to the host (compose has no `ports:` for it).
