# 2026-08-26 — Loopback dev door + full production verification

Operator request: a way to test without Google OAuth. Added STYRMAN_DEV_ADDR
— a second listener with auth forced to "none", published by compose ONLY on
the host's loopback (127.0.0.1:8080 → container :8081). Reachable solely
from processes on dave; LAN and internet still go through the OAuth ingress.
Verified: bound to 127.0.0.1 only, unreachable via the LAN IP.

Full battery, all green:
- unit: 3 packages ok
- integration: 23/23 (isolated state dir, safe next to production)
- Playwright e2e: 18/18 — now running against the PRODUCTION container
  through the dev door (previously needed a separate dev server)
- production (after operator forwarded 80+443): port 80 → 301 https;
  unauthenticated app + API → 302 to the Google portal; portal login 200;
  certificates valid (Let's Encrypt, DNS-01)

Remaining untested: the interactive Google sign-in (operator's click).
