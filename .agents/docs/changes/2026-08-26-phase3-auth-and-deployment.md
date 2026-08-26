# 2026-08-26 — Phase 3: auth layer, containerized server, vendored ingress

## Summary

Everything Phase 3 needs except the operator's secrets: `internal/userstore`
(env allowlist + users.json, ported/trimmed from devbox), `authGuard`
middleware (`STYRMAN_AUTH=header` requires an authorized `X-Caddy-Email`;
health stays open; unit-tested matrix), the Caddy ingress vendored into
`deploy/caddy/` (R11 — no reliance on the devbox checkout), a containerized
server image (`deploy/Dockerfile.server`, runs as operator uid + docker
group, state mounted at host-identical paths), `deploy/docker-compose.yml`
(caddy host-network + labeled server, restart unless-stopped ⇒ reboot-safe),
`.env.example`, and host-setup.sh now seeds `deploy/.env` with computed
UID/GID/DOCKER_GID.

## Failures / gotchas

- `user.Current()` panics-by-error in a static binary without $USER — the
  containerized server died at startup. config.Load now treats it as
  best-effort and requires STYRMAN_USER instead (clear error).
- Docker group access from a non-root container: `group_add: <docker gid>`
  in compose; host-setup computes the gid into .env.

## Verified

- Unit: authGuard matrix (401/403/200/case-insensitive/health-open),
  userstore round-trip via existing store patterns.
- Live: styrman-server image ran with the real state dir + socket as
  uid 1000 + docker group → 401 no header, 403 wrong email, 200 + real
  project list with the right email, health open.
- styrman-caddy image builds (xcaddy + vendored ingress module).
- `docker compose config` validates with a filled sample .env.
- Dev-mode default (STYRMAN_AUTH=none) unchanged: integration 23/23 + e2e
  18/18 still green from the same code.

## Remaining for Phase 3 completion (operator input)

Fill deploy/.env: BASE_DOMAIN, GOOGLE_CLIENT_ID/SECRET (redirect URI
`https://auth.<domain>/oauth2/google/authorization-code-callback`),
CLOUDFLARE_API_TOKEN, ADMIN_EMAIL, STYRMAN_ALLOWED_EMAILS — then
`docker compose -f deploy/docker-compose.yml up -d --build` and test the
OAuth handshake end-to-end.
