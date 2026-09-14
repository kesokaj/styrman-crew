# image/ — the styrman-runtime microVM image

The per-project Kata microVM image: pure tooling, no server binary
(ADR-002). The entrypoint seeds each project's OWN home write-once from
`skel/` (dotfiles) and `harness/` (the agent harness — a mirror of
`styrman-harness` at `harness/COMMIT`, installed by the `sh-agent` binary
the Dockerfile builds from it into `~/.claude` and `~/.gemini/config`: global
context, 27 skills, 7 subagents; no hooks, no scripts, no settings files —
ADR-062; the mirror has no `.git`, so `sh-agent install` treats it as a plain
copy — it never rebuilds the binary and never runs git (harness BUG-015); a home carrying the previous harness is migrated first by
`configs/harness-migrate.sh`, which moves its artifacts to
`~/.styrman/harness-backup-<ts>/`), sets up per-project Docker-in-Docker on
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
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> .
```

`STYRMAN_VERSION` matters: it stamps `/styrman/VERSION`, which keys the
write-once harness seeding in existing homes — a version bump is what
RE-SEEDS an already-onboarded project (`sh-agent install` rewrites the files
it manages and removes the ones it wrote that the harness has since dropped;
`harness/VENDORED.md` has the refresh procedure).

Full spec: `../.agents/docs/specs/styrman/runtime-image.md`.
