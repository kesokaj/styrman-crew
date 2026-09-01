# Re-review — harness vendor sync v1.15.0 (fixes pass)   (2026-09-01)

Follow-up to `.agents/docs/reviews/2026-09-01-review-harness-vendor-sync-v1.15.0.md`
(verdict REQUEST CHANGES). Narrow scope per dispatch: is each prior finding
genuinely resolved, and did the fixes break anything? Mirror fidelity, dead code,
directory layout and decoupling were passed last time and are re-checked only
where the fixes touched them. `image/harness/**` content remains out of scope
per ADR-046 except `VENDORED.md`.

## Verdict: APPROVE

Nothing is broken. The high finding is genuinely fixed, and I re-measured it
myself rather than accepting the reported numbers: on the shipped image, the
harness deploy against an empty home takes **456 ms** with the trimmed PATH and
**60,900 ms** without it, both `rc=0`, and `diff -r` between the two resulting
homes shows the *harness deploy is identical* — 6/6 subagents and 28/28 skills
under **both** `~/.claude` and `~/.gemini/config`, 69-line `CLAUDE.md`/`GEMINI.md`,
both gates in `~/.agents/bin/` at `0755`, valid-JSON `settings.json`, zero errors
in the log. The PATH trim is safe: `agy` is the only `/usr/local/bin` binary any
of the three setup scripts references, and it sits behind `command -v agy`.
The guest's own login shells are untouched. Two items remain open and both are
release mechanics, not code defects: the verification log on disk still describes
the **pre-fix** image and explicitly claims "zero Go, zero web/ source changes",
which is no longer true now that `image/entrypoint.sh` is in the change set; and
`v1.15.0` is still untagged. Both are mechanically closable before the commit,
and neither undermines the code, which I verified independently.

## Findings

- **[medium / confidence: high]** `.agents/docs/testing/2026-09-01-verify-harness-upgrade-v1.15.0.md` —
  **The verification log predates the fix it is supposed to cover.** It records
  `styrman-runtime:dev` image ID `f8e986b1c571`, built `08:49:52`; the image that
  actually carries the PATH trim is `1a8381a805a1`, built `09:17:15` (confirmed:
  `grep "PATH=/usr/bin" /styrman/entrypoint.sh` hits in `:dev` today). The log
  contains no mention of PATH, boot latency, or the 2.69 s cold create, and its
  "Coverage of changed code" section states the change set is
  `image/harness/** (19 modified, 19 added, 25 deleted) plus docs` — both the
  retracted counts *and* the pre-fix scope — and concludes "No Go/web/deploy
  source changed → the 'changed code without covering test = FAIL' rule has no
  packages to apply to here." `image/entrypoint.sh` is now changed source sitting
  directly on the container boot path, and it has no automated covering test. So
  the verifier DoD row I raised last time is filled for the vendoring half and
  unfilled for the fix half. Fix: append a section to that log covering image
  `1a8381a805a1` — the A/B timing, the identical-deploy diff, and the cold-create
  end-to-end — and correct the file counts and the "zero source changes" claim.
  This must land before the commit, per the constitution's own ordering ("before
  any completion claim or commit").

- **[medium / confidence: high]** repo root — **`v1.15.0` is still untagged**
  (`git describe --tags --always` → `v1.14.2-3-gc57abb2`). Accepted-and-scheduled
  is a reasonable disposition, but I will not record it as verified, because it
  is not: the tag does not exist, and the one thing that matters about it — that
  it lands on the commit that *contains the entrypoint PATH fix*, not on the
  vendoring commit alone — cannot be checked in advance. Treat it as a release
  condition, closable with `git tag v1.15.0 <commit>` and re-checkable in one
  command.
  **Correction to my own prior finding, in the interest of accuracy:** I
  overstated its blast radius. `/api/version` is fed by the compose build-arg
  `STYRMAN_VERSION` supplied by the operator (`deploy/docker-compose.yml:68`),
  not by `git describe`. Only `deploy/host-setup.sh:319` falls back to
  `git describe` for the *runtime image* stamp, and only when the image does not
  already exist. So the "no document matches a rebuild" impact is narrower than I
  wrote — it is a reproducibility and provenance gap, not a version-mismatch bug.
  Still worth doing; downgrade the urgency, not the action.

