# Forked from devbox — not vendored verbatim

This Caddy ingress (Caddy 2 + caddy-security Google OAuth + Cloudflare DNS +
Docker-label app discovery) was originally imported from
`~/workspace/devbox/caddy` (module `github.com/spgo/devbox/caddy/ingress`)
per R11 — a styrman host must build from this repo alone, no external
checkout at build time. It is now a **fork, not a mirror**: `git log --
deploy/caddy/ingress` carries styrman-specific commits on top of the
import, including the shared ingress token (`X-Styrman-Ingress`), the
public/private preview wildcard domains (ADR-025/026), deterministic
multi-homed IP selection (`extractContainerIP`, v1.8.5 — see `deploy.md`'s
"Caddy ingress upstream selection" section), `sanitizeHostname`/
`sanitizePolicyName` input hardening (`labels.go`/`generator.go`), and the
removal of the unused `picture_extractor` code path and the `caddy-l4`
module/dependency (2026-08-30 cleanup, see `deploy.md`). A 2026-09-01 review
(`.agents/docs/reviews/2026-09-01-audit-reviewer.md`) flagged the previous
"vendored verbatim" claim here as false; this file is the fix.

Because this is a fork, do not blindly re-sync it from upstream the way a
verbatim mirror would with a bulk `rsync --delete` refresh — that would
silently drop every styrman-specific change listed above. Pull an upstream
fix by hand instead: diff the specific file(s) against
`~/workspace/devbox/caddy`, port only the fix, and keep the local changes
intact. Styrman-specific changes belong in this copy and are not meant to
go back upstream.
