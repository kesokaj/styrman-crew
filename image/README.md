# image/ — the styrman-runtime microVM image

The per-project Kata microVM image: pure tooling, no server binary
(ADR-002). The entrypoint seeds each project's OWN home write-once from
`skel/` (dotfiles) and `harness/` (the agent harness — a mirror of
`styrman-harness` at `harness/COMMIT`, installed by the `sh-agent` binary
the Dockerfile builds from it into `~/.claude` and `~/.gemini/config`: global
context, 28 skills, 7 subagents; no hooks, no scripts, no settings files —
ADR-062; a home carrying the previous harness is migrated first by
`configs/harness-migrate.sh`, which moves its artifacts to
`~/.styrman/harness-backup-<ts>/`), sets up per-project Docker-in-Docker on
a loop-mounted ext4 disk, and
installs the credential materialization snippet
(`configs/styrman-credentials.sh`, ADR-032). Also carries the per-language
syntax checkers the editor's Check feature runs inside the guest —
`shellcheck` (apt) and `hadolint` (GitHub release binary, same pattern as
`yq`) round out the set alongside `gofmt`/`python3`/`node`/`jq`/`yq`/`tofu`
that were already here for other tools; full checker table in
`../.agents/docs/specs/styrman/runtime-image.md`.

```bash
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> .
```

`STYRMAN_VERSION` matters: it stamps `/styrman/VERSION`, which keys the
write-once harness seeding in existing homes — a version bump is what
RE-SEEDS an already-onboarded project (`sh-agent install` rewrites the files
it manages and removes the ones it wrote that the harness has since dropped;
`harness/VENDORED.md` has the refresh procedure).

Full spec: `../.agents/docs/specs/styrman/runtime-image.md`.