- **[low / confidence: high]** `.agents/docs/testing/styrman/README.md` (Tier 5) —
  **The new manual procedure has no latency assertion**, which is precisely the
  regression class it was written in response to. The fix depends on an implicit
  invariant — `agy` lives in `/usr/local/bin` and nowhere on
  `/usr/bin:/bin:/usr/sbin:/sbin`. If a future `image/Dockerfile` change or an
  upstream packaging change puts `agy` (or any other blocking self-check) on the
  trimmed PATH, the workaround silently stops working and the 60 s block returns
  with no alarm anywhere: `go test`, `integration.sh` and `web/e2e` are all blind
  to it, and Tier 5 as written would pass. ADR-047 names exactly this staleness
  risk ("an upstream change could shift the assumption the workaround depends
  on") but assigns the re-check to nothing executable. Two lines close it: in
  Tier 5 step 3, assert the deploy completes in under ~10 s, and assert
  `runuser -u <user> -- env PATH=/usr/bin:/bin:/usr/sbin:/sbin command -v agy`
  finds nothing.

- **[low / confidence: high]** `.agents/docs/specs/styrman/runtime-image.md:57-78`
  and `image/README.md` — **The PATH trim is documented in the entrypoint comment
  and ADR-047 only.** Both of these files were rewritten in this same release to
  describe the harness seeding step in detail, and neither mentions that the
  invocation deliberately hides `/usr/local/bin`. The spec is the low-level
  description of exactly this entrypoint step; a reader reconstructing the boot
  path from it would not know the workaround exists. ADR-047 itself flags
  discoverability as the trade-off it accepts — one sentence in spec step 6
  ("…invoked with `/usr/local/bin` trimmed from PATH, ADR-047") pays most of it
  back.

- **[low / confidence: high]** `.agents/docs/changes/2026-09-01-harness-upgrade-v1.15.0.md`
  (closing paragraph) — refers to "ADR-046 … and its amendment for where
  styrman-side workarounds … belong". ADR-047 is a separate ADR, not an amendment
  to ADR-046, and its number appears nowhere in the changelog. A reader following
  the changelog to `DECISIONS.md` will land on ADR-046 and find no such
  amendment. Name ADR-047 explicitly.

- **[low / confidence: high]** `TODO.md` — the deferred `internal/httpapi/setup.go`
  Settings-roster item is recorded only inside my prior review report. **I agree
  the deferral is correct** (reasoning below), but `TODO.md` is the project's
  declared home for "small deferred items" (`ROADMAP.md` division-of-labor
  comment) and is currently empty. A review report is an archive, not a work
  queue; the item will be lost. One bullet in the Inbox.

## Disposition of each prior finding

| Prior finding | Status |
|---|---|
| **high** — 60 s boot block | **RESOLVED**, independently re-measured (details below) |
| **medium** — no `v1.15.0` tag | **OPEN**, accepted-and-scheduled; see finding above |
| **medium** — unrecorded sync command | **RESOLVED**, and I executed the documented command to confirm it |
| **medium** — verifier DoD row unfilled | **PARTIALLY RESOLVED** — filled for the vendoring, stale for the fix; see finding above |
| **low** — stale `~/.agents/bin` comments | **RESOLVED** (`entrypoint.sh:113-115` and `:124-125` both updated) |
| **low** — wrong changelog file counts | **RESOLVED**, all four numbers re-run and exact: modified 20, deleted 25, added-files 39, 136 → 150 |
| **low** — trail-check "both exit 0" claim | **RESOLVED**, narrowed to hook mode with the `--check` counterexample cited |
| **low** — `setup.go` empty Settings roster | **DEFERRED, agreed** — see below |
| **low** — ROADMAP split code span / stale "Now" | **RESOLVED**, span gone, "Now" empty per ADR-045, gap added under "Next" *including* the missing boot-latency assertion |
| **low** — VENDORED.md "downgrade" wording | **RESOLVED**, reworded to "change"/"re-seed" |

## Evidence for the high finding's resolution (measured, not accepted)

Run inside `styrman-runtime:dev` (`1a8381a805a1`), mirroring
`image/entrypoint.sh:136-143` exactly — `useradd`, `mktemp -d` copy of
`/styrman/harness`, then `runuser -u t -- env HOME=<h> [PATH=…] bash <copy>/setup.sh`:

- **A, trimmed PATH** (`/usr/bin:/bin:/usr/sbin:/sbin`): `rc=0`, **456 ms**.
- **B, default PATH** (control): `rc=0`, **60,900 ms**.
- `diff -r -x '*.bak.*' /home/tA /home/tB` → the ONLY differences are agy's own
  runtime state, created by *running* agy in B and by nothing in the harness:
  `.cache`, `.gemini/antigravity-cli/` (incl. an agy crash log),
  `.gemini/config/{config.json,mcp_config.json,.migrated,projects}`. Nothing is
  "only in A"; no file differs in content. **The harness deploy is identical.**
- Roster in A: `~/.claude/agents` 6, `~/.gemini/config/plugins/harness/agents` 6,
  `~/.claude/skills/*/` 28, `~/.gemini/config/skills/*/` 28 — same as B.
  `CLAUDE.md`/`GEMINI.md` 69 lines each. `settings.json` parses as valid JSON.
  `~/.agents/bin/{resume-context.sh,trail-check.sh}` present, `0755`.
  `grep -icE 'error|command not found|Traceback|not found'` on A's log → **0**.
- The 61 s buys nothing even when it runs: B's log line is
  `agy skills: 0 records listed; 0/28 deployed skills present by name` — a minute
  spent producing a false negative on an unauthenticated home.

**PATH audit (the dispatch's specific question).** `/usr/local/bin` in the image
contains exactly `agy go gofmt helm styrman-git-credential styrman-setup tofu yq`;
`/usr/local/sbin` is empty. Grepping all eight names with word boundaries across
`harness/setup.sh`, `harness/antigravity/setup.sh`, `harness/claude/setup.sh`
returns hits for **`agy` only**, all inside or below the
`command -v agy` guard at `antigravity/setup.sh:237`. Every other command the
three scripts invoke — `bash cp mv rm mkdir chmod date rsync python3 sed grep
head git jq` — resolves under `/usr/bin` or `/bin`. `command -v` / `which` /
`type -p` appear exactly once in the three scripts, and it is that one guard, so
no absolute path derived from PATH is baked into any deployed file.

**No collateral on the guest.** `docker exec <live project> bash -lc 'echo $PATH'`
→ `…:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`, and
`command -v agy go yq` all resolve to `/usr/local/bin`. `grep -n PATH` across
`image/entrypoint.sh`, `image/Dockerfile` and `image/configs/*` shows the trim
exists on exactly one line (`entrypoint.sh:143`) with no export and no profile
change — it is scoped to that single `runuser … env` call, as claimed.

**One non-obvious consequence I checked and cleared.** The trimmed run no longer
creates `~/.gemini/antigravity-cli/`, because agy never runs. That directory is
where ADR-032 credential materialization writes
`antigravity-oauth-token`, and where `internal/httpapi/setup.go:41,245` and
`internal/httpapi/credharvest.go:136` probe for it. Not a problem:
`image/configs/styrman-credentials.sh:65` does
`mkdir -p "$(dirname "${__p}")"` before writing, and the Go side tests for the
*file*, not the directory. No regression.

**Sync command verified executable.** I ran the newly documented
`rsync -a --delete --exclude /VENDORED.md ~/workspace/devshell_prompt/harness/ image/harness/`
into a sandbox copy: `rc=0`, the root `VENDORED.md` survived the `--delete`, the
unrelated upstream `shared/skills/VENDORED.md` was transferred (the anchor works
as documented), and `diff -rq` against the current tree reported no differences —
i.e. the working tree is exactly what a fresh sync produces. The documented check
line reproduces: `diff -rq image/harness ~/workspace/devshell_prompt/harness` →
`Only in image/harness: VENDORED.md`.

**Fix hygiene.** `bash -n image/entrypoint.sh` OK; exec bit intact; the diff is
the one PATH token plus comments; `go build ./...` and `go vet ./...` clean. The
comment block above the call is accurate on every claim I could check, including
the `timeout` rejection (the self-check is indeed the last step of
`antigravity/setup.sh`, so a timeout there would leave the marker unwritten).
`deploy/.env` has `STYRMAN_IMAGE=styrman-runtime:dev`, so the platform will use
the fixed image on the next project create.

## On item 6 — deferring `internal/httpapi/setup.go`

**I agree it is safe to defer, and would have deferred it too.** It is
pre-existing (predates v1.15.0 by many versions), the impact is confined to three
`omitempty` fields silently absent from one Settings response — no functional,
data, security or latency consequence — and a correct fix requires a product
decision this release has no business making: the setup VM runs with
`STYRMAN_MINIMAL=1` and legitimately has no seeded harness, so "which home should
the card report" is an open question, not a typo. Fixing it inside a
verbatim-mirror vendoring release would also widen the blast radius of a change
that currently touches zero Go. One condition, per the finding above: put it in
`TODO.md` so the decision survives the commit.

## Criteria (re-checked where the fixes touched)

1. **Dead code — pass.** The fixes added no code, only one PATH token and prose.
   No new unreferenced file, no commented-out block (the long entrypoint comment
   is rationale, not disabled code). The only standing dead-reporting item is the
   deferred `setup.go` roster, agreed above.
2. **Flow tracing — pass** (was fail). Re-traced `entrypoint.sh:136-143` →
   `harness/setup.sh` → both vendor setups → the three deploy targets under the
   trimmed PATH, and then forward into the two consumers that depend on the
   resulting home state: the login-shell credential snippet (creates its own
   parent dirs — safe) and `internal/httpapi/setup.go`'s token probe (tests the
   file — safe). Error handling is unchanged and still sound: `set -euo pipefail`
   throughout, failure leaves the marker unwritten so the next boot retries, and
   the entrypoint's `|| log WARN` keeps it non-fatal. Timing now fits inside R1.
3. **Standards conformance — pass** (was fail on R1). Measured 456 ms for the
   seeding block and a reported 2.69 s cold create against R1's 5 s target /
   15 s hard limit; ADR-045's "~5–10 s Kata cold boot" holds again. Still no Go
   or TypeScript in the diff, so `golang`/`typescript` do not apply; `go build`
   and `go vet` re-run clean here. No new dependency, no secrets, no auth/crypto
   surface touched by the fixes.
4. **Directory layout — pass.** New artifacts landed correctly:
   verification log in `.agents/docs/testing/` under the `<date>-verify-<slug>.md`
   convention the shipped `trail-check.sh` expects (which also closes the
   convention-divergence note from my prior report), Tier 5 appended to the
   existing `testing/styrman/README.md`, ADR-047 appended to `DECISIONS.md`,
   `ROADMAP.md` at the repo root.
5. **Simplicity / DIY-except-security — pass.** The chosen fix is the smallest
   one available: a single `PATH=` on an existing `env` invocation, no new
   process, no new file, no background job, no `timeout` wrapper and no edit
   inside the vendored tree. ADR-047 generalizes it into a rule and honestly
   records its cost. Nothing hand-rolled in auth, crypto, sessions or JWT/OIDC.
6. **Decoupling — pass, improved.** ADR-047 turns the previously implicit mirror
   boundary into a stated rule with a named escape hatch (raise it upstream),
   and VENDORED.md now makes the seam operable by anyone rather than dependent on
   operator memory. The one remaining softness is that the workaround lives away
   from the code it compensates for — acknowledged in ADR-047 and mitigated by
   the low findings above (spec sentence + Tier 5 assertion).
