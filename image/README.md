# image/ — the styrman-runtime microVM image

The per-project Kata microVM image: pure tooling, no server binary
(ADR-002). The entrypoint seeds each project's OWN home write-once from
`skel/` (dotfiles) and `harness/` (agent harness), sets up per-project
Docker-in-Docker on a loop-mounted ext4 disk, and installs the credential
materialization snippet (`configs/styrman-credentials.sh`, ADR-032).

```bash
docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=<ver> .
```

`STYRMAN_VERSION` matters: it stamps `/styrman/VERSION`, which keys the
write-once harness seeding in existing homes.

Full spec: `../.agents/docs/specs/styrman/runtime-image.md`.
