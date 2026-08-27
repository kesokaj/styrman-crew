# Handoff — 2026-08-27 — port sharing + public preview links (v1.1.0)

Session ID: 01CwBm5VxevwnoWsQUWnc1CY
Project: /home/kesokaj/workspace/super-devbox
Branch: main @ `c66803e`, clean (only `web/dist/` shows as dirty — it's a build artifact)
Tags: `v1.0.0`, `v1.1.0` (both local only — never pushed)

## Context

styrman is a self-hosted multi-user platform running AI coding agents in per-project
Kata microVMs, deployed on the user's baremetal home network (domain `styrman.xyz`).
This session added two ways to share a project's exposed ports, hardened them through
two independent reviews, tagged v1.0.0 + v1.1.0, and redeployed prod.

## Completed

- **Per-port email sharing (v1.0.0):** owner grants specific Google emails access to
  ONE exposed port. `project.SharedPorts map[int][]string`; `GET/PUT
  /api/projects/{id}/ports/{port}/shares`. Ingress flipped to ModeAuthenticated so a
  shared guest can sign in; `authGuard` is the gate (`identity.User.Authorized`).
- **Public TTL preview links (v1.1.0):** signed, expiring, no-login links to one port at
  `https://<token>.preview.styrman.xyz/`. `internal/links` store; owner mint/list/revoke
  + public `/link/{token}` handler; `previewHostRewrite` maps the subdomain. Ingress:
  `caddy_preview_target` label + `PREVIEW_DOMAIN` → a `*.PREVIEW_DOMAIN` ModePublic
  wildcard + wildcard DNS + auto cert. Enabled by `STYRMAN_PREVIEW_DOMAIN` (set live).
- **Two adversarial reviews done and remediated** (pre-1.0.0 + 1.1) — no auth holes;
  all blockers/should-fixes fixed and verified live.
- Prod redeployed at v1.1.0, healthy. go test -race 8/8, integration 62/62, e2e 36/36.
- Docs updated: README, api.md, deploy.md, ADR-024/025, changelogs, TODO, plan (SHIPPED).

## Open / In Progress

- **TOP PRIORITY (user-scheduled for next session): push to a PRIVATE git remote.**
  No remote configured, `gh` not authenticated → nothing is pushed. See Next Steps.
- Deferred TODO (documented, low-priority for a home-network deploy): **S9** legacy
  `sharedEmails`→`SharedPorts` migration (only unreleased intermediate data); **S10**
  force-close a WS/stream opened just before link expiry; guest app sees `Host:<ip>`;
  `linkURL` returns a login-gated path form when `STYRMAN_PREVIEW_BASE` unset.
- Browser OAuth flows can't be tested headlessly; rest on the user's confirmation +
  live curl-through-ingress checks.

## Key Decisions

- **Per-port, not project-wide sharing** — least privilege (project-wide exposed every
  port = port-scan surface). See ADR-024.
- **Public links: stateful token store, not a signed blob** — so links are revocable +
  listable. **Per-token subdomain** → each preview gets its own origin (clean root +
  free cross-origin isolation). See ADR-025.
- **CSP sandbox only on the same-origin `/p/` preview**, NOT on public links (their own
  origin already isolates them; sandbox broke their same-origin fetches).
- Kept the sandbox for `/p/` (operator chose security over stateful-app support there).

## Corrections (do not repeat)

- The share "remove" button was `opacity-0`/hover-only → user couldn't delete a share.
  Access-control controls must be **always visible**. (e2e now covers add+remove.)
- e2e originally only fetched a static `i.html`, which HID the CSRF/CSP bugs that broke
  every mutation from a previewed app. Test **mutating** paths, not just GET.
- Don't deploy a "public link" UI whose links aren't actually public yet (staged the UI
  until the ingress route existed).

## Environment State

- Branch `main`, clean (bar `web/dist/`). HEAD `c66803e`. Tags `v1.0.0`, `v1.1.0` local.
- Prod: `styrman-server` + `styrman-caddy` up & healthy, v1.1.0. `STYRMAN_PREVIEW_DOMAIN=
  preview.styrman.xyz` live in `deploy/.env` (gitignored). Wildcard DNS + cert issued.
- Secrets in `deploy/.env` (600, gitignored) — never rotate per the user (home network).

## Artifacts to Read

- `.agents/code-review/2026-08-27-1.1-review.md` — the 1.1 review + fix status (start here).
- `.agents/code-review/2026-08-27-pre-1.0.0-review.md` — the earlier review + resolutions.
- `.agents/docs/architecture/DECISIONS.md` — ADR-024 (per-port sharing), ADR-025 (links).
- `.agents/docs/plans/public-ttl-preview-links.md` — the links design (marked SHIPPED).
- `.agents/docs/specs/styrman/{api,deploy}.md` — current API + deploy config.
- `TODO.md` — S9/S10 + nits, the only remaining code work.
- `.agents/docs/testing/styrman/integration.sh` (62 checks), `web/e2e/run.sh` (36).

## Suggested Skills

- `code-review` — if any of the deferred TODO items get implemented.
- `golang` / `frontend` — respective code areas.
- `feedback` then `/retro` — the two test-coverage corrections above are worth logging.

## Next Steps

1. **Set up the private remote and push (top priority).** Have the user run
   `! gh auth login`, then `gh repo create kesokaj/styrman --private --source=. --remote=origin --push`
   and `git push origin v1.0.0 v1.1.0`. OR, if they give a remote URL,
   `git remote add origin <url>` and push (no gh needed). Also see the memory
   `next-priority-git-remote`.
2. Optional: knock out the deferred TODO (S9 migration, S10 stream-close at expiry).
3. Everything is deployed and healthy — no urgent code work; the push is the one gap.
