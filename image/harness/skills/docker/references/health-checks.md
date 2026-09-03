# Docker Health Checks

Comprehensive health monitoring for Docker development containers.

## Container Healthcheck in docker-compose.yml

```yaml
services:
  dev:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: my-project-dev-1
    ports:
      - '${DEV_PORT:-3000}:3000'
    volumes:
      - .:/app
      - node_modules:/app/node_modules
    environment:
      - NODE_ENV=development
    command: npm run dev -- --host 0.0.0.0
    healthcheck:
      test: ['CMD', 'node', '-e', "console.log('healthy')"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    profiles:
      - dev

volumes:
  node_modules:
```

## Health Check Script

Create `scripts/docker-health.sh` for pre-test verification:

```bash
#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${CONTAINER_NAME:-my-project-dev-1}"
COMPOSE_PROFILE="${COMPOSE_PROFILE:-dev}"
MAX_WAIT_SECONDS=60
CHECK_INTERVAL=2

check_docker() {
    command -v docker &>/dev/null || { echo "[ERROR] Docker not in PATH"; exit 1; }
    docker info &>/dev/null || { echo "[ERROR] Docker daemon not running (colima start?)"; exit 1; }
}

is_container_running() {
    docker ps --filter "name=$CONTAINER_NAME" --format "{{.Names}}" 2>/dev/null | grep -q "$CONTAINER_NAME"
}

is_container_healthy() {
    local status
    status=$(docker inspect --format='{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo "none")
    [ "$status" = "healthy" ]
}

wait_for_healthy() {
    local elapsed=0
    echo "[INFO] Waiting for container to be healthy..."
    while [ $elapsed -lt $MAX_WAIT_SECONDS ]; do
        is_container_healthy && { echo "[INFO] Container is healthy!"; return 0; }
        is_container_running || { echo "[ERROR] Container stopped"; docker logs "$CONTAINER_NAME" --tail 20; return 1; }
        sleep $CHECK_INTERVAL
        elapsed=$((elapsed + CHECK_INTERVAL))
    done
    # Fallback: check if responsive
    docker exec "$CONTAINER_NAME" node -e "console.log('ready')" &>/dev/null && { echo "[INFO] Container responsive"; return 0; }
    echo "[ERROR] Container not responsive"; return 1
}

main() {
    check_docker
    if is_container_running; then
        echo "[INFO] Container '$CONTAINER_NAME' running"
        is_container_healthy || docker exec "$CONTAINER_NAME" node -e "console.log('ready')" &>/dev/null || {
            echo "[WARN] Restarting..."
            docker compose --profile "$COMPOSE_PROFILE" restart
            wait_for_healthy
        }
    else
        echo "[INFO] Starting container..."
        docker compose --profile "$COMPOSE_PROFILE" up -d
        wait_for_healthy
    fi
    echo "[INFO] Docker environment ready!"
}

main "$@"
```

## NPM Scripts Integration

```json
{
  "scripts": {
    "pretest": "bash scripts/docker-health.sh 2>/dev/null || true",
    "test": "vitest run",
    "docker:health": "bash scripts/docker-health.sh"
  }
}
```

## Healthcheck Configuration

| Option | Description | Recommended |
|--------|-------------|-------------|
| `test` | Command to run | `['CMD', 'node', '-e', "console.log('healthy')"]` |
| `interval` | Time between checks | `30s` |
| `timeout` | Max time per check | `10s` |
| `retries` | Failures before unhealthy | `3` |
| `start_period` | Grace period on start | `10s` |

## Troubleshooting

### Container Shows "unhealthy"
```bash
docker inspect --format='{{json .State.Health}}' <container> | jq
```

### Health Check Never Passes
```bash
docker exec <container> which node
docker exec <container> node -e "console.log('healthy')"
```

Increase `start_period` if container needs more initialization time.
