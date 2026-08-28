# Credential-service P1, Increment 1 — base image carries the harness

Date: 2026-08-28. First increment of ADR-030 (per-project homes + credential
broker). Plan: `.agents/docs/plans/2026-08-28-credential-service.md`.
Safe/foundational; no credential handling, no home restructure yet.

## What changed

- **Harness vendored into the runtime image** (`image/harness/`, snapshot of
  `devshell_prompt/harness`, 136 text files, ~1.6M; `VENDORED.md` explains
  refresh). The Dockerfile `COPY`s it to `/styrman/harness/`.
- **Entrypoint seeds the harness into each project home** write-once, keyed
  on the image's `/styrman/VERSION` (`~/.styrman/.harness-version`), run as
  the guest user. Deploys the claude/agy skills/subagents/commands into
  `~/.claude` and `~/.gemini` — NOT credentials (verified `.credentials.json`
  untouched). Non-fatal: a harness error never blocks boot.
- Runs from a user-owned throwaway copy of the harness: `setup.sh` also
  refreshes files inside its own tree (GEMINI.md ← shared/AGENTS.md), which
  fails on the root-owned image path — the throwaway makes that write land
  harmlessly while the HOME deploy still happens.
- **R5 revised** (REQUIREMENTS.md, operator-approved): the harness + default
  config are carried by the base image and seeded identically into every
  project home, replacing the shared-home/host-side deploy. This also fixes
  the long-standing multi-user harness gap (TODO: users 2+ never got it).
- **R3 revised** + **ADR-030 recorded** for the full P1/P2 design (per-project
  homes + credential broker); Increments 2 (broker + adapters) and 3
  (per-project homes + guarded migration) follow.

## Why this first

It's the prerequisite for per-project homes (each home must get identical
defaults from the image, not a shared home), it's safe on the live box
(additive write-once seeding), and it delivers immediate value (every
project — not just the operator's — now gets the harness).

## Verification

Image rebuilt; a real prod project recreated on it — harness deployed
(skills present), version stamp written, credentials intact. go test + e2e
(see session log).

## Not yet done (next increments)

- Inc 2: the credential broker + per-tool adapters (ssh-agent, git/docker/
  gcloud helpers, gh/npm env, claude apiKeyHelper, agy keyring-seed).
- Inc 3: per-project home volumes + the guarded, BACKED-UP migration of the
  existing shared home (credentials must never be lost — final go-ahead
  before running it on live credentials).
