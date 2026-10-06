---
owner: reviewer
date: 2026-10-06
---
# Review — harness-sha256-pin   (2026-10-06)

Range reviewed: `git diff HEAD` in /home/kesokaj/workspace/styrman-crew (7 files, uncommitted): image/Dockerfile, image/README.md, README.md, ADR-090 in DECISIONS.md, specs/styrman/deploy.md, specs/styrman/runtime-image.md, testing/styrman/README.md

## Status: COMPLETE

## Verdict: REQUEST CHANGES

The Dockerfile RUN step is correct. I downloaded the real v2.4.1 release and ran the checks by hand. The pinned hash matches the binary, `SHA256SUMS` has the header and the exact line, `sh-agent version` prints `sh-agent v2.4.1`, and `sh-agent list` shows 36 skills, 8 subagents and 1 rule (`critical-thinking.md`, installed under `~/.claude/rules` and `~/.gemini/config/rules`). What is broken is a restatement left behind. REQUIREMENTS.md R5 still says the harness is "downloaded ... its signed checksums verified against the harness's signer". The image no longer does that, and the harness cannot supply a signature (v2.4.1 ships no `.sig`). That is a requirement the change contradicts, and REQUIREMENTS.md is immutable without the operator's approval. The diff includes no R5 edit and no record that one was approved. Three other living files still say "signer" as current.

## Findings
- **[high / confidence: high]** `REQUIREMENTS.md:55` — R5 says the harness is verified by "its signed checksums ... against the harness's signer". ADR-090 and the Dockerfile now verify a SHA-256 pinned in the repo and drop the signer check. The requirement is contradicted and untouched. A change to REQUIREMENTS.md needs the proposed exact edit, the operator's approval, an ADR, then a REQUIREMENTS.md and ROADMAP.md update in one commit. ADR-090 records approval of the pin choice only, not of an R5 rewording. Fix: propose the R5 text (for example "its SHA-256 pinned in the Dockerfile and checked against the release's SHA256SUMS") and get approval before committing. Otherwise R5 stays in force and is breached.
- **[medium / confidence: high]** `image/Dockerfile:6` — the file header still reads "verified against the harness's signer (ADR-074)". This is the same file whose RUN step dropped the signer. Fix: "verified against the SHA-256 pinned in HARNESS_SHA256 (ADR-074, ADR-090)".
- **[medium / confidence: high]** `.agents/docs/testing/styrman/README.md:416` — Tier 5 intro says the binary is "verified against the harness's signer". The diff edited the guard-control list further down in the same section but not this sentence. Fix as above.
- **[low / confidence: high]** `image/README.md:47` — "Bumping the harness a release ships is editing the `ARG HARNESS_VERSION=vX.Y.Z` line". The diff rewrote lines 4-11 of the same file, but this sentence is now false. A bump with only the version edited fails the build at the pin. The same file still says to edit one line, while deploy.md and ADR-090 say both ARGs. Fix: "edit BOTH `ARG HARNESS_VERSION` and `ARG HARNESS_SHA256`". `.agents/docs/specs/styrman/versioning.md:102` names only HARNESS_VERSION as the pin. It is not wrong, but the pair is worth naming there.
- **[low / confidence: high]** `image/Dockerfile:182-183`, `.agents/docs/specs/styrman/deploy.md:378`, `DECISIONS.md` ADR-090 Context — "unsigned since harness v2.4.0" / "stopped signing releases in v2.4.0" is inexact. The v2.4.0 release still ships `SHA256SUMS.sig` (checked with `gh release view v2.4.0`). Releases are unsigned from v2.4.1 on, as ADR-090's own Context says a few words later. Fix: "from v2.4.1 (decided in harness ADR-053, v2.4.0)".

