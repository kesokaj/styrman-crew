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
<the build command from the setup answers>
```

## Configure

<!-- Environment variables with defaults, config files, ports. No secrets: name the variable, say where its value comes from. -->

<!-- One row per setting the code reads. No code yet, or no settings: delete the placeholder row and keep the empty table. -->

| Variable | Default | Purpose |
|---|---|---|
| `<VARIABLE>` | `<default>` | `<what it sets>` |

## Run

```bash
<the run command from the setup answers>
```

## Test

```bash
<the test command from the setup answers>
```

## Verify

<!-- The smoke check that proves it is up: a health endpoint, a CLI call, a log line — the real command with the real port, written when the endpoint exists, never before. -->

## Container (when there is a Dockerfile)

<!-- `colima status || colima start`, native-arch build and run for local testing; the amd64 build belongs in gcp.md. -->

## Operator steps

<!-- What the operator does by hand on this target: standing procedures, never pending items (those live in the newest handoff). -->

## Stop and clean up

<!-- How to stop it and what it leaves behind (data dirs, containers, volumes). -->
