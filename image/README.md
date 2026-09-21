# image/ — the styrman-runtime microVM image

The per-project Kata microVM image: pure tooling, no server binary
(ADR-002). The entrypoint seeds each project's OWN home write-once from
`skel/` (dotfiles) and installs the agent harness with `/styrman/bin/sh-agent`
— the `styrman-harness` RELEASE binary, harness embedded (global context, 27
skills, 7 subagents; no hooks, no scripts, no settings file but the one
`attribution` key), downloaded from GitHub Releases at image build at the
version `ARG HARNESS_VERSION` names and verified against the harness's
signer (ADR-074, replacing the ADR-062 mirror and the ADR-073 channel).
`sh-agent install` runs once per image version + harness release into each
home and never downgrades a home whose user ran `sh-agent update` inside the
project — that command is how a project moves to a newer harness between
image bumps; a home carrying the pre-ADR-062 harness is migrated first by
`configs/harness-migrate.sh`, which moves its artifacts to
`~/.styrman/harness-backup-<ts>/`, sets up per-project Docker-in-Docker on
a loop-mounted ext4 disk, and
installs the credential materialization snippet
(`configs/styrman-credentials.sh`, ADR-032). Also carries the per-language
syntax checkers the editor's Check feature runs inside the guest —
`shellcheck` (apt) and `hadolint` (GitHub release binary, same pattern as
`yq`) round out the set alongside `gofmt`/`python3`/`node`/`jq`/`yq`/`tofu`
that were already here for other tools; full checker table in
`../.agents/docs/specs/styrman/runtime-image.md`. Since ADR-068 it also
carries the project browser: Debian's headless `chromium`, the pinned
`chrome-devtools-mcp`, and `configs/styrman-browser` (installed to
`/styrman/bin`), one Chromium per project that the operator watches from the
SPA's Browser view and the agents drive through an MCP server the entrypoint
registers for `claude` and `agy` — the "Browser" section of the spec has the
ports, sandbox and registration details.

```bash
GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN \
  -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> .
```

The token is a build SECRET (never a layer) and must read the private
`kesokaj/styrman-harness` repository; on the host the updater passes its own
token file the same way. Bumping the harness a release ships is editing the
`ARG HARNESS_VERSION=vX.Y.Z` line in the Dockerfile.

`STYRMAN_VERSION` matters: it stamps `/styrman/VERSION`, which keys the
write-once harness seeding in existing homes — a version bump is what
RE-SEEDS an already-onboarded project (`sh-agent install` rewrites the files
it manages and removes the ones it wrote that the harness has since dropped)
— unless the home already holds a newer release than the image carries.

Full spec: `../.agents/docs/specs/styrman/runtime-image.md`.
