# Docker First-Time Setup

Detailed setup guide for configuring Docker development environments in new projects.

## Step 1: Determine Configuration

**Q1: Does your project use native Node.js modules?**

| Module | Native? | Requires glibc? |
|--------|---------|-----------------|
| `better-sqlite3` | Yes | Yes |
| `sharp` | Yes | Yes |
| `bcrypt` | Yes | Yes |
| `node-gyp` builds | Yes | Usually |
| Pure JS packages | No | No |

```bash
# Check for native module indicators
grep -E "node-gyp|prebuild|nan|napi" package.json package-lock.json 2>/dev/null | head -5
```

**Q2: Base image priority?**

| Priority | Recommended Base Image |
|----------|------------------------|
| Smallest image size | `node:20-alpine` (if no native modules) |
| Maximum compatibility | `node:20-slim` (Debian, has glibc) |
| Full tooling | `node:20` (full Debian) |

## Step 2: Generate Docker Files

### With Native Modules (Recommended Default)

```dockerfile
FROM node:20-slim
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 make g++ git \
    && rm -rf /var/lib/apt/lists/*

COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build --if-present

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
```

### Without Native Modules (Smaller Image)

```dockerfile
FROM node:20-alpine
WORKDIR /app

RUN apk add --no-cache python3 make g++ git

COPY package*.json ./
RUN npm install
COPY . .
RUN npm run build --if-present

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
```

### docker-compose.yml

```yaml
services:
  dev:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: ${COMPOSE_PROJECT_NAME:-my-project}-dev-1
    ports:
      - "${DEV_PORT:-3000}:3000"
    volumes:
      - .:/app
      - node_modules:/app/node_modules
    environment:
      - NODE_ENV=development
    command: npm run dev -- --host 0.0.0.0
    profiles:
      - dev

volumes:
  node_modules:
```

## Step 3: Start and Verify

```bash
# Ensure Colima is running
colima status || colima start

# Start container
docker compose --profile dev up -d

# Verify
docker ps --filter "name=my-project"

# Install dependencies (first time)
docker exec my-project-dev-1 npm install
```

## Troubleshooting

### ERR_DLOPEN_FAILED

**Cause:** Using Alpine image with modules that require glibc.

```bash
docker compose --profile dev down
docker volume rm <project>_node_modules
docker compose --profile dev build --no-cache
docker compose --profile dev up -d
docker exec <container> npm install
```

### Decision Tree: Alpine vs Slim

```
Does package.json contain native modules?
│
├─ YES → Use node:20-slim (glibc)
│        └─ Examples: sharp, bcrypt, sqlite3
│
├─ UNSURE → Use node:20-slim (safe default)
│
└─ NO (pure JS only) → Use node:20-alpine (smaller)
         └─ Examples: express, react, typescript
```
