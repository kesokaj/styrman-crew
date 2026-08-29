# Setup: only git+ssh required, everything else optional; gcloud added (v1.8.0)

Date: 2026-08-29. Operator direction: in first-run setup only the username,
git identity and ssh key are mandatory; GitHub, Google Cloud, Claude and agy are
optional and can be done later by re-running the setup tool. Also add gcloud
(both `gcloud auth login` and `gcloud auth application-default login`).

## styrman-setup (`image/configs/styrman-setup`)

Restructured into **Required** (git identity, ssh key — unchanged, still
prompted/generated) and **Optional** (GitHub, Google Cloud, Claude, agy). Each
optional tool now asks `[y/N]` before signing in (a new `ask_optional` helper;
non-interactive runs always skip), so any of them can be skipped and picked up
on a later `styrman-setup` run. New **Google Cloud** step (guarded by
`command -v gcloud`): offers `gcloud auth login` (account) and, separately,
`gcloud auth application-default login` (ADC — what terraform and the client
libraries use, and what the broker already syncs). The agy check now looks for
its OAuth token file (`~/.gemini/antigravity-cli/antigravity-oauth-token`), with
the keyring as a fallback.

## Status + UI

- `internal/httpapi/setup.go`: added `gcloud` (ADC file) to the setup status and
  renamed the agy signal from `keyring` to `agy` (file-based check + keyring
  fallback).
- `web/src/components/SetupBanner.tsx` + `web/src/lib/api.ts`: the first-run
  checklist now shows two rows — **required** (git, ssh) and **optional**
  (gh, gcloud, claude, agy) — with the required items highlighted until done.

## Note

The gcloud **ADC** syncs to every project via the broker (existing harvest).
The gcloud **account** login (`gcloud auth login`, in `~/.config/gcloud/`) is
not yet broker-synced — a follow-up if per-project `gcloud` CLI auth is wanted
(ADC covers terraform + client libraries).

## Release

`styrman-runtime:dev` rebuilt with the new script; styrman-server deployed as
1.8.0. Live status now reports `gcloud`/`agy`.
