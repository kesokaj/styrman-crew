# Clean reinstall of the live server (operator-requested)

Date: 2026-08-28. Operator: "reinstall clean the live server. Remove
everything old... no old volumes... everything new. No old projects
nothing." Purpose: exercise the fresh-install path (R11) and the new-user
experience end to end on v1.5.1.

## Removed (everything)
- Containers: styrman-server, styrman-caddy, the project microVM.
- Networks: styrman-ingress, the project's sdb-<uuid> bridge.
- Volumes: caddy_data + caddy_config (certs/ACME account re-issued fresh).
- Images: styrman-server, styrman-caddy, styrman-runtime (:dev + :latest) —
  full from-scratch rebuild.
- State: /var/lib/styrman entirely (homes, projects, credentials, users,
  links.json, settings.json).
- Rotated in deploy/.env: STYRMAN_JWT_KEY (invalidates every session →
  forced fresh Google sign-in), STYRMAN_INGRESS_TOKEN, STYRMAN_KEYRING_PW.

## Kept (operator directive)
deploy/.env's operator secrets: Google OAuth client id/secret, Cloudflare
API token, ADMIN_EMAIL/STYRMAN_ALLOWED_EMAILS, BASE_DOMAIN + public/private
domains, UID/GID/user/hostname. (Safety copy of the pre-rotation .env in
the session scratchpad only — not in git.)

## Reinstall (the documented path, exactly)
1. `sudo STYRMAN_IMAGE=styrman-runtime:dev bash deploy/host-setup.sh --user kesokaj`
   — idempotent converge on the existing Docker/Kata; created the CURRENT
   state layout (empty `homes/` + `projects/` roots — the audit's fix);
   built the runtime image from scratch, version-stamped by git describe
   (`v1.5.1-1-g65929e6` — the audit's harness-reseed fix).
2. `STYRMAN_VERSION=1.5.1 docker compose up -d --build` — server + caddy
   images rebuilt; fresh ACME account; certificates re-issued for
   crew.styrman.xyz, auth.styrman.xyz, *.public.crew…, *.private.crew….

## Verified fresh (new-user experience)
- `GET /api/projects` → `[]`; `GET /api/setup/status` → all false (the
  first-time setup banner shows); `setupComplete:false`,
  `terminalUsernameChosen:false` (setup asks for the username);
  `GET /api/settings/credentials` → `{brokerEnabled:true, items:[]}`.
- Boot smoke on the from-scratch image: project running in ~6 s (SLA);
  dotfiles + harness seeded from the image; `git init -b main` (unborn
  HEAD, as designed); `STYRMAN_CRED_BROKER` injected; `/env` answers `{}`;
  credential snippet present and the MOTD sorts AFTER it (audit fix
  visible); DinD overlay2; claude + agy on PATH. Then deleted (wipe) and
  the metadata traces scrubbed + server restarted → byte-virgin state:
  `/var/lib/styrman` holds exactly the two empty roots.
- https://crew.styrman.xyz → 302 to the Google auth portal (fresh certs,
  fresh session required).

## Included fix
`credentialsList` returns `"items": []` (never `null`) when the store is
empty — cosmetic nil-slice JSON spotted during the fresh-state checks.

## Operator's first-login walkthrough (what you should see)
1. https://crew.styrman.xyz → Google sign-in (old session invalid).
2. Empty dashboard + "First-time setup" banner → Open setup: pick your
   terminal username, the setup VM boots, `styrman-setup` walks git
   identity → ssh key → gh → claude (+ agy per project).
3. Create a project → sign in to your tools in its terminal →
   Settings → Credentials → **Import from this project** → every later
   project is signed in automatically (git/gh/npm/claude/gcloud/ssh; agy
   stays per-project — upstream).
