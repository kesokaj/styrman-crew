---
owner: reviewer
date: 2026-09-22
---
# Review — phase3-code   (2026-09-22)

Range reviewed: 49eb863..0024de7 (git diff 49eb863...0024de7)

## Status: COMPLETE

## Verdict: APPROVE

This is a clean review-remediation commit closing findings 4, 5, 6 and the
lock half of 9 from `reviews/2026-09-22-review-project-merged.md`. All four
call sites I checked against their originals behave identically for
`install.sh`'s callers, the drift-guard test asserts exact security-load-
bearing text (not a cosmetic marker) and would catch a real divergence, the
`X-Caddy-Role` strip sits before the matchers that set it as the test
claims, and the `credMu` split touches exactly the eight call sites named
and introduces no lock-ordering hazard. `go vet ./...`,
`go test ./...` (internal/config, internal/httpapi), `go test -race
./internal/httpapi/...`, and `(cd deploy/caddy/ingress && go test ./...)`
all pass bare, read as green.

## Findings

None that meet the bar (correctness/security/rule violation). Two
observations, both sub-finding severity, noted for completeness:

- **[none / n/a]** `deploy/gcp/lib.sh:26-31` — the `env_get` literal-match
  awk program and `is_key` guard are duplicated verbatim from
  `deploy/styrman-ctl` rather than sourced, and the diff itself documents why
  (inverted dependency direction) and backs the duplication with
  `TestGCPLibEnvGetMatchesStyrmanCtl`, satisfying the "two copies that
  already drifted once will drift again" rule family (BUG-035/BUG-070). Not
  a finding — the mitigation is exactly the shape those bugs call for.
  Verified: `deploy/styrman-ctl:112-115` carries the identical `is_key`
  regex and awk line the test pins.

## Criteria

1. **Dead code** — pass. The two forked helpers (`env_get`, `private_file`)
   removed from `install.sh` had a single caller each, both migrated to
   `lib.sh`'s copies at the same call sites; no orphaned code left behind.
2. **Flow tracing** — pass, checked in depth:
   - `install.sh`'s three `private_file` call sites (`ENV_FILE`,
     `TOKEN_FILE`, `SIGNERS_FILE`) all use the new function's defaults
     (`self`, `077`), which is exactly what the old hardcoded `self`-owner /
     `&077` check enforced — confirmed against `TestGCPLibPrivateFileDefaultsAndPolicies`,
     which pins both defaults as "what install.sh relied on before the merge".
   - `env_get`'s "last assignment wins" semantics and "empty string on no
     match / missing file" semantics are preserved (old: `grep -E ... |
     tail -n1`; new: awk loop that keeps overwriting `v` without breaking,
     printing only at END). All of `install.sh`'s call-site keys
     (`BASE_DOMAIN`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`,
     `CLOUDFLARE_API_TOKEN`, `ADMIN_EMAIL`, `STYRMAN_ALLOWED_EMAILS`,
     `STYRMAN_HOSTNAME`) satisfy the new `is_key` guard, so no call site is
     silently weakened or broken by the stricter regex.
   - The `X-Caddy-Role` strip (`generator.go:453`) is written before the
     three `header_regexp`/`request_header @...` pairs that conditionally
     set the role, and both are inside the `route {}` block before the
     final `reverse_proxy`, so a request matching none of the three
     matchers now gets the ingress's default (nothing) rather than a
     client-supplied value — matches the stated intent and the new test
     `TestRoleGatedStripsClientSuppliedRole`, which explicitly checks byte
     order.
   - `credMu` split: grepped every remaining `deletingMu` and `credMu` use
     in non-test `.go` files — all eight `credPushing`/`credPushRerun`
     accesses now use `credMu` exclusively, no stray site left on
     `deletingMu`, and no site accesses both locks' state in one critical
     section (`pushCredsToRunningProjects`'s two callers, `credentials.go:217`
     and `setup.go:98`, hold neither lock when calling it) — no nested-lock
     ordering hazard, confirmed by a clean `go test -race
     ./internal/httpapi/...`.
3. **Standards conformance** — pass. Matches `review-standards` and the
   project's shell/Go conventions; `shellcheck -S warning` and `bash -n` are
   claimed run in the change entry (not independently re-run by me, but
   `deploy/gcp/*.sh` shows no obvious violations). Doc-comments in
   `server.go` and `lib.sh` were updated in the same change to describe the
   new lock split and parameterisation — satisfies the describing-text rule.
4. **Directory layout** — pass. Shared GCP-only shell logic lives in
   `deploy/gcp/lib.sh` (already the shared lib for `install.sh`/`vm.sh`); the
   new Go test lives beside its siblings in `internal/config/`; the new
   Caddy test lives in `deploy/caddy/ingress/` beside `generator.go`, inside
   that module's own package, not the root module.
5. **Simplicity / DIY-except-security** — pass. No new dependency. The
   `env_get` awk rewrite avoids interpolating a caller-controlled key into a
   `grep -E`/`sed` program (the documented past incident), which is the
   security-exemption's stdlib-consuming form (`awk -v`), not a hand-rolled
   primitive. `private_file`'s two new parameters (`OWNER_POLICY`,
   `PARENT_MASK`) are the minimal generalisation needed to merge two
   genuinely-different-context checks without silently picking one; both
   defaults preserve `install.sh`'s prior behaviour, so no caller is
   weakened by the parameterisation (checked every call site — only
   `install.sh`'s three, all default args).
6. **Decoupling** — pass. `credMu` now guards only `credPushing`/
   `credPushRerun`, separating an unrelated feature (credential push) from
   `deletingMu`'s project-lifecycle maps — a real decoupling win, and the
   change entry explicitly declines to also split `server.go` itself
   ("no lens has read server.go for cohesion"), which is the right scope
   discipline rather than a missed opportunity.

## Not examined

- `bash -n` / `shellcheck -S warning` over `deploy/gcp/*.sh` — claimed run
  in the change entry, not independently re-run here.
- Tier 2/Tier 4 integration suites and container builds — the change entry
  states these are deferred to "Phase 5"; out of scope for this diff.
- The plan file `.agents/docs/plans/2026-09-22-project-review-remediation.md`
  and the full source review `.agents/docs/reviews/2026-09-22-review-project-merged.md`
  were read only for the four findings this phase claims to close (4, 5, 6,
  9); their other findings were not re-verified against this diff since they
  are out of this phase's stated scope.
