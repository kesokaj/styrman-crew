# Vendored harness snapshot

This is a snapshot of `devshell_prompt/harness` (the agent harness for
`claude` and `agy`), vendored into the runtime image so every project gets
the SAME defaults from the base image (R5/ADR-030) — no shared home, no
host-side deploy dependency.

The image entrypoint runs `setup.sh` into each project home write-once
(marker `~/.styrman/.harness-version`). To refresh: re-copy from
`devshell_prompt/harness`, bump the image, redeploy.

Source of truth remains `devshell_prompt/harness`; do not edit here.