## Dockerfile RUN step (checked)
- `set -eu;` opens the script and every step is `;`-separated, so any failing command aborts the RUN.
- `grep -qxF` header check: a missing line fails. Both ARGs are build args, so an empty `HARNESS_SHA256` makes the grep fail (the line would be `  sh-agent-linux-amd64`), and `sha256sum -c` rejects it too.
- `echo "<hash>  sh-agent-linux-amd64" | sha256sum -c -` is a pipeline, but its status is that of the last command, `sha256sum -c`, which is the real check. It runs in `$d`, where the binary was downloaded. The `SHA256SUMS` grep is a redundant consistency check, not the trust anchor. The pin is independent of anything downloaded.
- The `version` test uses `test ... =`, so it can fail the build.
- `cd /; rm -r "$d"` removes the temp dir. Only `/styrman/bin/sh-agent` stays. The token is read from a secret mount, not a layer. `ssh-keygen` is no longer used by this step; `openssh-client` stays installed for other uses.
- I did not run a must-fail docker build (wrong `HARNESS_SHA256` against the real release). The checks above were run by hand outside Docker. The new testing README control 2 describes that must-fail build, but no log of it was read here.

## Restatement grep
Searched README.md, AGENTS.md, image/, deploy/, `.agents/docs/{specs,deploy,architecture}`, testing/styrman/README.md for HARNESS_SIGNER, SHA256SUMS.sig, ssh-keygen, allowed_signers, "harness ... signer", and v2.3.2. No live restatement of HARNESS_SIGNER or `ssh-keygen -Y verify` for the harness is left outside the findings above. Remaining hits are deliberate or unrelated:
- The deploy.md mention of the removed check is the ADR-090 pointer text.
- `allowed_signers`, `ssh-keygen` in host-setup, styrman-ctl, install.sh and the updater concern crew release-tag signing, not the harness.
- v2.3.2 hits: `runtime-image.md:267` ("v2.3.2's install wrote 1,788 files") and `:311` and `testing/styrman/README.md:460` are dated measurements, kept as history; `deploy.md:377` is the "until 2026-10-06" note.
- `README.md:140` names "agent harness v2.3.2" inside a past release's bullet in the version history. It is history, not a current claim.
- DECISIONS.md 2735/2773 are older ADRs, append-only.
- `deploy/verify-commit.sh:235-241` still checks only the version, which is enough because the build itself fails on a hash mismatch.

## Criteria
1. Dead code — pass: `HARNESS_SIGNER`, the `.sig` download and the `allowed_signers` scratch file are removed with no leftover reference.
2. Flow tracing — pass for the build. `verify-commit.sh` reads only `HARNESS_VERSION`, and `bump-pins.sh` does not touch the harness pair, as ADR-090 says. The hash and the version drift only if edited separately, and the build catches that. The "1 rule" and 36/8 counts in README.md, image/README.md, runtime-image.md and testing README match `sh-agent list` at v2.4.1.
3. Standards conformance — fail: the R5 contradiction and the stale restatements above.
4. Directory layout — pass: no new files in the diff. The untracked `.agents/docs/testing/2026-10-06-verify-harness-v2.4.1.md` is a dated record and was not reviewed.
5. Simplicity (DIY-except-security) — pass: a stdlib `sha256sum` against a pinned constant, nothing hand-rolled. It replaces a hand-assembled `ssh-keygen` verify flow and is simpler.
6. Decoupling — pass: the pin lives in one place (the two ARGs); no new cross-file coupling.

## Not examined
- A docker build of the changed image, and a must-fail build with a wrong hash. Only the individual commands were run against the real v2.4.1 assets, in `.agents/scratch/`.
- The untracked verification log `.agents/docs/testing/2026-10-06-verify-harness-v2.4.1.md`.
- ADR-090's claim that v2.3.2 cannot `sh-agent update` to an unsigned release, which I did not test.
- Dated history records, out of scope per the dispatch.

## Outcome
- R5 (high) — fixed: `REQUIREMENTS.md` R5 now says the binary is verified against the SHA-256 pinned in `HARNESS_SHA256`; the operator approved the exact text ("Approve", 2026-10-06), recorded in ADR-090.
- `image/Dockerfile:6` (medium) — fixed: "verified against its pinned SHA-256 (ADR-074, ADR-090)".
- `testing/styrman/README.md:416` (medium) — fixed: "verified against the SHA-256 pinned in `ARG HARNESS_SHA256` (ADR-090)".
- `image/README.md:47` (low) — fixed: a bump edits both ARGs, the hash from `SHA256SUMS`.
- "unsigned since v2.4.0" (low) — fixed: the Dockerfile and deploy.md say "unsigned from v2.4.1"; ADR-090's Context says v2.4.0 carried one last `SHA256SUMS.sig`.
