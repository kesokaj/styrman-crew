---
name: gcloud-setup
description: |
  GCloud CLI authentication and project configuration. Use when encountering "ADC not found",
  "gcloud auth error", "No credentialed accounts", "Application Default Credentials", "project not set",
  "permission denied", a 403 from gcloud, bq, or a client library, or when setting up gcloud CLI
  for a new machine or project.
  Activate for any gcloud auth, gcloud config, or GCP credential question.
  Do NOT use for GKE cluster management (use gke-basics) or Terraform GCP resources (use terraform).
---

# GCloud CLI Setup & Authentication

Quick-reference for configuring `gcloud` CLI authentication and project settings (macOS or Linux).

**When NOT to use this skill:** GKE cluster management (use the `gke-basics` skill), Terraform GCP resources (use the `terraform` skill), or Cloud Run deployments (use the `cloud-run-deploy` skill).

## Quick Fixes

| Problem | Fix |
|---------|-----|
| ADC not found | `gcloud auth application-default login` |
| Project not set | `gcloud config set project PROJECT_ID` |
| Permission denied | Check IAM roles in Cloud Console |
| Token expired | `gcloud auth application-default login` (re-run) |
| Quota exceeded | Check quotas in Cloud Console |
| Wrong account | `gcloud auth login` (re-authenticate) |

## First-Time Setup

### 1. Install gcloud CLI

```bash
# macOS (Homebrew — recommended)
brew install google-cloud-sdk
```

### 2. Authenticate

```bash
# Login with your Google account (opens browser)
gcloud auth login

# Set up Application Default Credentials (required for SDKs and APIs)
gcloud auth application-default login
```

### 3. Set Default Project

```bash
# List available projects
gcloud projects list

# Set default project
gcloud config set project YOUR_PROJECT_ID

# Verify
gcloud config get project
```

### 4. Verify Setup

```bash
# Check authenticated accounts (* = active)
gcloud auth list

# Check current project
gcloud config get project

# Test API access
gcloud services list --enabled --limit=5
```

## Common Configurations

### Set Default Region/Zone

```bash
gcloud config set compute/region us-central1
gcloud config set compute/zone us-central1-a
```

### Multiple Configurations (Named Profiles)

```bash
# Create a named config for a different project
gcloud config configurations create my-staging
gcloud config set project staging-project-id
gcloud config set compute/region europe-west1

# Switch between configs
gcloud config configurations activate default
gcloud config configurations activate my-staging

# List all configs
gcloud config configurations list
```

### Artifact Registry Authentication

```bash
# Configure Docker to push to Artifact Registry
gcloud auth configure-docker REGION-docker.pkg.dev
```

## Before asking the user to log in

1. Verify first: `gcloud auth list`. `No credentialed accounts.` means missing authentication; an account that IS listed plus a `403` means IAM permissions on that account, not authentication — investigate the roles instead of asking for a new login. The same applies to `bq`, client libraries and notebooks (`DefaultCredentialsError`).
2. Never fix credentials by code or alternative tools; the user runs `gcloud auth login` and `gcloud auth application-default login` in their own terminal.
3. After they confirm, `gcloud auth list` again, then continue the original task.

The project ID is never discovered from `gcloud config` or `.env`: the global context has the agent ask for the project's GCP defaults once and record them in the project's `docs/deploy/gcp.md`.

## Troubleshooting

### ADC Location

Application Default Credentials are stored at:
```
~/.config/gcloud/application_default_credentials.json
```

To reset: delete that file and re-run `gcloud auth application-default login`.

### Service Account Keys (CI/CD only)

For CI/CD pipelines (not local dev):
```bash
# Activate service account
gcloud auth activate-service-account --key-file=key.json

# Set ADC from service account
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

> **Never commit service account keys to git.** Use Workload Identity for GKE, or Secret Manager for other compute.
