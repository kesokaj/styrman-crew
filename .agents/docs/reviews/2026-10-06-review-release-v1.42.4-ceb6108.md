---
owner: reviewer
date: 2026-10-06
---
# Review — release-v1.42.4-ceb6108   (2026-10-06)

Range reviewed: v1.42.4..ceb6108 (`git diff v1.42.4..ceb6108`, 7 commits, 26 files)

## Status: COMPLETE

## Verdict: APPROVE

Nothing in the range is broken. The one real code change is `image/Dockerfile`'s
harness step (ADR-090): it drops the `ssh-keygen -Y verify` signature check against
`HARNESS_SIGNER` and pins the `sh-agent-linux-amd64` SHA-256 in `ARG HARNESS_SHA256`.
That step fails closed on every path I traced. It fails on an empty or non-hex pin,
on a `SHA256SUMS` header for a different version, on a `SHA256SUMS` that does not list
the pinned hash, on bytes that do not match the pin, and on a binary that reports
another version. The amended R5 in `REQUIREMENTS.md` describes exactly this mechanism.
The old harness signer (the exposed `5HpY…` key, BUG-094) was pinned in the v1.42.4
Dockerfile, and this range removes it. No source, config or test in the tree still
names that key. The `web/` changes touch comments only, and they are accurate:
`budgetGuard` exists at `internal/httpapi/budget.go:217` and caps a request's body by
the route it matched. Everything else in the range is docs or trail.

## Findings

None that meet the bar (correctness, security, stated requirement or project rule).

Checked and found sound:
- `image/Dockerfile:200` (`set -eu; case … *[!0-9a-f]*) exit 1;; esac; [ "${#HARNESS_SHA256}" -eq 64 ]`).
  An empty pin passes the `case` but fails the length test, so `set -e` stops the build.
  An uppercase or short pin also fails. Even without this guard, `grep -qxF` with an
  empty hash could not match a line. The guard costs nothing and is not dead code.
- `image/Dockerfile:204-206`. The header check, `grep -qxF "<pin>  sh-agent-linux-amd64"`
  and `echo … | sha256sum -c -` all run bare under `set -eu`. The one pipeline exits
  with `sha256sum`'s status, which is the status that matters.
- `--repo github.com/kesokaj/styrman-harness` is a valid `gh` `HOST/OWNER/REPO` form.
  The Tier 2 rebuild in `testing/2026-10-06-verify-harness-v2.4.1.md` built with it
  (`sh-agent v2.4.1`).
- `image/bump-pins.sh:111` anchors its `sed` on exact ARG names (`GO_SHA256`,
  `NODE_SHA256`, …), so it cannot rewrite `HARNESS_SHA256`. This matches ADR-090's
  "bump-pins does not refresh the harness pair".
- No build site passes `--build-arg HARNESS_SIGNER`, and no test or script names
  `SHA256SUMS.sig`. I grepped the whole tree except the trail; only the closed 2026-09-21
  plan still mentions them, as history.
- R5 against v2.4.1's actual install, run by hand in `styrman-runtime:test`:
  - `~/.claude/settings.json` holds only `attribution`.
  - No hooks are installed.
  - `critical-thinking.md` lands in `~/.claude/rules/` and `~/.gemini/config/rules/`,
    as `runtime-image.md:265-267` says.
  - The 22 `*.sh` files under skills (e.g. `review/scripts/restatement-grep.sh`) were
    already in the v2.3.2 images (1.42.1–1.42.4: 22 files, same count). They are not
    new in this range. Whether skill-bundled scripts are compatible with R5's "no
    scripts" is an existing question outside this range's scope.
- Restatements of the old check are all updated: README.md:16, image/README.md:7-10 and
  :46-49, deploy.md:377-389, runtime-image.md:264, testing README:416 and :474-477.
  README.md:140 still says v2.3.2, but it is the v1.40.0 release-history line, which is
  correct.
- Tier 2 asserts skill and subagent counts dynamically (`integration.sh:161`). The
  rule-file roster is not asserted by any suite. The docs do not claim it is.

## Criteria
1. **Dead code: pass.** The `HARNESS_SIGNER` ARG, the `allowed_signers` scratch file and the `.sig` download were removed together. Nothing is orphaned.
2. **Flow tracing: pass.** Every build site (updater, host-setup, styrman-ctl, ensure-test-image) passes only the `gh_token` secret. The new ARG has a default, so none of those sites needs a change. The entrypoint, migrate and seed path is unchanged and passed Tier 2 (146/0/3 per the verify log). The must-fail control (zero hash) is recorded as rc=1 at the harness RUN.
3. **Standards conformance: pass.** R5 was amended with recorded operator approval, ADR-090 was added, and every restatement of the check was found and updated. The `web/` comments match the code. No binary diffs (`git diff --numstat` shows no `-` rows).
4. **Directory layout: pass.** No new source files. The new docs are in the trail directories.
5. **Simplicity (DIY-except-security): pass.** It uses `sha256sum` as a checksum, which is stdlib and not hand-rolled crypto. Replacing a signature check with a hash pin in a signed crew commit is simpler, and ADR-090 records the trust trade-off for in-project `sh-agent update`.
6. **Decoupling: pass.** The harness pin lives only in `image/Dockerfile` and is independent of `bump-pins.sh`'s pins.

## Not examined
- I did not re-run the reproducible-build provenance claim in the Dockerfile comment and ADR-090 (commit 7ea9068, go1.27.1, bit for bit).
- I did not re-run the must-fail controls for the Dockerfile; I read them from the verify log only.
- I read the trail markdown (changes/, handoffs/, reviews/, bugs/, testing/20*) only where it makes claims about code, per the dispatch.
- The operator-side state of `allowed_signers` on dave and the GCP VM is outside the repository; BUG-094 is still open.
