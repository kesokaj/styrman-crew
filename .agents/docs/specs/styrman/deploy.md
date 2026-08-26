# Deployment spec (Phase 3)

Two ways to run styrman; both share the same state under `/var/lib/styrman`.

## Dev mode (no auth, localhost/LAN)

```bash
STYRMAN_IMAGE=styrman-runtime:dev go run ./cmd/styrman/          # 127.0.0.1:8080
STYRMAN_ADDR=0.0.0.0:8080 ... go run ./cmd/styrman/              # LAN
```

`STYRMAN_AUTH` defaults to `none`; whoever reaches the port acts as the
default user (first allowed email, else the OS user) with admin rights.
Never expose this beyond loopback/trusted networks. In the compose deploy
the same no-auth handler is published on host loopback only
(`127.0.0.1:8080` → `STYRMAN_DEV_ADDR`) as the test/automation door.

## Production mode (Google OAuth, HTTPS, survives reboots)

Composition (`deploy/docker-compose.yml`):
- **styrman-caddy** — the ingress vendored from devbox (`deploy/caddy/`,
  see VENDORED.md): Caddy 2 + caddy-security (Google OAuth portal at
  `auth.<BASE_DOMAIN>`), Cloudflare DNS records + ACME certs, and Docker-label
  app discovery. Host networking; watches the docker socket read-only.
- **styrman-server** — the Go binary containerized
  (`deploy/Dockerfile.server`), discovered via labels
  (`caddy_hostname=styrman` → `https://styrman.<BASE_DOMAIN>`), running as
  the operator's uid/gid + the host docker group. `STYRMAN_AUTH=header`:
  every request must carry the ingress's `X-Caddy-Email` AND pass the
  userstore (`STYRMAN_ALLOWED_EMAILS` + `/var/lib/styrman/users.json`) — R8's
  two layers. The state dir and docker.sock are mounted at host-identical
  paths so microVM bind sources stay valid.

Both containers are `restart: unless-stopped`: a host reboot brings the whole
stack back via docker.service — no systemd units needed.

### Bring-up

```bash
sudo bash deploy/host-setup.sh --user $USER   # also seeds deploy/.env identity values
vi deploy/.env                                # fill: BASE_DOMAIN, GOOGLE_CLIENT_ID/SECRET,
                                              #       CLOUDFLARE_API_TOKEN, ADMIN_EMAIL,
                                              #       STYRMAN_ALLOWED_EMAILS
docker build -t styrman-runtime:latest image/
docker compose -f deploy/docker-compose.yml up -d --build
# → https://styrman.<BASE_DOMAIN>  (Google sign-in via auth.<BASE_DOMAIN>)
```

Google OAuth client (console.cloud.google.com → APIs & Services →
Credentials): Web application, redirect URI
`https://auth.<BASE_DOMAIN>/oauth2/google/authorization-code-callback`.
Cloudflare token: Zone:DNS:Edit on the zone.

### Verified without credentials

Images build; compose config validates; the containerized server passed the
header-auth matrix (401 no header / 403 unauthorized / 200 authorized /
health open) against the live state dir. The remaining untested step is the
Google OAuth handshake itself, which needs the real client + domain.
