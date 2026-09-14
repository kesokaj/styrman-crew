---
owner: explorer
date: 2026-09-14
---
# Recon — harness upstream items (2026-09-14)

## Question
Resume styrman-harness's trail, confirm its state vs the commit styrman-crew
mirrors, and describe three upstream items styrman-crew carries: BUG-058
(vendored `TestDryRunNeverBuilds` fails in a plain copy), the "run it, do not
hand it back" root-steps sentence in `context/dev-setup.md`, and path
validation lows in the manifest name→path derivation.

## Summary
`styrman-harness` `main` is clean, at `8046e4d5f24b009bd2461ebef28931cade7a4acd`
(2026-09-14, "Handoff: BUG-015 fixed and the mirror-refresh prompt…") —
exactly the commit `image/harness/COMMIT` in styrman-crew records, so the
mirror is fully current (`git log 8046e4d..HEAD` is empty). BUG-058 is real
and already documented on both sides: the harness's own `VENDORED.md` names
it as "upstream" and it is NOT yet in the harness's `bugs/INDEX.md` (no
BUG-058 entry found there — only BUG-015 exists in the index at that number
range). The root-steps sentence is exactly one line in `context/dev-setup.md`
line 8, rendered verbatim into every consumer's `AGENTS.md` via `sh-agent
install`'s context assembly (`plan.go` line 88, `context` blob = harness's
own `context/AGENTS.md` + `context/dev-setup.md` appended). The frontmatter
`name:` → path derivation in `inventory.go` takes `fm["name"]` unchecked
into `filepath.Join` in `plan.go`; the module is at Go 1.24.4, so
`filepath.IsLocal` (available since 1.20) fits as a guard with no toolchain
bump needed.

## Trail resumed (styrman-harness)
- `AGENTS.md`: describes repo as harness (context/skills/agents/commands/rules
  + `sh-agent`); setup commands `gofmt -l . && go vet ./... && go test -race ./...`
  before any completion claim; `go run ./cmd/sh-agent install --home
  .agents/scratch/home` to exercise the installer; `go run ./cmd/sh-agent list`.
- `TODO.md`: empty (format-only, no items).
- `ROADMAP.md`: Now/Next/Later all empty — no active plan.
- Newest handoffs (`.agents/docs/handoffs/`, newest first):
  `2026-09-14-bug-015-and-mirror-refresh-prompt.md`,
  `2026-09-14-session-close.md`, `2026-09-14-known-limits-sentence.md`.
- `.agents/docs/bugs/INDEX.md`: BUG-015 = "installer/binary … fixed …
  `install` rebuilt from source into a read-only, git-less mirror copy
  whenever `go` was on PATH … gate the rebuild on `IsCheckout`; a change to
  `install` traces every consumer named in `deploy/remote.md § Downstream
  mirrors`". No BUG-058 row found in this index (searched with grep for
  "BUG-058" across the repo — the only hits are inside styrman-crew's own
  `VENDORED.md`, none inside styrman-harness itself). This is the one
  finding worth operator attention: the harness has not yet recorded its own
  bug for the vendored-test failure that styrman-crew's docs call "BUG-058,
  upstream".
- `git status`: clean, `main` up to date with `origin/main`.
- Last commit: `8046e4d5f24b009bd2461ebef28931cade7a4acd`, 2026-09-14,
  "Handoff: BUG-015 fixed and the mirror-refresh prompt for styrman-crew and
  argolis-devbox (checked against both consumers)".
- `git merge-base --is-ancestor 8046e4d… HEAD` succeeds and
  `git log --oneline 8046e4d..HEAD` is EMPTY — HEAD *is* 8046e4d. styrman-crew's
  mirror is at parity with upstream `main`, not behind it.
- Test/verify commands (from AGENTS.md "Testing" section): no test framework
  beyond `go test`; a change to `context/`/`agents/`/`skills/` is verified by
  `go run ./cmd/sh-agent install --home .agents/scratch/x` succeeding + `status`
  showing no drift, then one CLI start per vendor with logs grepped for parse
  errors (`.agents/docs/testing/2026-09-03-verify-sandbox.md`); full matrix at
  `.agents/docs/testing/skill-matrix.md`; real-home smoke checks at
  `.agents/docs/testing/2026-09-03-verify-real-home.md`.

## (a) BUG-058 — vendored `TestDryRunNeverBuilds` fails in a plain copy

- Test: `cmd/sh-agent/main_test.go:196-225`. It runs `install --dry-run --repo
  <this checkout> --home <tmp>` and asserts the output contains
  `"[dry-run] the binary would be rebuilt from"` — the CHECKOUT-shaped
  message. `repo` is computed as `filepath.Abs("../..")` (line 201) —
  relative to the test file's own package directory, so it resolves to
  whatever tree the test happens to live inside.
- Dependency: `IsCheckout` at `internal/harness/repo.go:75-78`:
  ```go
  func IsCheckout(repo string) bool {
      _, err := os.Lstat(filepath.Join(repo, ".git"))
      return err == nil
  }
  ```
  It never asks git — only checks for a `.git` entry directly inside `repo`
  (by design, per the doc-comment above it: git's answer "can be redirected
  by its environment").
- The two message shapes, in `cmd/sh-agent/main.go`:
  - checkout shape (line 131): `fmt.Fprintf(out, "[dry-run] the binary would
    be rebuilt from %s\n", repo)`
  - plain-copy shape (line 143): `fmt.Fprintln(out, "[dry-run] plain copy,
    not its own git checkout: the running binary would be installed as is")`
- In styrman-harness's own checkout, `../..` from `cmd/sh-agent/` has a
  `.git` (it's the harness's own tree), so `IsCheckout` is true and the test
  passes there. In styrman-crew's vendored mirror (`image/harness/`), that
  same relative path resolves to `image/harness/` — nested inside
  styrman-crew's own `.git`, with no `.git` of its own — so `IsCheckout` is
  false and `install` prints the plain-copy message instead, failing the
  test's `strings.Contains` check for the checkout-shaped string.
  styrman-crew's own `image/harness/VENDORED.md` (§ Refresh, read directly)
  already documents this: "The mirror's own `go test ./...` is not a check
  here, and it is red by construction: `TestDryRunNeverBuilds` … asserts the
  checkout-shaped dry-run output and fails in any plain copy (BUG-058,
  upstream); `deploy/verify-commit.sh` vets the mirror and never runs its
  tests."
- Fix shapes the test itself could take (not applied — read-only recon):
  either (1) build its own checkout-shaped temp dir (a `t.TempDir()` with a
  bare `.git` file/dir planted in it, `--repo` pointed there instead of
  `../..`) so the test is independent of what tree it happens to sit inside,
  or (2) skip when `!harness.IsCheckout(repo)` (mirroring what `plainCopy(t)`
  a few lines below already does for its own scenario — see
  `main_test.go:227-238`, which manually assembles a plain-copy fixture from
  `context/skills/agents/commands/rules` for a *different* test). Option (1)
  is more robust since it doesn't depend on the ambient tree's git-ness at
  all.
- BUG record: **not found in the harness's own `.agents/docs/bugs/INDEX.md`**
  under that or any number — only styrman-crew's `VENDORED.md` names it
  "BUG-058, upstream". Whether that number corresponds to a harness-side
  bug record that hasn't landed, or is styrman-crew's own numbering scheme
  applied to an upstream defect, was not determined from the code alone —
  the harness repo itself has no BUG-058.

## (b) The root-steps sentence in `context/dev-setup.md`

- File: `styrman-harness/context/dev-setup.md:8` (bullet under "Environment
  & Stack"), quoted verbatim:
  > "**Root steps on the operator's own Linux box:** passwordless `sudo -n`
  > is available. When the operator says to run a root step (an installer,
  > `host-setup`, a host clean-up), run it — do not hand it back — and
  > record the exact command plus the verification (`systemctl
  > is-enabled …`, `du` before an `rm -r`) in the change entry. Look at the
  > target before deleting or overwriting it. Project `.claude/settings.json`
  > records the standing permission."
- Reach path: `context/AGENTS.md` is the single global-context source; per
  `AGENTS.md`'s own layout table, `context/dev-setup.md` is "appended to the
  context at install." Concretely, `plan.go:88` writes
  `File{Rel: filepath.Join(ClaudeRoot, "AGENTS.md"), Data: context, ...}`
  where `context` is the assembled bytes (harness AGENTS.md + dev-setup.md
  appended); the same `context` blob is also written to the Antigravity path
  (`plan.go:94`, `AgyRoot/rules/AGENTS.md`). Every consumer project (any home
  `sh-agent install --home <home>` targets, including a project VM image
  seeding `~/.claude/AGENTS.md`) gets this sentence verbatim, unscoped to
  that consumer's own environment — it is written as if EVERY consumer is
  the operator's own bare-metal/VM box with `sudo -n`, which is not true for
  a project microVM whose "root" is the guest, not the operator's host.
- Scoping mechanisms that would fit the harness's existing conventions
  (not implemented, options only):
  - A **project-level rule** in `.agents/rules/` that overrides or narrows
    this bullet for that project — matches the documented priority ("A
    project's own AGENTS.md overrides this" is already the harness's stated
    override mechanism, per `context/AGENTS.md`'s own header line quoted in
    styrman-crew's CLAUDE.md chain).
  - A **precondition the agent checks** before treating "run a root step" as
    license — e.g. requiring the agent to first confirm it is running on
    the box the sentence describes (operator's own Linux box) rather than
    inside a project guest, mirroring the harness's existing pattern of
    gating a mechanism on a checkable fact (`IsCheckout` above is the same
    shape: decide from a directly-observable property, never trust
    environment/ambient claims).
  - A **config flag** passed to `sh-agent install` (e.g. a vendor/consumer
    flag already exists: `NormalizeVendor` in `apply.go:19`) that could
    select which `context/*.md` fragments get appended per target — the
    installer already has a "vendor" axis (`--vendor V`) that this could
    extend to a "profile" axis if the harness wanted per-consumer context
    assembly instead of one-size-fits-all.
  No such mechanism exists today; `dev-setup.md` is appended unconditionally
  to every install regardless of target.

## (c) Path-validation lows: manifest `name:` → path derivation

- Module Go version: `go.mod` line 3: `go 1.24.4` — well above the 1.20
  minimum for `filepath.IsLocal`.
- Parse boundary: `internal/harness/inventory.go`:
  - `LoadSkills` (~line 95-97):
    ```go
    fm := Parse(front)
    name := fm["name"]
    if name == "" { name = e.Name() }
    ```
  - `LoadAgents` (~line 131-134):
    ```go
    fm := Parse(front)
    name := fm["name"]
    if name == "" { name = strings.TrimSuffix(filepath.Base(p), ".md") }
    ```
  `fm["name"]` comes straight from YAML-ish frontmatter parsed by `Parse`/
  `Split` (`frontmatter.go`) with no path-shape validation — no rejection of
  `..`, no absolute-path rejection, no separator check.
- Consumption: `internal/harness/plan.go` joins that `Name` directly into
  destination paths with no re-validation, e.g.:
  - line 122: `filepath.Join(ClaudeRoot, "skills", s.Name, rel)`
  - line 136: `filepath.Join(ClaudeRoot, "agents", a.Name+".md")`
  - line 143: `filepath.Join(AgyRoot, "agents", a.Name, "agent.md")`
  A frontmatter `name: ../../../../etc/passwd` (or similar) in a skill's
  `SKILL.md` or an agent's `.md` would flow through unchecked into these
  `filepath.Join` calls; `filepath.Join` itself does not sandbox `..`
  segments — the result can escape `ClaudeRoot`/`AgyRoot`.
- `filepath.IsLocal(name)` (Go 1.20+) fits exactly at the point `name` is
  derived from frontmatter, before it's stored in `Skill.Name`/`Agent.Name`:
  it reports false for paths containing `..`, an absolute path, or an empty
  string, all in one call — the natural gate at the parse boundary, not at
  every `filepath.Join` call site downstream. Today the harness's threat
  model is arguably "the content is the operator's own repo" (skills/agents
  are files the operator authored), which may be why this was left as a
  "low" rather than fixed — recon does not judge severity, only shape.

## Consumer mirror refresh (styrman-crew's own docs)
- `image/harness/VENDORED.md` § Refresh: `rsync -a --delete` from
  `~/workspace/styrman-harness` into `image/harness/` (excluding `.git`,
  `.agents`, `.gitignore`, and the mirror's own `COMMIT`/`VENDORED.md`/
  `SHA256SUMS`), then `git -C "$H" rev-parse HEAD > image/harness/COMMIT`,
  then a `SHA256SUMS` regeneration, then a `diff -rq` check expecting exactly
  three residual-only lines. Requires the harness checkout to be clean first
  (`test -z "$(git -C "$H" status --porcelain)"`).
- Pin style: **full commit SHA**, not a tag — `image/harness/COMMIT` holds
  `8046e4d5f24b009bd2461ebef28931cade7a4acd` today, matching harness `HEAD`
  exactly. The harness repo itself has **no git tags** (`git tag` empty) —
  it has no release/tag convention of its own; styrman-crew's release
  procedure (`.agents/docs/specs/styrman/versioning.md`, not read in this
  recon) is what names the new `COMMIT` in its own `.agents/docs/changes/`
  entry when it re-mirrors, per `VENDORED.md`'s closing paragraph.

## Not examined
- `.agents/docs/specs/styrman/versioning.md` release-procedure text itself
  (only referenced via `VENDORED.md`'s summary of it).
- The full `deploy/remote.md § Downstream mirrors` list of BUG-015-affected
  consumers.
- `argolis-devbox`'s own mirror state (out of scope; not this repo).
- Whether a harness-side BUG-058 record is queued/planned but simply not
  yet written (only confirmed it is absent from the index as of this scan).
- `plainCopy(t)` helper's full body past line 238 (only skimmed the start).

recon complete
