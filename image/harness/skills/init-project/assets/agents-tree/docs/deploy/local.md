---
owner: documentation
date: YYYY-MM-DD
target: local
---
# Deploy — local

How to build, configure, and run this project on the machine in front of you. `/init-project` fills it from the setup answers; the `documentation` agent keeps it current (Definition of Done step 3) whenever the build, run, or configuration path changes. The `verifier` follows it when a plan names a local smoke test.

## Prerequisites

<!-- Toolchain and versions (Go 1.24+, Node …), Colima/Docker if there is a Dockerfile, anything installed once per machine. -->

## Build

```bash
go build ./...
```

## Configure

<!-- Environment variables with defaults, config files, ports. No secrets: name the variable, say where its value comes from. -->

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Listen port |

## Run

```bash
go run ./cmd/server
```

## Verify

<!-- The smoke check that proves it is up: a health endpoint, a CLI call, a log line. -->

```bash
curl -i localhost:8080/healthz
```

## Container (when there is a Dockerfile)

<!-- `colima status || colima start`, native-arch build and run for local testing; the amd64 build belongs in gcp.md. -->

## Stop and clean up

<!-- How to stop it and what it leaves behind (data dirs, containers, volumes). -->
