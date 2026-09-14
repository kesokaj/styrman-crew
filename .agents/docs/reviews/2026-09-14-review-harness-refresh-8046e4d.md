---
owner: reviewer
date: 2026-09-14
---
# Review — harness-refresh-8046e4d   (2026-09-14)

Range reviewed: v1.30.1..worktree — `git diff HEAD` (63 tracked files) plus the
five untracked, non-ignored paths under `image/harness/`
(`cmd/sh-agent/main_test.go`, `skills/golang/references/golang-samber.md`,
`skills/golang/references/golang-structure.md`,
`skills/init-project/assets/agents-tree/docs/retros/`, `skills/review/`).
Reviewed in full: the seven crew-authored files (`image/entrypoint.sh`,
`.agents/docs/testing/styrman/integration.sh`, `deploy/verify-commit.sh`,
`README.md`, `image/README.md`,
`.agents/docs/specs/styrman/runtime-image.md`,
`.agents/docs/testing/styrman/README.md`) and the vendored Go that compiles
into the shipped binary (`image/harness/{go.mod,cmd/sh-agent,internal/harness}`).

## Verdict: APPROVE WITH NITS

Nothing in this change set is broken. The load-bearing claim — that dropping
`GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` from the entrypoint's
install line is safe because `sh-agent install` on a plain copy never execs
git — traces clean through the vendored Go and holds (see Flow tracing
below). Mirror fidelity, the integrity manifest, and the roster counts all
verify against the live sibling checkout. The two findings below are a
partially self-referential new integration check and a documentation gap
around the mirror's own (permanently red, upstream-owned) test suite; both
are low/medium, neither blocks a release.

## Verified by running it (not by reading)

- `diff -rq --exclude .git --exclude .agents --exclude .gitignore
  ~/workspace/styrman-harness image/harness` → **exactly the three expected
  lines** (`COMMIT`, `SHA256SUMS`, `VENDORED.md`). Upstream checkout is
  clean; `git -C ~/workspace/styrman-harness rev-parse HEAD` =
  `8046e4d5f24b009bd2461ebef28931cade7a4acd` = `image/harness/COMMIT`. The
  refresh recipe in `VENDORED.md` § Refresh was followed exactly.
- `(cd image/harness && sha256sum -c SHA256SUMS)` → **rc 0**, 154 entries,
  and `find . -type f ! -name SHA256SUMS | wc -l` = 154 (no unlisted file).
  `find . -type l` → empty. This is what `deploy/verify-commit.sh:92-99`
  re-checks in the commit's own worktree; its `cut -c67-` offset is correct
  for `sha256sum`'s 64-hex + two-space prefix.
- `(cd image/harness && go vet ./...)` → **rc 0** (what the release gate
  runs, `deploy/verify-commit.sh:92`).
- `(cd image/harness && go test ./...)` → **rc 1**, read bare per
  `.agents/rules/verify-bare.md`. `TestDryRunNeverBuilds` fails; already
  recorded as **BUG-058** (open, upstream). See finding 2 for the residual.
- Roster: `ls image/harness/skills/*/SKILL.md | wc -l` = **27**;
  `ls image/harness/agents/*.md | wc -l` = **7**. Matches every updated
  count in `integration.sh:114`, `runtime-image.md:235`, `image/README.md:8`
  and `testing/styrman/README.md`, and matches what
  `deploy/verify-commit.sh:193-194` derives from the tree rather than
  restating (the BUG-025 rule, correctly applied).
- `image/harness/commands/` and `rules/` are empty but carry tracked
  `.gitkeep` files, so `LoadDocs`' glob and the mirror's checksum list both
  behave identically in a fresh worktree. `image/harness/.agents/` exists in
  the working tree as empty directories only (`find -type f` → nothing), so
  it neither enters `SHA256SUMS` nor reaches git; no action needed.

## Findings

