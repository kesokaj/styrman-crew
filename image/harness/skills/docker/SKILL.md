---
name: docker
description: |
  Container-based development with Docker via Colima. Use when running containerized commands,
  building images, managing docker-compose services, writing Dockerfiles, debugging container issues,
  or working with volumes and networking. Activate for any docker, docker-compose, container,
  or Dockerfile mention — even simple "build the container" requests.
  Do NOT use for Kubernetes orchestration (use gke-basics) or Terraform infrastructure (use terraform).
allowed-tools: Bash
---

# Docker & Container Development

Local container work for a Go service (`cmd/<app>/` + `internal/`, see the `golang`
skill). Docker runs via **Colima** on macOS (Apple Silicon); Linux boxes use the plain
Docker daemon and skip every Colima step.

**When NOT to use this skill:** pushing or deploying (`cloud-run-deploy`), Kubernetes
(`gke-basics`), infrastructure-as-code (`terraform`).

## Environment

```bash
# macOS only: Colima must be up before any docker command
colima status || colima start
# both platforms
docker info && docker compose version
```

## Core Rules

### 1. Colima first (macOS)
`colima status || colima start` before every Docker operation on the Mac. On Linux,
`docker info` is the check.

### 2. One architecture rule
Native build for the local smoke test; `linux/amd64` for anything that goes to Google.
An arm64 image on Cloud Run fails with `exec format error`.
```bash
docker build -t <app>:dev .                                        # local smoke test only
docker buildx build --platform linux/amd64 --load -t <app>:<sha> .  # everything bound for GCP
```
Tagging, Artifact Registry, and the deploy itself live in `cloud-run-deploy`.

### 3. No TTY in agent contexts
`docker exec <container> <cmd>` — never `-it`, it fails with "not a TTY".

### 4. Smoke test before anything leaves the machine
```bash
docker run --rm -p 8080:8080 <app>:dev &
curl -fsS http://localhost:8080/healthz
```

## Dockerfile: Go multi-stage

```dockerfile
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app ./cmd/<app>

FROM gcr.io/distroless/static-debian12
COPY --from=build /app /app
USER nonroot
EXPOSE 8080
CMD ["/app"]
```

**Key rules:**
1. Pin the base images (`golang:1.24`, `distroless/static-debian12`), never `:latest`.
2. `CGO_ENABLED=0` so the binary is static and runs on `distroless/static`; if cgo is
   unavoidable, switch to `distroless/base-debian12` and say why.
3. Copy `go.mod`/`go.sum` and `go mod download` before the source, so the module layer caches.
4. `.dockerignore` at minimum: `.git`, `.agents/scratch`, `web/node_modules`, `*.env*`, `*.pem`, `*.key`.
5. No secrets in any layer: no `COPY .env`, no build args carrying tokens. Secrets arrive at
   runtime (Secret Manager on Cloud Run, `env_file` locally).
6. Health check through the app's `/healthz`. Distroless has no shell or curl, so the
   check is the binary itself (`/app -healthcheck`) or the platform's HTTP probe —
   see [Health Checks](references/health-checks.md).
7. `USER nonroot` stays; bind a port above 1024 (`PORT`, default 8080).

If the Go binary also serves the `web/` frontend, add a `node:<pinned>` build stage
that produces the static bundle and `COPY --from=` it next to the binary; the runtime
image stays distroless.

## Docker Compose (local dependencies)

Compose runs what the app needs locally (Postgres, Redis, an emulator); the app itself
usually runs with `go run ./cmd/<app>` on the host for fast iteration, in a container
only for the smoke test. Pattern in [First-Time Setup](references/setup.md).

```bash
docker compose up -d           # start dependencies
docker compose down -v         # stop and drop volumes (fresh DB)
docker compose logs -f <svc>   # follow one service
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

## Troubleshooting

| Problem | Fix |
|---|---|
| `Cannot connect to the Docker daemon` | macOS: `colima start`; Linux: `sudo -n systemctl start docker` |
| `exec format error` on Cloud Run | arm64 image was pushed — rebuild with `--platform linux/amd64 --load` |
| buildx complains about the builder | `docker buildx create --use` once, then retry |
| amd64 build slow on Apple Silicon | Expected: QEMU emulation. `CGO_ENABLED=0` keeps it a pure Go cross-compile in practice |
| `no such file or directory` on start | Binary linked against cgo on `distroless/static` — set `CGO_ENABLED=0` or use `base-debian12` |
| Port already in use | `lsof -i :<port>` then stop it or change `PORT` |
| Container has no shell for debugging | Run the same binary in `golang:1.24` or use `docker cp` / logs; do not add a shell to the runtime image |
| Compose DB keeps old schema | `docker compose down -v` before `up -d` |

## Reference Library

- [First-Time Setup](references/setup.md) — `.dockerignore`, compose file for local dependencies, first run
- [Health Checks](references/health-checks.md) — `/healthz` handler, `-healthcheck` flag for distroless, compose healthcheck
