---
owner: bugs
date: 2026-10-02
id: BUG-087
status: fixed
area: deploy/caddy/ingress/watcher.go (reloadCaddy), production host
---
# BUG-087: crew was down for six hours after a host boot: the ingress took a failed Caddy load for a success

## Symptom
The host booted at 12:40 local time on 2026-10-02; until 18:43 `crew.styrman.xyz` was down. `docker ps` showed `styrman-caddy` up and `styrman-server` healthy; `ss -ltn` showed nothing on `:80` or `:443`; `curl -s http://127.0.0.1:2019/config/` printed `{"admin":{"listen":"tcp/127.0.0.1:2019"}}`.
Ingress log: at 10:40:28Z `failed provisioning app server instance … failed to fetch metadata for OAuth 2.0 authorization server`, then `caddy reloaded successfully`, then caddy's `request error … loading config: loading new config: loading http app module: provision http: …` and `http: superfluous response.WriteHeader call`; afterwards, at every container event, `caddyfile unchanged, skipping reload`.
Found by the agent while checking release prerequisites, not by a monitor.

## Repro
Start the ingress with no network reachable (caddy-security cannot fetch Google's OIDC metadata) and a Caddyfile that adapts with warnings: `/load` answers 200 and nothing is served.

## Root cause
Caddy's `/load` handler (`caddyconfig/load.go`, `handleLoad`, caddy v2.11.4) adapts the Caddyfile and, when the adapter returned warnings, writes them to the response BEFORE calling `caddy.Load`, which sends status 200. When the load then fails, the error status can no longer be set ("superfluous response.WriteHeader"); only the body carries the `{"error": …}` object.
`reloadCaddy` checked `resp.StatusCode != http.StatusOK` and nothing else (the fix for the same boot race on 2026-09-01, when the failing load answered 400). It logged success, the Caddyfile was recorded as applied, and every later scan compared equal and skipped the reload. The retry loop that exists for a rejected load never started.
Why it was not caught: the 2026-09-01 fix was tested against a 400 (`TestReloadCaddyRejectedLoadIsAnErrorButNotUnreachable`), the only failure shape seen then; nothing checked that a load reported as successful produced a serving config; nothing watches the site from outside.

## Fix
`deploy/caddy/ingress/watcher.go`: after a 200, `reloadCaddy` reads the body (capped at 1 MiB) and `loadError` returns caddy's message when the body carries its error object. That is returned as a plain error (not `errAdminUnreachable`), so `noteReloadFailure` schedules the 15 s retry and the Caddyfile is not recorded as applied. Warnings alone, an empty body, and anything that is not caddy's error object remain a success. Ships in the release after v1.39.0.
Live host: brought back by restarting the ingress container at the operator's word ("Yes, restart it now (Recommended)", 2026-10-02); it does not yet run the fix.
Tests, `deploy/caddy/ingress/watcher_masked_load_test.go`: `TestReloadCaddyFailedLoadBehindA200IsAnError` (written first; failed against the unfixed code with "a load caddy reports as failed in the body must be an error, whatever the status"), `TestReloadCaddyWarningsAloneAreSuccess`, `TestLoadErrorReadsOnlyCaddysErrorObject` (nine body shapes). The failing body is built from caddy's source (warnings array, then `APIError`, JSON field `error`).

## Prevention
- A success reported by a status code is checked once against what the other side's own source says it writes on failure: a 2xx proves the request was answered, not that the work was done.
- When a mechanism's first fix was written for ONE observed failure shape, its test names the shapes it does not cover.

## Related
The 2026-09-01 outage (same boot race, answered 400 then; `reloadRetryDelay` in `watcher.go`). BUG-084 (the same host's updater failing unnoticed: the second silent multi-hour failure on this host). BUG-034 family (a green signal that measured something else).