- **[low / confidence: high]** `.agents/docs/testing/styrman/integration.sh:129-130`
  — the new check "boot log records the harness deployed at the mirror's
  commit" compares the boot log against values read from *the same two files
  inside the same container* that produced the log line. The entrypoint
  builds the line from `cat /styrman/VERSION` and
  `cut -c1-7 /styrman/harness/COMMIT` (`image/entrypoint.sh:131,177`); the
  check interpolates `docker exec ${CN} cat /styrman/VERSION` and
  `docker exec ${CN} cut -c1-7 /styrman/harness/COMMIT`. On the commit axis
  it re-asserts a constant against itself: a mirror whose `COMMIT` disagrees
  with the tree it labels still passes, which is exactly the failure
  `VENDORED.md` says `COMMIT` is unguarded against ("an attestation by
  whoever refreshed"). Second, smaller edge: if the inner
  `docker exec … cut` fails, the substitution is empty and the grep pattern
  ends at `styrman-harness ` — which still matches the real log line, so
  the commit half fails *open*. The check's real and useful content is "the
  deploy line was printed at all this boot", which is what the script's own
  comment says; the check NAME and the testing README's prose
  (`testing/styrman/README.md`, the "Automated (Tier 2…)" paragraph) claim
  more than it proves. Concrete fix: name it for what it verifies ("boot log
  records the harness deployed this boot"), and compare the sha against the
  HOST's `image/harness/COMMIT` (the way `deploy/verify-commit.sh:202` does)
  if a real mirror-vs-stamp assertion is wanted.

- **[low / confidence: high]** `image/harness/VENDORED.md` — the mirror's own
  `go test ./...` is red in this repo and will stay red for every future
  refresh (BUG-058), yet nothing in `VENDORED.md` § Refresh says so. The
  next person to refresh the mirror will run the module's tests, see
  `FAIL github.com/kesokaj/styrman-harness/cmd/sh-agent`, and have to
  rediscover from scratch that it is upstream's tree-shape assumption and
  not a broken refresh — or, worse, treat a *different* future failure in
  that package as "the known one". `deploy/verify-commit.sh:92` runs only
  `sha256sum -c` and `go vet ./...` against the mirror, so the gate is
  honest; the gap is purely the refresher's instructions. Concrete fix: one
  line in `VENDORED.md` § Refresh — "the mirror's own `go test ./...` fails
  in `cmd/sh-agent` by construction (BUG-058, upstream); the crew gate vets
  and never tests it" — with the BUG-058 id so a second, unrelated failure
  is still visible.

Not findings, checked and dismissed:

- **The dropped `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1`
  (`image/entrypoint.sh:137`) is safe.** Traced every exec in the module:
  `os/exec` appears only at `internal/harness/apply.go:181` (`diff`, the
  `diff` command only) and `internal/harness/repo.go:122,150,156,167`
  (`git pull`, `go build`, the rebuilt binary, `gitOut`). On the install
  path the only reachable one is `Inspect` → `gitOut`, and `Inspect`
  (`repo.go:83-86`) returns `RepoInfo{}` before any exec when
  `IsCheckout(repo)` is false. `IsCheckout` is a bare `os.Lstat(repo/.git)`
  that deliberately never asks git (`repo.go:75-78`), so its answer cannot
  be redirected by the home's git environment — which is precisely what the
  removed guard existed to contain. `/styrman/harness` ships with no `.git`
  (confirmed: the mirror has none, and `image/Dockerfile` copies it
  root-owned + `chmod -R go-w`, so the project user cannot plant one). The
  `timeout 20` remains, so even a hypothetical reintroduced git call is
  bounded, and the whole block runs under `runuser -u "${USERNAME}"` — the
  home's owner and the process's principal are the same, so no boundary is
  crossed either way. The entrypoint comment's new text states this
  accurately, including that it is now the timeout alone.
- **`install --repo /styrman/harness` still reaches the plain-copy path
  after the `--vendor` rework.** `resolveVendor("", home)` →
  `VendorAll` on a fresh home and `"all"` from the manifest on a re-seed
  (`state.go:41-43` back-fills an empty `vendor`), so a re-seed never
  narrows the installed set. `knownCheckout` returns `nil` immediately
  because `--repo` is explicit (`main.go:361`). `shouldRebuild` is false
  (`own` is false), so `Reinstall` — the only path that would `go build`
  into the root-owned, `go-w` mirror or write `repo/.agents/scratch/` — is
  unreachable in the image, and `update` is refused earlier by
  `Behind` → `errPlainCopy`.
- **The status string the new check greps is exact.** `main.go:234` emits
  `repo       %s (plain copy, not its own git checkout; update is refused)`;
  `integration.sh:128` greps the substring from `/styrman/harness (` onward.
  `grep -F … >/dev/null` rather than `grep -q` is the right call under this
  script's `set -euo pipefail` (BUG-057, recorded and fixed in this same
  change).
- **Check counts are internally consistent.** `grep -c 'check "'
  integration.sh` = 97, matching the README's "97 checks … passed=94
  failed=0 skipped=3" (was 95/92, +2). The harness-section prose enumerates
  seven assertions (drift, plain-copy label, boot log, CLAUDE.md stub,
  27/7 roster, no previous-harness artifacts, boot→ready) — consistent with
  "five" before the two additions.
- **Placement of the new checks is correct.** Both sit in the harness
  section (`say` at line 112), immediately after project create and well
  before "persistence across container destruction" (line 378), which is the
  only section that destroys and recreates the container. That ordering
  matters: on a second boot the `~/.styrman/.harness-version` stamp matches,
  the whole seeding block is skipped, and the `agent harness deployed` line
  is *not* re-emitted — so moving the boot-log check after line 378 would
  break it (the BUG-021 family). Worth knowing; not a defect today.
- **No live dangling reference to the two deleted skills.** Grepped the
  crew tree for `/audit`, `skills/audit`, and `gcloud-auth-verification`
  outside the mirror: every remaining hit is in dated
  `.agents/docs/{changes,reviews,testing,plans,handoffs}` records, which are
  history. No hardcoded roster count in the server either — the Settings
  roster is read from the project home at runtime
  (`internal/httpapi/setup.go:166-200`), not from a constant.
- **`README.md:145-150`** now declines to restate a roster count in the
  v1.20.0 history line and points at `image/README.md` instead. This is a
  history line being corrected for accuracy, not an append-only record being
  rewritten, so it does not conflict with AGENTS.md's append-only rule for
  `changes/` and `DECISIONS.md`.
- **`deploy/verify-commit.sh:48`** — "151 files" → "150-odd files": prose
  only, and now deliberately approximate so it does not go stale again. The
  real count (154) is derived, not restated, everywhere it matters.

## Criteria

1. **Dead code — pass.** The `--vendor` rework removes no callers and leaves
   none orphaned: `Plan`, `Install` and `status` all take the new parameter
   and every call site passes it (`main.go:153,157,163,170,295`). Two skill
   directories were deleted outright rather than commented out
   (`skills/audit/`, `skills/gcloud-auth-verification/`), with no surviving
   reference anywhere in the crew tree. `slashNote`
   (`inventory.go:276-283`) replaces a sentence that described nothing when
   no skill carries `disable-model-invocation` — a deletion, not a wrapper.
2. **Flow tracing — pass, with one weak assertion (finding 1).** Traced the
   entrypoint's install line → `main.run("install")` → `FindRepo` →
   `resolveVendor` → `knownCheckout` → `shouldRebuild`/`IsCheckout` →
   `Plan` → `Install` → `Inspect`, confirming no git exec and no rebuild on
   the plain-copy path, and that the `--vendor` default can never narrow a
   re-seed. Installer-as-caller check (BUG-035 rule): the only caller that
   must act on the mirror refresh is `image/entrypoint.sh`, and it is
   unconditional per boot, idempotent, and gated only on the image-version
   stamp — not on "did something change this tick". The new `status` line
   reads `buildinfo.ReadFile(home/.local/bin/sh-agent)` and degrades to
   `binary unknown (…)` on any read error rather than failing the command,
   so the existing "no drift" check cannot start 500-ing on a home without
   an installed binary. Both new integration checks can fail (mutation-check
   per BUG-036): the plain-copy check fails if the message or the mirror's
   shape changes; the boot-log check fails if the deploy line is absent —
   though its commit half is self-referential and fails open, which is
   finding 1.
3. **Standards conformance — pass.** `review-standards` Go checklist: errors
   wrapped with context at every new site (`apply.go:21-23`,
   `main.go:366`, `repo.go:144-161`); no discarded `err` that matters
   (`Inspect`'s `_` on `gitOut` is deliberate and documented, and now
   unreachable for a plain copy); no floating goroutines or unclosed
   resources introduced. `go vet ./...` rc 0 on the mirror module. Shell:
   `integration.sh` runs `set -euo pipefail` and the new checks respect it —
   `grep -F … >/dev/null` rather than `grep -q` is the pipefail-correct form
   and is documented at the call site. `image/entrypoint.sh` keeps the
   argument-passing discipline for the stamp write (audit 2026-09-13) and
   the `timeout`-per-step structure. My own verification of the red test was
   run bare with its exit status read, per `.agents/rules/verify-bare.md`.
4. **Directory layout — pass.** Everything vendored stays under
   `image/harness/` at the exact paths upstream uses; the Go module keeps
   `cmd/sh-agent/` + `internal/harness/`, and the new test file sits beside
   the code it covers (`cmd/sh-agent/main_test.go`). Crew-side edits touch
   only files that already existed. The new untracked
   `skills/init-project/assets/agents-tree/docs/retros/` mirrors upstream's
   asset tree; `skills/review/` replaces `skills/audit/` in place.
5. **Simplicity, DIY-except-security — pass.** `go.mod` is unchanged
   (stdlib only, `go 1.24.4`); the new code uses `debug/buildinfo`,
   `os/exec` and `flag` from the standard library. No auth, crypto, hashing,
   session or token handling is hand-rolled anywhere in the diff — the only
   security-adjacent primitive is `IsCheckout`'s `os.Lstat`, which is
   deliberately *not* delegated to git precisely so its answer cannot be
   redirected by the caller's environment. `--vendor` is three constants and
   a normalizer, not an abstraction layer.
6. **Decoupling — pass.** The vendor selection is a single normalized string
   threaded through `Plan`/`Install`/`status`; it does not leak into
   `LoadInventory`, `DeriveAgy` or the frontmatter parser. The crew↔harness
   seam is exactly two things — the `sh-agent` CLI contract and
   `/styrman/harness/COMMIT` — and every crew-side workaround stays at the
   call site in `image/entrypoint.sh`, as `VENDORED.md` requires; nothing in
   this diff edits the mirror to suit styrman. The one place the seam is
   thinner than it looks is finding 1: the boot-log check reaches back
   through the container for both sides of its comparison instead of
   holding one side on the host.

## Not examined

- **Vendored markdown prose** under `image/harness/{skills,agents,context,
  commands,rules}` — ~3,700 lines of the diff. Sampled for behaviour-
  affecting content only (the deleted `audit`/`gcloud-auth-verification`
  skills, the new `review` skill's existence, `skills/VENDORED.md`'s roster
  table, and the `Installed` counts rendered into the context). Its prose is
  upstream's and is out of scope per the dispatch; a separate docs-lens
  review (`.agents/docs/reviews/2026-09-14-review-harness-refresh-8046e4d-docs.md`)
  and a security review
  (`.agents/docs/reviews/2026-09-14-security-harness-refresh-8046e4d.md`)
  cover the same range from their own angles.
- `image/harness/internal/harness/harness_test.go` (+384 lines) and
  `cmd/sh-agent/main_test.go` were read only around the failing test and the
  `plainCopy`/`TestPlainCopyNeverRebuilds` fixtures; the remaining new cases
  were not individually mutation-checked. They are upstream's tests and are
  not run by any crew gate.
- No runtime image was built and no microVM booted during this review, so
  the two new integration checks were reasoned through against
  `image/entrypoint.sh` and the module source rather than observed green.
  The change entry records a full run at `passed=94 failed=0 skipped=3`
  (`.agents/docs/changes/2026-09-14-harness-refresh-8046e4d.md`), and the
  verifier's log is
  `.agents/docs/testing/2026-09-14-verify-harness-refresh-8046e4d.md`.
- `deploy/verify-commit.sh` was read whole but not executed (it builds both
  images; BUG-056's Docker Hub rate-limit window makes an unnecessary run
  costly).
- `web/` was untouched by this range and was not reviewed.
