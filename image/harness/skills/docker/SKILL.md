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

Expert guidance for container-based development using Docker — via **Colima** on macOS (ARM/Apple Silicon), or the native Docker daemon on Linux (skip the Colima steps there).

**When NOT to use this skill:** Kubernetes orchestration (use the `gke-basics` skill), infrastructure-as-code (use the `terraform` skill), or CI/CD pipeline design (use the `bash` skill).

## Environment Setup

**Runtime:** Docker via Colima (not Docker Desktop).

```bash
# Ensure Colima is running
colima status || colima start

# Verify Docker
docker info
docker compose version
```

## Core Rules

### 1. Colima First
Always verify Colima is running before any Docker operation:
```bash
colima status || colima start
```

### 2. Multi-Architecture Builds
Build for both ARM (local Mac) and AMD64 (Linux/Cloud Run):
```bash
# Local dev (ARM)
docker build -t myapp:dev .

# Production (multi-arch for GCP)
docker buildx build --platform linux/amd64,linux/arm64 -t myapp:latest .
```

### 3. Container Command Execution
Use `docker exec` WITHOUT `-it` flag in agent contexts (no TTY available):
```bash
# Correct
docker exec mycontainer npm test

# Wrong (fails with "not a TTY")
docker exec -it mycontainer npm test
```

### 4. Registry: GCP Artifact Registry
Push to Artifact Registry as the primary registry:
```bash
# Tag for Artifact Registry
docker tag myapp:latest REGION-docker.pkg.dev/PROJECT/REPO/myapp:latest

# Push
docker push REGION-docker.pkg.dev/PROJECT/REPO/myapp:latest
```

## Docker Compose Patterns

### Start/Stop Services
```bash
docker compose up -d           # Start all services
docker compose down            # Stop and remove
docker compose restart         # Restart
docker compose up -d --build   # Rebuild and start
```

### Check Status
```bash
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
docker logs <container> --tail 50
```

### Execute Commands in Running Container
```bash
docker exec <container> npm install <package>
docker exec <container> npm test
docker exec <container> npm run build
```

## When to Use docker exec

| Operation | Use docker exec? | Reason |
|-----------|------------------|--------|
| `npm install` | **Yes** | Packages install in container only |
| `npm run dev` | No | Already running via docker-compose |
| `npm test` | Yes | Tests run in container environment |
| `npm run build` | Yes | Build happens in container |
| `git` commands | No | Git runs on host |
| File editing | No | Volume mount syncs automatically |

## Dockerfile Best Practices

```dockerfile
# Use specific version tags, never :latest for base
FROM node:20-slim AS builder

# Set working directory
WORKDIR /app

# Copy dependency files first (layer caching)
COPY package.json package-lock.json ./
RUN npm ci --only=production

# Copy source code
COPY . .
RUN npm run build

# Production stage
FROM node:20-slim
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
EXPOSE 3000
CMD ["node", "dist/index.js"]
```

**Key rules:**
1. Use multi-stage builds to minimize image size
2. Copy dependency files before source (layer caching)
3. Use `node:20-slim` by default — Alpine only if no native modules
4. Never run as root in production (add `USER node`)
5. Use `.dockerignore` to exclude `node_modules/`, `.git/`, etc.

## Volume Mount Patterns

```yaml
# docker-compose.yml
services:
  app:
    volumes:
      - .:/app                           # Source code (synced)
      - node_modules:/app/node_modules   # Dependencies (container-only)

volumes:
  node_modules:  # Named volume — persists between restarts
```

**Key:** `node_modules` lives in a named Docker volume, NOT on host filesystem. Host stays clean.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Container not running | `colima status \|\| colima start && docker compose up -d` |
| Port already in use | `lsof -i :<port>` then kill or change port |
| Module not found | `docker compose down && docker compose build --no-cache && docker compose up -d` |
| File changes not syncing | `docker inspect <container> \| grep -A 10 "Mounts"` |
| Platform warnings (M1/M2) | Add `platform: linux/amd64` to docker-compose service |
| `ERR_DLOPEN_FAILED` | Switch from Alpine to Slim, or rebuild: `npm rebuild` |

## Reference Library

For detailed setup and health check patterns:
- [First-Time Setup](references/setup.md) — Project scaffolding, native module troubleshooting
- [Health Checks](references/health-checks.md) — Container healthcheck configuration, monitoring scripts
