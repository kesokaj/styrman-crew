# image/ — the styrman-runtime microVM image

The per-project Kata microVM image: pure tooling, no server binary
(ADR-002). The entrypoint seeds each project's OWN home write-once from
`skel/` (dotfiles) and `harness/` (agent harness — constitution, skills,
subagents — deployed into `~/.claude`, `~/.gemini`, and the vendor-neutral
`~/.agents/bin/` shell gates — with `/usr/local/bin` deliberately trimmed
from that invocation's PATH to dodge a 60s upstream self-check, ADR-047),
sets up per-project Docker-in-Docker on a loop-mounted ext4 disk, and
installs the credential materialization snippet
(`configs/styrman-credentials.sh`, ADR-032).

```bash
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> .
```

`STYRMAN_VERSION` matters: it stamps `/styrman/VERSION`, which keys the
write-once harness seeding (into `~/.claude`, `~/.gemini`, and
`~/.agents/bin/`) in existing homes — a version bump also RE-SEEDS and
RETIRES agents/skills the harness has since dropped.

Full spec: `../.agents/docs/specs/styrman/runtime-image.md`.
