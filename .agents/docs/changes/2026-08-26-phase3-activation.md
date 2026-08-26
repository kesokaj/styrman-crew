# 2026-08-26 — Phase 3 activated: https://app.styrman.xyz live

Operator provided domain (styrman.xyz), Google OAuth client, and Cloudflare
token (verified against the API before use; saved in deploy/.env, mode 600,
gitignored — operator plans a private repo for it). Stack up via compose:
ingress detected public IP 151.177.14.99, auto-created DNS records (auth,
shell, *.styrman.xyz), obtained Let's Encrypt certs for app+auth via DNS-01,
OAuth kickoff verified 302→accounts.google.com with the registered callback.
Router already forwards 443 (devbox-era) and hairpins — site reachable from
LAN and internet. Unauthed dev server on :8080 stopped; production
(containerized, STYRMAN_AUTH=header, allowlist=operator) is the only front
door now. Remaining: operator's interactive Google sign-in.
