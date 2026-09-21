---
owner: reviewer
date: 2026-09-21
---
# Review — e5bd830 docs lens   (2026-09-21)

Range reviewed: e5bd830~1..e5bd830 (docs-lens only, per dispatch; no code review). Diffed
files: `deploy/gcp/vm.sh`, `deploy/gcp/install.sh`, `deploy/gcp/lib.sh`,
`deploy/gcp/vm.conf`, `deploy/styrman-ctl` (`cmd_install`'s `GH_TOKEN` export),
`deploy/host-setup.sh` (the Kata step), `deploy/verify-commit.sh` (lines
168-171), against `.agents/docs/deploy/gcp.md`, `deploy/README.md`,
`README.md`, `AGENTS.md`, `.agents/docs/specs/styrman/deploy.md`,
`.agents/docs/deploy/remote.md`, `.agents/docs/testing/styrman/README.md`,
`.agents/docs/testing/guide.md`, plus `.agents/docs/architecture/DECISIONS.md`
(ADR-075) for cross-checking claims.

## Status: COMPLETE

## Verdict: APPROVE WITH NITS

This is an unusually well-covered docs pass: every new command, flag,
default, exit code and refusal message in the six code files traces to
`gcp.md`, `deploy/README.md`'s new GCP section, `specs/styrman/deploy.md`'s
Kata/GH_TOKEN paragraphs, and the new `testing/guide.md`; the
`GCP_TAGS=http,https` / `.tar.zst` / GitHub-asset-digest / `--gh-token-file`
changes are reflected with no stale `.tar.xz`, `sha256sum`-sidecar,
`http,https,ssh`, or "Not applicable" wording left standing in any living
doc. The only real gap is `AGENTS.md`'s "Setup Commands" section, which still
has no line for the two new GCP scripts even though its own Deploy section
now names them — a reader who follows only "Setup Commands" (as the section
header invites) will not find `deploy/gcp/vm.sh` / `install.sh`. One smaller
completeness nit: the third restatement of the "GH_TOKEN on a sudo-logged
command line" form, in `specs/styrman/deploy.md`'s "Rebuilding a host"
paragraph, did not get the "prefer `--gh-token-file`" caveat that the same
commit added to the other two restatements of the identical line.

## Findings

- **[low / confidence: high]** `AGENTS.md` "Setup Commands" section (around
  line 50, after the `deploy/verify-commit.sh` line) — the section lists
  local build/run/compose commands but no line for `deploy/gcp/vm.sh` /
  `deploy/gcp/install.sh`, even though this same commit updated the "Deploy"
  section further down to say "Three targets exist: local … the production
  host … and a disposable GCP Compute Engine VM … (`deploy/gcp/vm.sh`,
  `deploy/gcp/install.sh`, ADR-075)". A reader who stops at "Setup Commands"
  (the section this range's own dispatch checklist calls out) has no pointer
  to the GCP scripts or to `gcp.md`. Fix: add one line under "Run locally …
  / deploy production", e.g. `# Deploy to GCP: deploy/gcp/vm.sh create &&
  deploy/gcp/install.sh — full procedure: .agents/docs/deploy/gcp.md`.

- **[low / confidence: medium]** `.agents/docs/specs/styrman/deploy.md:286`
  ("Rebuilding a host" bullet) — still reads `sudo GH_TOKEN=$(gh auth token)
  bash deploy/host-setup.sh --user $USER` with no caveat, while this same
  commit added "This form works but puts the token on a sudo-logged command
  line; prefer `sudo deploy/styrman-ctl install --gh-token-file <600 file>`
  …" to the *other two* restatements of the identical line — `remote.md:68-76`
  and `specs/styrman/deploy.md:200-209` (the "Bring-up" section, ~30 lines
  above this one in the same file). This is the third live occurrence of the
  same command and it wasn't updated alongside its siblings. Not a
  correctness bug (the bare form still works), but an inconsistent
  restatement of a security-relevant preference the commit otherwise applied
  uniformly. Fix: mirror the same one-sentence caveat onto line 286, or
  replace the line with a pointer to the "Bring-up" section above it.

## Not examined

- No code review performed (out of lens, per dispatch).
- `.agents/docs/bugs/BUG-069/070/071*.md`, the plan file, cold-read files,
  and the three verifier/security/review artifacts under `.agents/docs/{plans,
  reviews,security,testing}/2026-09-21-*` were not read in detail — the
  dispatch excludes `changes/`, `reviews/`, `testing/2026-*`, `bugs/`,
  `handoffs/` and the plan from the staleness grep, and this review only
  spot-checked `DECISIONS.md`'s ADR-075 text against the code for
  cross-consistency, not the full bug/plan/verify trail.
- `internal/config/gcp_scripts_test.go` and `styrman_ctl_test.go` were only
  checked for `func Test…` names against the README's roster (matched
  exactly, 11 + 1); test bodies/assertions were not read line-by-line —
  out of the docs lens.
- `.agents/docs/testing/styrman/README.md`'s "Tier 1" unit-test count line
  update was not independently re-derived from the full go test suite; only
  the new gcp_scripts/styrman_ctl entries were checked against the source.

## Outcome

1. `AGENTS.md` Setup Commands: **fixed** — a GCP block (`vm.sh create`, `install.sh`, the guide) before "Test suites" (main agent; rules for `AGENTS.md` findings).
2. `specs/styrman/deploy.md` "Rebuilding a host": **fixed** — names `styrman-ctl install --gh-token-file` first, keeps the bare form with the sudo-log caveat, and adds the GCP host's two commands.
