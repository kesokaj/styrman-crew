# Docker Health Checks (Go service)

The runtime image is distroless: no shell, no `curl`, no `wget`. A health check must
therefore be either the binary itself or an HTTP probe from outside the container.

## 1. The `/healthz` handler

Every service exposes it; `cloud-run-deploy` verifies a release by hitting it.

```go
mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
    // Liveness only: no downstream calls. A readiness check that pings the DB
    // belongs on a separate path (/readyz) so a slow dependency cannot restart the pod.
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ok"))
})
```

## 2. `-healthcheck` flag for Dockerfile `HEALTHCHECK`

The binary probes itself, so the check works on `distroless/static`:

```go
// in cmd/<app>/main.go, before the server starts
if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
    port := cmp.Or(os.Getenv("PORT"), "8080")
    resp, err := http.Get("http://127.0.0.1:" + port + "/healthz")
    if err != nil || resp.StatusCode != http.StatusOK {
        os.Exit(1)
    }
    os.Exit(0)
}
```

```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app", "-healthcheck"]
```

Cloud Run ignores `HEALTHCHECK`; it has its own HTTP startup and liveness probes
(`gcloud run deploy --startup-probe httpGet.path=/healthz`), configured at deploy time.
The Dockerfile line is for local Docker and compose.

## 3. Compose `healthcheck` for the app service

```yaml
  app:
    build: .
    healthcheck:
      test: ["CMD", "/app", "-healthcheck"]
      interval: 10s
      timeout: 3s
      retries: 5
      start_period: 5s
```

Dependencies use their own tools (`pg_isready`, `redis-cli ping`); see `setup.md`.

## 4. Waiting for healthy before a test run

```bash
docker compose up -d --wait          # blocks until every healthcheck passes, or fails
docker compose up -d --wait --wait-timeout 60
```

`--wait` replaces any hand-written polling script. A container that never becomes
healthy fails the command with a non-zero status; read `docker compose logs <svc>`.

## Troubleshooting

| Symptom | Check |
|---|---|
| `unhealthy` in `docker ps` | `docker inspect --format='{{json .State.Health}}' <container>` |
| Healthcheck exec fails immediately | The check names a binary the image lacks (`curl` on distroless) — use `/app -healthcheck` |
| Healthy locally, Cloud Run restarts | Cloud Run probes `PORT` from the environment; the app must listen on it, not a hardcoded port |
| Slow first check | Raise `start_period`; the app should not be doing migrations in the request path |
