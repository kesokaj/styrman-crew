# AGENTS.md

## Project Overview

<!-- Brief description: what this project does, who it serves, and why it exists. -->

## Setup Commands

<!-- Project-specific build, run, and test commands. -->

```bash
# Install dependencies
go mod download

# Build
go build ./cmd/...

# Run
go run ./cmd/server/

# Test
go test ./...

# Lint
go vet ./... && staticcheck ./...
```

## Architecture

<!-- Project-specific architecture notes. -->
<!-- For ADRs, see .agents/docs/architecture/DECISIONS.md -->

## Project-Specific Standards

<!-- Only list standards that DIFFER from the global context (~/.claude/AGENTS.md / ~/.gemini/config/rules/AGENTS.md). -->
<!-- If nothing differs, delete this section. -->

## Testing

<!-- Project-specific testing instructions that supplement the global testing tiers. -->
<!-- For detailed guides, see .agents/docs/testing/ -->

## Deploy

<!-- One line per target that exists. The procedures live in .agents/docs/deploy/ (local.md, remote.md, gcp.md); GCP defaults are recorded in gcp.md, never invented. -->

Local only — see `.agents/docs/deploy/local.md`.

## Key Files

<!-- Help agents navigate this codebase quickly. -->

| Path | Purpose |
|------|---------|
| `cmd/server/main.go` | Application entry point |
| `internal/config/` | Configuration loading |
| `internal/api/` | HTTP/gRPC handlers |
| `REQUIREMENTS.md` | Non-negotiable project requirements |
| `.agents/docs/` | All project documentation |
