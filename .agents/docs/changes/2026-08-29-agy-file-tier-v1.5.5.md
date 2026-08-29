# agy harvests via the file tier; obsolete setup message fixed (v1.5.5)

Date: 2026-08-29. Closes the two items left open by the v1.5.4 broker-seed
work (`2026-08-29-setup-seeds-broker-v1.5.4.md`) — after which every tool
except agy flowed to new projects automatically. Both are now fixed; agy is
the last piece of the "sign in once, every project stays signed in" promise
(the long-standing "agy real-token check" from ADR-032a).

## The agy discovery

ADR-032a modelled agy as a gnome-keyring (Secret Service) credential and the
broker grew a whole keyring tier for it. That assumption was wrong for this
agy build. Inspecting a real signed-in home showed the OAuth token lives in a
plain file:

- `~/.gemini/antigravity-cli/antigravity-oauth-token` — 498 bytes, mode 0600,
  JSON `{ "token": { access_token, token_type, refresh_token, expiry },
  "auth_method": "consumer" }`. A standard, portable OAuth2 token with a
  refresh_token.
- The `~/.local/share/keyrings/login.keyring` is only 719 bytes — far too
  small to hold that token; `secret-tool search service antigravity` returns
  nothing because agy simply doesn't use the keyring for this.

That also explains why the v1.5.4 harvest reported agy "not found": the
minimal-mode setup VM runs no keyring daemon, but even a running one would
have found nothing — the secret was never in the keyring.

## Fix

- **agy → file tier.** `harvestCredentials` (internal/httpapi/credentials.go)
  now harvests `~/.gemini/antigravity-cli/antigravity-oauth-token` as a 0600
  file credential, exactly like claude — dropping the two-step
  `secret-tool search`/`lookup` keyring path. This works from the minimal
  setup VM (a plain `base64` file read; no Secret Service needed), and the
  refresh_token lets each project renew the token on its own. The pull-on-login
  file tier already `mkdir -p`s nested parents, so `~/.gemini/antigravity-cli/`
  is created in each project home before the token lands — no image change was
  needed for materialization.
- **Setup message.** `image/configs/styrman-setup` no longer tells the operator
  to manually open Settings → Credentials → Import (obsolete since v1.5.4
  auto-seeds the broker on completion). The completion text and header comment
  now say credentials are shared with every project automatically; the manual
  Import is mentioned only as a supplementary re-import path. This is the one
  change that needed a runtime-image rebuild.

The broker's keyring tier (credstore KeyringCred / broker `GET /keyring` / the
login snippet's `secret-tool store` block) is now unused by any tool. It's left
in place, dormant and harmless, rather than ripped out in this change; removal
is a separate cleanup (see TODO).

## Verification (live, styrman-server 1.5.5 + rebuilt styrman-runtime:dev)

Re-ran the setup harvest against the operator's real shared home: report now
reads `agy: stored (oauth token)` and the broker store gained
`file-agy-oauth-token.json` alongside git/gh/ssh/claude. Restarted the "test"
project → login-shell pull materialized
`~/.gemini/antigravity-cli/antigravity-oauth-token` (498 bytes, valid JSON with
a refresh_token) plus ssh/claude/gh. So every tool the operator signed into now
lands in every project automatically, agy included — the ADR-032a open
verification is closed. (gcloud/npm still report "not found" only because the
operator hasn't signed into them.)

## Files changed

- `internal/httpapi/credentials.go` — agy harvest switched keyring → file tier.
- `image/configs/styrman-setup` — completion message + header comment updated
  (runtime-image rebuild).

## Release note

Built + deployed live as styrman-server 1.5.5; styrman-runtime:dev rebuilt with
the message change. Committed together with the v1.5.3/v1.5.4 work in one
change set (prior live builds 1.5.3/1.5.4 were never tagged — this is the first
committed state of the session's work).
