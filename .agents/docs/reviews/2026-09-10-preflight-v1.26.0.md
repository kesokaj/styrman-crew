---
owner: main
date: 2026-09-10
---
# Preflight — v1.26.0   (2026-09-10)

Range reviewed: v1.25.0..5df2959 (release commit with these artifacts follows)

Version: **v1.26.0**, MINOR — adds behaviour for a user (the settings key row,
re-PUT as the recovery for a drifted network mode), breaks nothing. No file in
the repo tracks the version; the signed tag is the bump (versioning.md).

## Findings, worst first

No open blocking finding. Every finding raised today was fixed in the gated
range or rebutted in writing in its report; the defects found by this gate
are listed below. **Two rows were still in flight when the operator
instructed the tag** (asked three times; the prior rounds of both had
converged to comment wording and test placement): the reviewer's round 3
over `21ba6d9` and the security round 3 over `a8ebbbb..5df2959`. Their
results are appended to this report when they land. If either lands a
blocking finding, the remedy is a NEWER tag — the updater never retries a
failed one and a released tag is rolled forward, not amended.

- **fdef866 was pushed with one red vitest case** (BUG-033): a `grep` in the
  check chain swallowed vitest's exit status. Fixed in `5df2959`
  (138/138); never tagged. Recorded, not hidden.

## Rows

| Row | Result | Artifact |
|---|---|---|
| Verification of the COMMIT | PASS — `deploy/verify-commit.sh 21ba6d9`: "safe to tag"; four rounds (29e05a1, a875624, 21ba6d9, 5df2959). go test -race all pkgs, ingress module, gofmt, web/dist freshness, check/lint/vitest 137, server image build | `testing/2026-09-10-verify-v1.26.0.md` |
| Suites on real hardware | PASS — integration 84/0/3 ×3 on Kata (main thread); e2e 86/0/2 exit 0 (main thread, `.agents/scratch/e2e-2026-09-10.log`) | cited in `changes/2026-09-10-network-mode-put-locked.md`, `-ssh-pubkey-in-settings.md` |
| Reviewer — network-mode PUT (42716a8) | APPROVE after 3 rounds on the working tree, delta since round 3 covered by the release-delta review | `reviews/2026-09-10-review-network-record-first{,-round2,-round3}.md` |
| Reviewer — release delta (opus) | Round 1 APPROVE WITH NITS (both fixed); round 2 REQUEST CHANGES — `syscall.Openat` broke the macOS build — fixed in `21ba6d9`; round 3 over `21ba6d9` IN FLIGHT at tag time. The two commits after it (`fdef866`, `5df2959`) change comments, add two tests and a two-line control-character check, self-read by main | `reviews/2026-09-10-review-v1.26.0-delta.md` |
| Security — network-mode PUT | FINDINGS→closed over 3 rounds + narrow round 4; residual a strict subset of v1.25.0's; every interleaving closed | `reviews/2026-09-10-security-network-record-first{,-round2,-round3}.md` |
| Security — ssh public key in Settings | Round 1 FINDINGS (2 medium: the symlink guard did not exist; the regex admitted a newline) — fixed `a8ebbbb`; round 2 FINDINGS (a hard link or a copy defeats any open flag; the claim was false) — the content check made the guard and the comments rewritten, `21ba6d9`/`fdef866`; round 3 IN FLIGHT at tag time | `reviews/2026-09-10-security-ssh-pubkey-settings.md` |
| Trail freshness | PASS — newest `changes/` entries describe this diff; no future dates; `changes/` and `DECISIONS.md` append-only since v1.25.0 (`git log --diff-filter=M` empty); ROADMAP/TODO touched in 7 commits | — |
| Docs freshness | PASS — `specs/styrman/api.md` (network rows rewritten: they still described ADR-065/066, a pre-existing gap), `guide.md` (drift repair, settings key row), `deploy/remote.md` (sshd decision). README unchanged: it does not describe the PUT's mechanics | — |
| Secrets scan | PASS — staged diffs scanned for key/token patterns on every commit today; the one hit is the test fixture string `BEGIN OPENSSH PRIVATE KEY` in `files_test.go` (a literal used to prove the symlinked .pub is refused), no real material | — |

## Defects found by this gate and fixed before the tag

- The server's "refuse a symlinked `.pub`" guard did not exist (`IsRegular`/`ReadRaw` follow confined links); `files.ReadRawNoFollow` (`a8ebbbb`). Pre-existing since the setup page; the caller's own key only.
- The settings regex admitted a newline (`a8ebbbb`).
- The mid-PUT 404 branch had no discriminating test (`75171f0`, `a875624`).

## What was not checked

- A fresh Google sign-in after the bump (the deploy step an agent cannot do).
- The credential-broker reachability test from a LAN project on the compose deployment — still owed (`reviews/2026-09-09-security-adr-067.md`).
- `internal/orchestrator`'s new code (`WithProject`, `lockedProject`, `stopMarked`, `currentBridge`, the locked mode read) has no orchestrator-package unit test: no fake Docker client (BUG-003). Covered by the httpapi fake and the integration suite.
- `govulncheck`: not re-run today (no dependency change); v1.25.0's two accepted `docker/docker` advisories stand.
- Eight dependency-divergence WARNs between the root and ingress modules (TODO Inbox item) — warnings, unchanged.

**VERDICT: SHIP** — on the gated commit `5df2959` ("safe to tag" in
isolation, four verifier rounds, suites green on real hardware) and the
operator's instruction to tag with the two narrow re-check rows outstanding.

## Pre-gate commits on the branch

`42716a8`, `d31ab63`, `cebad2b`, `29e05a1`, `a8ebbbb`, `75171f0`, `a875624`, `21ba6d9`, `fdef866`, `5df2959` (all pushed to `origin/main` before the tag).
