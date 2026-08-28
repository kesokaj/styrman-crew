# 2026-08-28 — Family domain layout: app moves to crew.styrman.xyz

## Summary

styrman.xyz becomes an umbrella for future styrman projects; this app now
lives in its own branch of the namespace. No code changes — config only.

- `app.styrman.xyz` → **`crew.styrman.xyz`** (`STYRMAN_HOSTNAME=crew`)
- `*.preview.styrman.xyz` → **`*.preview.crew.styrman.xyz`**
  (`STYRMAN_PREVIEW_DOMAIN=preview.crew.styrman.xyz`)
- `auth.styrman.xyz` **unchanged** — deliberately zone-level so every future
  styrman app shares the one Google OAuth client and redirect URI.

## Files changed

- `deploy/.env` (untracked) — the two values above.
- `README.md`, `AGENTS.md`, `.agents/docs/specs/styrman/guide.md` — live URL.
- `.agents/docs/specs/styrman/api.md` — `STYRMAN_PREVIEW_BASE` example.
- Historical docs (plans, ADRs, changelogs, TODO) intentionally untouched.

## Verification

- Redeployed prod (`docker compose up -d`); both containers healthy.
- `https://crew.styrman.xyz/` → TLS ok, redirects to Google sign-in via the
  unchanged `auth.styrman.xyz` callback (no Google console change needed).
- Caddy auto-created the DNS record and obtained certs for both
  `crew.styrman.xyz` and `*.preview.crew.styrman.xyz` (Let's Encrypt logs).
- Bogus-token preview host returns 404 with valid TLS (S6 behavior intact).

## Follow-up

- Stale Cloudflare A records `app.styrman.xyz` and `*.preview.styrman.xyz`
  still exist (deletion was permission-blocked in-session); harmless — Caddy
  no longer serves those names. Operator can delete them in the dashboard.
- Side effects as predicted: sessions reset; old preview-link URLs dead
  (all TTL-bound anyway).
