---
owner: documentation
date: 2026-09-03
target: gcp
---
# Deploy — GCP

**Not applicable.** styrman is self-hosted on the operator's own Debian host (`remote.md`). GCP appears in this project only as an optional place to run that host (a VM with nested virtualization, `README.md`) and as one of the tools the credential broker makes available inside project microVMs (`gcloud` ADC). No Cloud Run service, no Artifact Registry, no defaults to record.

If that changes, fill the template the harness ships (`~/.claude/skills/init-project/assets/agents-tree/docs/deploy/gcp.md`): the five defaults, image, procedure via the `cloud-run-deploy` skill, secrets, verify, rollback.
