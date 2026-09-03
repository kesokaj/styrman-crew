---
owner: documentation
date: YYYY-MM-DD
target: gcp
---
# Deploy — GCP

Cloud Run first (`cloud-run-deploy` skill); GKE or Compute Engine only when Cloud Run does not fit. GCP is the only cloud. Nothing here runs unless the user says to deploy.

**Not applicable yet:** this project has no GCP target. Delete this line and fill the defaults when the user asks for one.

## Recorded defaults

Asked once, per the global context, never invented or copied from another project. `AGENTS.md` points here; the next session reads this table instead of asking again.

| Setting | Value |
|---|---|
| Project ID | |
| Region | |
| Artifact Registry repository | |
| Service name | |
| Public (unauthenticated) | yes / no |
| Service account | |

## Image

`${REGION}-docker.pkg.dev/${PROJECT}/${REPO}/${SERVICE}:${GIT_SHA}` — built with `docker buildx build --platform linux/amd64 --load`, tagged with the short SHA; `latest` only as a convenience alias.

## Procedure

Follow the `cloud-run-deploy` skill with the defaults above: local test build and smoke run, amd64 build, push, `gcloud run deploy`, verify. Project-specific flags (memory, concurrency, VPC, min instances) and the reason for each go here.

## Secrets and configuration

<!-- Every secret via Secret Manager and `--set-secrets`, never `--set-env-vars`. List the names, not the values. -->

## Verify

<!-- `gcloud run services describe`, the health URL, the log query that shows the new revision serving. -->

## Rollback

```bash
gcloud run services update-traffic ${SERVICE} --region ${REGION} --to-revisions <previous>=100
```

## Infrastructure

<!-- When it is more than one service: the Terraform module path and how to plan/apply (`terraform` skill). -->
