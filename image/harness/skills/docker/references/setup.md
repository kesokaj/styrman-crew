# Docker First-Time Setup (Go service)

What a new Go project needs before its first `docker build`. The Dockerfile itself is
in `SKILL.md`.

## Step 1: `.dockerignore`

```
.git
.agents/scratch
web/node_modules
web/.output
*.env*
*.pem
*.key
*.tfvars
*.tfstate
```

Secrets are excluded by name so a stray `COPY . .` cannot bake them into a layer.

## Step 2: `docker-compose.yml` for local dependencies

The app runs on the host (`go run ./cmd/<app>`) during development; compose provides
what it talks to. Add only the services the project actually uses.

```yaml
services:
  db:
    image: postgres:16
    ports:
      - "${DB_PORT:-5432}:5432"
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: app
      POSTGRES_DB: app
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  pgdata:
```

Pin the image tag. Local credentials like `app`/`app` are fine in compose; real ones
never are.

## Step 3: First run

```bash
colima status || colima start        # macOS only
docker compose up -d
docker compose ps                    # db should be "healthy"
DATABASE_URL=postgres://app:app@localhost:5432/app?sslmode=disable go run ./cmd/<app>
```

Then the container smoke test from `SKILL.md` §4, and `docs/deploy/local.md` records
the exact commands for the next session.

## Optional: the app in compose too

Only when a second container must reach the app by name (a worker, an e2e runner):

```yaml
  app:
    build: .
    ports:
      - "${PORT:-8080}:8080"
    environment:
      DATABASE_URL: postgres://app:app@db:5432/app?sslmode=disable
    depends_on:
      db:
        condition: service_healthy
```

`build: .` uses the native architecture, which is what a local run wants; the amd64
build for Google is a separate `buildx` command, never a compose setting.
