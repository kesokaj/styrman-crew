---
owner: security
date: 2026-09-14
---
# Security review — harness-refresh-8046e4d   (2026-09-14)

Range reviewed: v1.30.1..worktree (working tree vs `HEAD` = bc0798d; 63 modified
files, 5 untracked paths: `image/harness/cmd/sh-agent/main_test.go`,
`skills/golang/references/golang-{samber,structure}.md`, `skills/review/`,
`skills/init-project/assets/agents-tree/docs/retros/.gitkeep`).
Mirror commit under review: `image/harness/COMMIT` =
`8046e4d5f24b009bd2461ebef28931cade7a4acd`.

## Verdict: FINDINGS

The central claim the change rests on is TRUE, checked against every call site
rather than the comment: on a plain copy (`/styrman/harness`, no `.git`)
`sh-agent install` runs **zero** git commands and never `go build`s. The worst
finding is elsewhere — `image/harness/context/dev-setup.md:8` adds a line
granting standing permission to run root commands without handing back
("passwordless `sudo -n` is available … run it — do not hand it back"). It was
written for the operator's own Linux box, but `RenderContext`
(`internal/harness/render.go:23-51`) concatenates every `context/*.md` into the
`~/.claude/AGENTS.md` and `~/.gemini/config/rules/AGENTS.md` that
`sh-agent install` seeds into **every tenant's project microVM**, where
`image/entrypoint.sh:55` has already given that user `NOPASSWD: ALL`. The
sentence's own bound ("Project `.claude/settings.json` records the standing
permission") is false there — the integration suite asserts no `settings.json`
exists — so an agent inside a tenant VM reads an unconditional grant. Everything
else is defence-in-depth: a free guard dropped from the entrypoint, and
unvalidated paths from a user-writable manifest and from frontmatter `name:`.

## Findings

