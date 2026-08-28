# Handoff — 2026-08-28 — repo rename, family domains, private share links (v1.2.0)

Session ID: 01XNurV4wk2em3VRezrEi7nx
Project: /home/kesokaj/workspace/super-devbox
Branch: main @ `0e0a5e9`, clean. Tags v1.0.0 / v1.1.0 / v1.2.0 — ALL PUSHED.

## Context

styrman, self-hosted multi-user platform running AI coding agents in per-project
Kata microVMs, baremetal home network. This session closed the "push to a
private remote" gap, renamed the project, moved to a family domain layout, and
shipped v1.2.0 (private share links), adversarially reviewed and live.

## Completed

- **Repo is now `github.com/kesokaj/styrman-crew` (private)** — first-ever push,
  main + all tags. Chosen because `kesokaj/styrman` on GitHub is an UNRELATED
  project (headscale dashboard) — never push there. Go module path renamed;
  binary/env/containers/domain keep the name *styrman*.
- **Family domain layout:** app moved `app.` → `crew.styrman.xyz`
  (`STYRMAN_HOSTNAME=crew`); `auth.styrman.xyz` stays zone-level ON PURPOSE so
  future styrman apps share the one Google OAuth client (no console changes
  were needed). styrman.xyz is now an umbrella for future styrman projects.
- **v1.2.0 — private share links (ADR-026):** both sharing modes on
  self-describing wildcards, named by the operator: `*.public.crew.styrman.xyz`
  (TTL capability links, renamed from `preview.`) and `*.private.crew.styrman.xyz`
  (email shares; NEW). Sharing a port mints a stable non-expiring
  `kind=private` token in `internal/links`; the ingress OAuth-gates the
  private wildcard (`writePrivateWildcard` reuses the app's authz policy —
  domain-scoped JWT cookie = free SSO) and `serveShared` re-checks the
  verified email against the port's CURRENT share list per request. Own
  origin per share → no CSP sandbox → stateful apps work for email guests.
  Kind-locked both ways (private token 404s on no-login /link, public on
  /shared) — tested in both directions.
- **Adversarial review done, all findings fixed** (see changelog "Review
  fixes"): blocker = main-origin /link|/shared PATH FORMS must stay
  sandboxed — sandbox now derives from arrival host (origPathKey is the
  discriminator). Cookie-theft concern verified safe in go-authcrunch
  v1.1.41 source (`AUTHP_*` = Secure; HttpOnly).
- Operator env rename: `STYRMAN_PREVIEW_DOMAIN` → `STYRMAN_PUBLIC_DOMAIN`
  (+ new `STYRMAN_PRIVATE_DOMAIN`); compose falls back to the old var.
- Verified: go -race, ingress module tests, integration 62/62, e2e 36/36
  (private link exercised end-to-end incl. revocation), prod healthy at
  v1.2.0, both wildcard certs live, user confirmed the OAuth flow in-browser.

## Open / Next

- **NEXT SESSION (user-stated): frontend focus + new functionalities.** The
  user will bring the list; load the `frontend` skill. Known frontend nits
  from the review, candidates to fold in: copy buttons give no confirmation
  and never show the URL (only in `title`); unhandled `writeText` rejection;
  recoverable React hydration warnings (#418/423/425) tracked in TODO.
- Stale Cloudflare A records `app.styrman.xyz` + `*.preview.styrman.xyz`
  still exist (in-session deletion permission-blocked; harmless — Caddy no
  longer serves those names). Delete via dashboard or grant the API call.
- Deferred TODO: S9 legacy sharedEmails migration, S10 force-close streams
  at link expiry (private links never expire — S10 is public-only).

## Key Decisions

- ADR-026 (this session): two sibling wildcards because the ingress decides
  auth per host block; stable private token minted/killed by the share list;
  the list, not the token, is the authorization.
- Naming: user picked `public`/`private` labels ("the hostname tells you the
  auth model") and repo name `styrman-crew` after a Norse-name exploration.

## Corrections (do not repeat)

- **Never pass `sandbox=false` based on which HANDLER runs — decide by which
  ORIGIN the request arrived on.** The /shared path form on the SPA origin
  with no sandbox = full session-riding escalation (caught by review, was my
  design error). The origPathKey context value is the discriminator.
- A share/link mint failure must fail the API call, not just log — silent
  200s hide broken state from the owner.
- When renaming operator env vars, keep a compose-level fallback for the old
  name — a silent feature-off on redeploy is unacceptable.

## Environment State

- Prod: styrman-server + styrman-caddy healthy at v1.2.0 on crew.styrman.xyz.
- `deploy/.env` (gitignored, secrets): BASE_DOMAIN=styrman.xyz,
  STYRMAN_HOSTNAME=crew, STYRMAN_PUBLIC_DOMAIN=public.crew.styrman.xyz,
  STYRMAN_PRIVATE_DOMAIN=private.crew.styrman.xyz.
- Remote `origin` = git@github.com:kesokaj/styrman-crew.git (ssh, gh authed).

## Artifacts to Read

- `.agents/docs/changes/2026-08-28-private-share-links.md` — feature + review fixes (start here).
- `.agents/docs/changes/2026-08-28-{rename-styrman-crew,family-domain-crew}.md`.
- `.agents/docs/architecture/DECISIONS.md` — ADR-026.
- `.agents/docs/specs/styrman/{api,deploy,guide}.md` — updated.
- `TODO.md`; `internal/httpapi/shared_link_test.go` — the access-matrix tests.

## Suggested Skills

- `frontend` + `typescript` — the stated next focus.
- `code-review` via the reviewer subagent — worked well; keep using it
  before commits.
