---
owner: main
date: 2026-09-08
---

# Whole-project audit   (2026-09-08)

Report: `.agents/docs/reviews/2026-09-08-audit-project.md`. Four agents in
parallel — `explorer`, `reviewer` (opus), `security` (opus), `verifier` —
over the whole tree at `414be72` (v1.24.0 live), auditing the code as it
stands rather than a diff.

## Result

**No high findings, and no path was found that escapes a project to another
tenant or to the host.** The isolation core was probed against its own
regression history (BUG-002/003/004) and none regressed. 11 medium, 11 low,
3 informational. Every suite green: Go 17/17 packages, ingress module, web
85/85, firewall 125/125, integration 79 pass / 0 fail / 4 skip.

## The shape worth remembering

Three of the four security mediums are the same thing: **a guard that is
correct where it was written and absent where it was not.** Not new attacks —
existing, documented defences that stop at a boundary nobody redrew.

The clearest is `internal/files`' `O_NONBLOCK` discipline, which exists
precisely so "a guest-planted FIFO/device returns immediately instead of
wedging the goroutine forever (a cross-tenant DoS)" — its own comment. Six
sibling opens on the same guest-writable trees use plain `rt.Open` and do not
have it; `history.searchOne`'s `IsRegular` check sits *after* the blocking
open, so it never runs. Verified first-hand: `files.go:129,285` set the flag,
`history.go:48,144,215` do not. Impact is genuinely cross-tenant — each
blocked request pins a goroutine and an OS thread, and the runtime's
10k-thread limit takes every tenant down with the process.

The same shape twice more: `styrman-update` asserts `.git` and `deploy/.env`
ownership but never the worktree root it then builds as root, despite its own
comment naming exactly that threat; and the ingress writes label-supplied
emails verbatim into a generated Caddyfile while `sanitizeHostname` strips
Caddyfile-significant bytes from the hostname for the same reason.

This is the same pattern the ADR-066 release gate hit sixteen times, found
now in code that predates it — which suggests it is a property of the
codebase, not of that feature's development.

## Two things fixed in this commit

- **`README.md` integration count was wrong**: it claimed 82 checks; two
  independent counts of the same run give 83. Introduced today — the ADR-066
  round-1 fixes added one check and only the firewall count was updated. The
  entry now also documents the stale-image trap that cost real diagnosis time
  today: the default `styrman-runtime:test` fails three GOPATH checks for
  image reasons, not code reasons.
- **`specs/styrman/api.md` was a release behind** — the six ADR-066 response
  fields shipped in v1.24.0 appear zero times, while `web/src/lib/api.ts`
  already types them. Dispatched to `documentation`.

## Largest untested surface

`deploy/update/styrman-update` — 695 lines of root-run, self-replacing
deploy and rollback code with **zero automated tests**, where its sibling
`deploy/firewall/test.sh` has 125 checks. The security pass read it in full
and found it unusually well hardened (annotated-tag requirement,
SSH-signature-only, last-armor-header defence against a decoy in the
author-controlled tag message, root-owned `allowed_signers` assertion) — which
makes the missing regression harness the risk rather than the code. Nothing
would catch a change that removed one of those defences.

## Agent notes

All four wrote their artifacts, and all four included an explicit "Not
examined" section. Two still hit their turn limits (security at 40, verifier
at 30) and were resumed; the `Write the artifact EARLY` instruction deployed
to the agent definitions an hour earlier meant the resumed runs had something
to build on rather than nothing — security had written a skeleton, the
verifier 12.5KB. That is an improvement over the four bare stalls earlier in
the day, but "write early" was read as "write the headings early", which is
worth sharpening.

Written directly rather than by the `changelog` agent, as with every entry
this session.