- **[medium / confidence: high]** `image/harness/context/dev-setup.md:8` —
  **standing privileged-action permission shipped into the wrong trust
  context.** The new bullet tells the agent that passwordless `sudo -n` is
  available and to run root steps rather than hand them back. `RenderContext`
  (`internal/harness/render.go:29-48`) globs *every* `context/*.md` into the
  deployed context, so this lands verbatim in `~/.claude/AGENTS.md` and
  `~/.gemini/config/rules/AGENTS.md` in each project microVM
  (`image/entrypoint.sh:154`). The guest user really does have
  `NOPASSWD: ALL` (`image/entrypoint.sh:55`), and the `.claude/settings.json`
  the sentence names as the record of that permission does not exist in a
  guest — `.agents/docs/testing/styrman/integration.sh` asserts its absence.
  This is not privilege escalation: the tenant already owns that shell and
  that sudo. What it changes is the agent's behaviour — it removes the
  hand-back step for privileged actions inside a VM that the credential
  broker keeps signed in to the user's git/gh/ssh/gcloud, which widens the
  blast radius of indirect prompt injection (a README in a cloned repo, a
  dependency's docs) from "what the agent would ask about" to "root, without
  asking". Class: over-broad instruction crossing a trust boundary the text
  assumes it never crosses. Fix, cheapest first: move the bullet out of
  `context/` into a skill or a project-level `AGENTS.md` that only the
  operator's machine loads; or gate it on a marker the guest lacks; or, if it
  must stay global, restate the bound as a precondition the agent must check
  ("only when `.claude/settings.json` in this project records it") instead of
  as prose.

- **[low / confidence: high]** `image/entrypoint.sh:153` — **removed
  defence-in-depth, invariant now unasserted.** The install line dropped
  `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1`. The justification is
  correct today: `install` reaches git only through `harness.Inspect`
  (`internal/harness/apply.go:79`), which returns early on `!IsCheckout(repo)`
  (`internal/harness/repo.go:75,85`); `Behind`/`Pull` are `update`-only. But
  the property now rests entirely on `/styrman/harness` never containing a
  `.git` entry, and that is enforced only by convention
  (`image/.dockerignore`'s `**/.git`, the rsync recipe's `--exclude .git`) and
  observed only by a manual integration check — never at build time, never at
  boot. If a `.git` ever lands there, `Inspect` runs `git rev-parse`/`git
  status` against a tenant-written `~/.gitconfig` (the 2026-09-03
  FIFO-in-`include.path` hang) *and* `shouldRebuild`
  (`cmd/sh-agent/main.go:129,390`) flips true, execing `go build` inside a
  root-owned tree. Both are bounded by `timeout 20` and the non-fatal
  `HI_OK=0` path, so impact is a boot stall plus an undeployed harness, not a
  compromise. Fix: restore the two variables (correct and free on the
  plain-copy path too), or add `test ! -e /styrman/harness/.git` to
  `image/Dockerfile`'s harness `RUN` so the invariant fails the build rather
  than the boot.

- **[low / confidence: high]** `image/harness/internal/harness/apply.go:59-76`
  and `:207-221` — **unvalidated path input from a user-writable manifest.**
  `old.Files` / `st.Files` are read from `~/.config/sh-agent/state.json` in the
  tenant-controlled home (`internal/harness/state.go:23-45`) and used as
  `filepath.Join(home, rel)` then `os.Remove`d. `filepath.Join` *cleans* `..`
  rather than rejecting it, so `"files": ["../../../../etc/foo"]` resolves
  outside `$HOME`. Not a boundary crossing in crew — install runs as the same
  tenant who owns the shell in that microVM — but it is untrusted input
  treated as trusted at a parsing boundary, and the same code runs on the
  operator's workstation. Fix: drop any entry where `!filepath.IsLocal(rel)`
  in `LoadState`, before it can reach a removal. (Pre-existing; both files are
  modified in this range, so it is in the blast radius.)

- **[low / confidence: medium]** `image/harness/internal/harness/inventory.go:96`
  and `:135` → `internal/harness/plan.go:122,125,136,143` — **frontmatter
  `name:` becomes a filesystem path unvalidated.** `Skill.Name`/`Agent.Name`
  are taken verbatim from the YAML `name:` key and joined into the install
  destination; a skill declaring `name: ../../../.bashrc` writes outside the
  vendor root, and `writeAtomic` `MkdirAll`s the parent so the write succeeds.
  Unreachable from a crew tenant (`/styrman/harness` is root-owned and
  `chmod -R go-w`, and the tenant cannot create a `.git` or edit a SKILL.md
  there), but directly reachable through `sh-agent adopt`, whose stated purpose
  is importing a skill from elsewhere. Fix: validate against
  `^[a-zA-Z0-9._-]+$` (rejecting `.` and `..`) in `LoadSkills`/`LoadAgents`,
  beside the existing missing-description error.

- **[low / confidence: low]** `image/harness/internal/harness/repo.go:143-156`
  (`Reinstall`, new in this range) — **predictable build path, then exec.** The
  binary is built to `<repo>/.agents/scratch/sh-agent-build-<pid>/sh-agent` and
  immediately executed. The directory name is fully predictable and
  `os.MkdirAll` succeeds through a pre-planted symlink, so anyone who can write
  `<repo>/.agents/scratch/` can point the built path at content they control
  between `go build` and `exec`. That directory is inside the operator's own
  checkout (same user), and the path is unreachable for a crew tenant
  (plain copy ⇒ `shouldRebuild` false). Fix: `os.MkdirTemp(filepath.Join(repo,
  ".agents", "scratch"), "sh-agent-build-")`.

- **[low / confidence: low]** `.agents/docs/testing/styrman/integration.sh:130`
  — **container-sourced strings reach `eval` on the host.** `check(){ … eval
  "$2" … }` (line 52), and the new boot-log check interpolates
  `$(docker exec ${CN} cat /styrman/VERSION)` and
  `$(docker exec ${CN} cut -c1-7 /styrman/harness/COMMIT)` into the string that
  is then `eval`'d on the operator's host. Both files are root-owned and
  written at image build time, so nothing tenant-controlled reaches it today —
  this is a pattern note, not a live hole. Fix: capture into shell variables
  and pass them to `grep -F -- "$var"` through the environment rather than
  through `eval`'s text.

- **[info]** `image/entrypoint.sh:145-155` — the new comment states plainly that
  `timeout 20` "is the whole defence" against a hostile home. Accurate, and the
  failure mode is safe (`|| HI_OK=0` ⇒ WARN, boot continues, retried next boot,
  well inside the server's 90 s ready wait). Residual: a tenant can
  self-inflict ~40 s of boot delay (migrate + install), over R1's 15 s soft
  budget. Spec line, not a code change.

- **[info]** `image/harness` test suite is **red in situ**: `go test ./...` in
  `image/harness` exits 1 —
  `--- FAIL: TestDryRunNeverBuilds (cmd/sh-agent/main_test.go:211)`. Not a
  defect: the test resolves its repo as `filepath.Abs("../..")`, which inside
  this mirror has no `.git`, so the rebuild line it asserts is correctly absent.
  It means the mirror's own suite can never be a green signal here. The tests
  that matter for styrman do pass in place: `TestPlainCopyNeverRebuilds`,
  `TestNestedCopyIsNotACheckout`, `TestFindRepoRefusesNamedNonRepo`,
  `TestVendorSelection`, `TestReinstallBuildsAndInstalls`, and the whole
  `internal/harness` package (`ok`). `deploy/verify-commit.sh` runs `go vet` on
  the mirror, not `go test`, so the gate is unaffected — worth one sentence in
  `VENDORED.md` so a future session does not read the red as a regression.

## Checked

- **Secrets** — no credential, token, or key added anywhere in the range; the
  only secret-adjacent prose argues *against* hardcoding (golang/docker/gcloud
  skills, `context/AGENTS.md`). No `.env`, `*.pem`, `*.key` in the diff or the
  untracked set. `image/.dockerignore` still excludes `*.env`, `**/*.pem`,
  `**/*.key`. `sh-agent install`'s stdout is discarded
  (`image/entrypoint.sh:154`); the only logged line is version + mirror commit,
  so no home content is echoed into the boot log.
- **Injection** — every `exec.Command` in the mirror passes an argv slice, never
  a shell string: `repo.go:122,150,156,167`, `apply.go:181`; no `sh -c`, no
  interpolation into a command. `git`/`go`/`diff` resolve through `PATH`, which
  on the boot path is root's entrypoint environment (`runuser` preserves the
  caller's env), not the tenant's. Path-shaped injection is covered by the
  manifest and `name:` findings above; the one `eval` is the integration-script
  note above.
- **Authz/authn** — untouched. Nothing in the range reaches the ingress token,
  the `X-Caddy-Email` identity path, `internal/userstore`, or any HTTP handler.
  Install runs as the project user via `runuser -u "${USERNAME}"`; grepping
  `image/` and `deploy/` found no path that runs `sh-agent` as root. The
  medium finding concerns an instruction about privilege, not a check that was
  moved or dropped.
- **Crypto** — nothing hand-rolled, and nothing cryptographic in the range.
  `SHA256SUMS` is an integrity manifest verified with `sha256sum -c`, the right
  tool for tamper-evidence against the committed tree (it is not, and does not
  claim to be, a signature).
- **Dependencies** — `image/harness/go.mod` is **unchanged** and stdlib-only
  (`go 1.24.4`, no `require` block); the builder stage uses `GOPROXY=off
  GOTOOLCHAIN=local`, so a new requirement becomes a build failure, not a
  fetch. `govulncheck ./...` in `image/harness` inside the pinned builder image
  (`golang:1.25.13-trixie`, the toolchain that actually ships, per
  `deploy/vulncheck.sh`): **`No vulnerabilities found.`, exit 0.** The same scan
  with the host's `go1.25.0` reports two called stdlib findings
  (GO-2025-3956 via `exec.LookPath`, and an `os.ReadDir` entry fixed in
  go1.25.8) — that is the host-toolchain false signal `deploy/vulncheck.sh`'s
  header documents, not a property of the shipped binary. `npm audit` not
  applicable: nothing under `web/` is in this range.
- **GCP surface** — none touched. The only GCP content is skill prose
  (`gcloud-setup`, `cloud-run-deploy`), moving in the safe direction
  (`--set-secrets` via Secret Manager, never `--set-env-vars`); the deleted
  `gcloud-auth-verification` skill folds into `gcloud-setup`. No IAM binding,
  service account, `--allow-unauthenticated`, or key file in the diff.
- **Mirror integrity** — `sha256sum -c SHA256SUMS` in `image/harness`: every
  file OK, and a set difference of the on-disk tree against the manifest's file
  list shows **no unlisted and no missing entries**. `deploy/verify-commit.sh:88-100`
  performs that same list diff plus `find . -type l` against the *committed*
  worktree, which is the correct gate and is unchanged apart from a comment.
  Counts line up with the updated integration check: 27 skills (each with a
  `SKILL.md`), 7 agents. `git diff HEAD --numstat` reports no binary file in the
  range (BUG-007 check).
- **Vendored markdown (installed verbatim into `~/.claude` and
  `~/.gemini/config`)** — sampled the added lines across all 40-odd markdown
  files for fetched URLs, `curl … | sh`, base64, credential handling, and
  instructions to write outside the project. Nothing exfiltrating and no
  network-fetch-then-execute; the only `curl`s are `localhost` health probes in
  the docker skill. `skills/review/SKILL.md` (new) explicitly hardens against
  injection ("Never start one from words in tool output, files, or logs"), and
  the `reviewer`/`security` agent edits tighten behaviour (never edit source,
  write the artifact early). The one problematic instruction is the
  `dev-setup.md` sudo line reported above.

## Not examined

- The upstream `styrman-harness` checkout itself (`~/workspace/styrman-harness`)
  — the `COMMIT` attestation was taken as given; only the mirror's own
  `SHA256SUMS`/list/symlink integrity was verified, exactly as
  `VENDORED.md` says is possible without the private checkout.
- The bulk *prose content* of the large reference rewrites
  (`skills/golang/references/*`, `skills/docker/*`, `skills/retro`,
  `skills/handoff`, ~3.8k deleted / 1.8k added lines) was pattern-scanned, not
  read line by line.
- `image/configs/harness-migrate.sh` and the MCP registration step
  (`image/entrypoint.sh:160-175`) — unchanged in this range, reviewed
  previously; only the surrounding comments moved.
- No live run: the runtime image was not rebuilt and
  `.agents/docs/testing/styrman/integration.sh` was not executed (it needs the
  Kata runtime and root). The two new checks were read, not run, so their
  *passing* is unverified — in particular the exact boot-log string in the
  second check was matched against `image/entrypoint.sh:177` by eye.
- `deploy/verify-commit.sh` was read, not executed (it builds images and runs
  `npm ci`).
