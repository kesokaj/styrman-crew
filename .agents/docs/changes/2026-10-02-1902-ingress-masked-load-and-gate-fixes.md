---
owner: main
date: 2026-10-02
---
# The ingress took a failed load for a success; what the release gate's reviews found   (2026-10-02)

Phase 3 of `plans/2026-10-02-tool-updates-freeze-harness-v2.md`, before the tag.

## Crew was down, and why (BUG-087)

Checking release prerequisites, the agent found nothing listening on 80/443 on
the NUC: crew.styrman.xyz had been down since the host booted at 12:40. At boot
the network was not up, so caddy-security could not fetch Google's OIDC
metadata and Caddy's config load failed — the same boot race as 2026-09-01.
That time the failing `/load` answered 400 and the fix was to retry on a
non-200. This time it answered 200: Caddy adapts the Caddyfile first and, when
the adapter has warnings, writes them to the response BEFORE it loads
(`caddyconfig/load.go`, `handleLoad`, caddy v2.11.4), so a load that then fails
can only say so in the body. The watcher read the status alone, logged "caddy
reloaded successfully", recorded the Caddyfile as applied, and skipped every
later reload as "unchanged".

`reloadCaddy` (`deploy/caddy/ingress/watcher.go`) now reads the body and treats
caddy's `{"error": …}` object as a failed load whatever the status; the
existing 15 s retry does the rest.

The operator was asked, and chose to have the ingress container restarted
("Yes, restart it now (Recommended)") — crew answered again at 18:44 — and to
have the fix in this release ("Fix it in this release (Recommended)").

## What the gate's reviews found, and what was done
- **Security, low** (`reviews/2026-10-02-security-v1.39.0-4dd51bf.md`): keys
  typed while a terminal pane was reconnecting stayed queued after the pane
  gave up, and a later wake event sent them into the shell. A pane that had
  given up now drops the queue when it comes back
  (`web/src/components/TerminalPane.tsx`); the browser suite stages exactly
  that and failed before the fix ("the line typed before the pane gave up was
  replayed into the shell").
- **Code, medium** (`reviews/2026-10-02-review-v1.39.0-4dd51bf-code.md`): the
  README said v1.39.0 was deployed on the NUC. It was not (BUG-084); and its
  tag was signed with a second key that only the GCP VM is known to list. The
  README, `deploy/gcp.md` and `specs/styrman/versioning.md` now say so.
- **Code, low**: the docs called `CLOUDFLARE_API_TOKEN` optional without the
  cost — no certificate for the wildcard preview and private-link hostnames.
  Stated in each place, and in `host-setup.sh`'s closing instructions, which
  still listed the token as a value to fill.
- **Docs, four low** (`reviews/2026-10-02-review-v1.39.0-4dd51bf-docs.md`):
  "six .env keys" in `deploy/gcp/install.sh`, "27" skills in a hand-run
  smoke, the token line in `host-setup.sh`, and the agy check's new clause
  missing from the testing README.
- **Left**: the Vertex AI section of `deploy/gcp.md` may omit the Application
  Default Credentials step (low, unconfirmed — the operator's own procedure);
  `js-yaml` in the web build chain (outside the range, build-time only).

## Files
- `deploy/caddy/ingress/watcher.go`, `deploy/caddy/ingress/watcher_masked_load_test.go` (new)
- `web/src/components/TerminalPane.tsx`, `web/e2e/run.mjs`, `web/dist/client` (rebuilt)
- `deploy/host-setup.sh`, `deploy/gcp/install.sh` (text)
- docs: `README.md`, `AGENTS.md` (Deploy), `deploy/gcp.md`, `deploy/remote.md`, `specs/styrman/deploy.md`, `specs/styrman/guide.md`, `specs/styrman/versioning.md`, `specs/styrman/terminal-protocol.md`, `testing/styrman/README.md`
- records: BUG-087; the three gate reviews; the release verifier's log for `4dd51bf`; the hand-run log

## Agents dispatched
- `verifier` over `v1.39.0..4dd51bf`: `testing/2026-10-02-verify-v1.40.0-release.md`, PASS (before the changes in this entry; it is rerun for the release).
- `security`, `reviewer` (opus) and `reviewer` (docs lens) over `v1.39.0..4dd51bf`: the three reports above.
- `documentation` twice, `bugs` (BUG-087).

## Verification
Red before green:
- `TestReloadCaddyFailedLoadBehindA200IsAnError` against the unfixed
  `reloadCaddy`: FAIL, "a load caddy reports as failed in the body must be an
  error, whatever the status". With the fix the ingress module's tests pass.
- On the REAL caddy build, with no network: the 1.38.0 ingress image logged
  `caddy reloaded successfully`, then `caddyfile unchanged, skipping reload`,
  and its live config was admin-only — the outage. An image built from the
  fixed ingress logged `initial scan failed (retry scheduled)` with caddy's own
  message, retried every 15 s, and 8 s after a network was attached logged
  `load complete` and served its `:443` server.
- The browser suite with the new check against the page before its fix:
  `passed=96 failed=1 skipped=2`. After: `passed=97 failed=0 skipped=2`.

Not covered: a real host reboot with the fixed ingress (this host does not run
it yet); the ingress's behaviour if caddy changes what it writes on a failed
load.

## Learnings for the local host
- The ingress is shared path: the fix reaches both hosts with the release. This
  host reaches it only after the updater's token is replaced (BUG-084).
- Two things on this host failed silently for hours on the same day — the
  updater, and the ingress. Nothing watches the site from outside. That is a
  question for the operator, in the handoff.
- Fresh-host path unexercised: `host-setup.sh` changed only in the text it
  prints at the end.
