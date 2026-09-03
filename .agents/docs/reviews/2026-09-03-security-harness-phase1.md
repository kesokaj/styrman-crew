---
owner: security
date: 2026-09-03
---
# Security review — harness phase 1 (vendored styrman-harness mirror + sh-agent builder stage)   (2026-09-03)

Range reviewed: c1a6eb9..working tree, limited to `image/Dockerfile`, the new
`image/harness/` mirror (151 files, all untracked: `context/ skills/ agents/
commands/ rules/ cmd/sh-agent internal/harness go.mod COMMIT VENDORED.md` plus the
repo's own README/AGENTS/TODO/ROADMAP/REQUIREMENTS) and the deletion of the previous
`image/harness/{claude,antigravity,shared}` tree. Out of scope by instruction and not
assessed: `image/entrypoint.sh` and `image/configs/harness-migrate.sh` (phase 2) — they
are cited only as the call site that determines the blast radius.

## Verdict: FINDINGS

Nothing in this change leaks a secret, ships a vulnerable dependency, or hands a tenant
a capability they do not already have; the worst problem is process, not code. The
release gates do not know that a third Go module and a second Docker image now exist:
`image/harness` is a separate module, so the root `go build/vet/test ./...` never
reaches it (`go list ./...` returns nothing under it), `deploy/vulncheck.sh:35` scans
only `.` and `deploy/caddy/ingress`, and `deploy/verify-commit.sh` builds only the
server image — so the one gate written specifically to catch "a file that exists in the
working tree but not in the commit" cannot see 151 currently-untracked files that
compile into a binary shipped to every project. A partial `git add` either breaks the
runtime image build at release time or silently seeds an incomplete harness corpus into
every tenant home, and the gate says OK either way. Everything else is low: the
installer parses names out of markdown frontmatter and paths out of a user-writable
manifest and turns both into filesystem paths without a `filepath.IsLocal` check —
harmless today only because the writer, the attacker, and the microVM's root are the
same principal.

## Findings

- **[medium / confidence: high]** `deploy/verify-commit.sh:35-42,76-78`, `deploy/vulncheck.sh:35` — the release gates ignore the new module and the runtime image. `verify-commit.sh` runs `go build/vet/test ./...` at the root (which excludes the nested module — verified with `go list ./...`), then the ingress module explicitly, then builds *only* `deploy/Dockerfile.server`. `image/Dockerfile` — now the only thing that compiles `image/harness/cmd/sh-agent` — is never built from the detached worktree, and `image/harness` is never vetted, tested, or vulnerability-scanned. Exploitable in principle as a supply-chain integrity gap: the tag-time gate reports OK for a commit whose runtime image cannot build, or which ships a corpus missing whichever `.md` files were left unstaged, and the `STYRMAN_VERSION` bump then re-seeds that partial corpus into every project home. Resolve: add `( cd "${WT}/image/harness" && go vet ./... && go test ./... )` and a `docker build -f image/Dockerfile … image/` (at minimum `--target harness-builder`) to `verify-commit.sh`, extend its `gofmt -l` list to `image/harness`, and add `image/harness` to the `for mod in` loop in `vulncheck.sh`. Run by hand for this review: `go vet` clean, `go test ./...` ok, `govulncheck ./...` inside `golang:1.25.13-trixie` → "No vulnerabilities found".
- **[medium / confidence: medium]** `image/harness/COMMIT:1`, `image/harness/VENDORED.md:20-29` — the mirror's provenance is asserted, never verified. `COMMIT` names `6893e2218414617da2bb88259b905ca3882b2659`, but nothing in the repo can recompute whether the 151 mirrored files *are* that commit: the refresh is a hand-run `rsync -a --delete` from a local checkout, the `diff -rq` in the procedure is local and unrecorded, no checksum manifest is committed, and the procedure never asserts the source worktree was clean — so a dirty checkout (or one tampered with) mirrors silently under a clean-looking commit id, and the id then *looks* like provenance in `versioning.md`. The mirrored content is prompt material and a binary that run in every tenant's home, so a bad mirror is agent-behaviour-level compromise. I verified today's mirror is faithful (source HEAD == `COMMIT`, `git status --porcelain` empty, `diff -rq` reports only `COMMIT` and `VENDORED.md`), so this is a gap in the process, not a live compromise. Resolve: make the refresh refuse a dirty source worktree, commit a `SHA256SUMS` (or the `git archive <COMMIT> | sha256sum` digest) beside `COMMIT`, and have `verify-commit.sh` recompute and compare it.
- **[low / confidence: high]** `image/harness/internal/harness/plan.go:85-86,101-102` and `apply.go:59,201` — names and manifest entries become filesystem paths with no containment check. The destination is `filepath.Join(ClaudeRoot, "skills", s.Name, rel)` / `filepath.Join(ClaudeRoot, "agents", a.Name+".md")`, where `Name` is whatever the `name:` key of a `SKILL.md`/agent file says (`inventory.go:96-99`, `:134-137`) — no `filepath.IsLocal`, no separator rejection — and the resync/uninstall loops `os.Remove(filepath.Join(home, rel))` over `rel` strings read straight from `~/.config/sh-agent/state.json` (`state.go:29-40`), a file inside the home the tenant owns. Either input containing `../` walks out of the two vendor roots. It crosses no boundary as shipped: the entrypoint invokes install as the project user (`image/entrypoint.sh:130`, `runuser -u`), that user already holds NOPASSWD sudo in its own microVM (`image/entrypoint.sh:55`), and the only host paths bind-mounted into the guest are that project's own home and workspace (`internal/orchestrator/orchestrator.go:782-789`) — so writer and attacker are the same principal and nothing else is in reach. It is still an unvalidated parse→path flow in code that ships to every tenant, and the tests (`harness_test.go`) cover no hostile name. Resolve: reject any `name` or manifest entry where `!filepath.IsLocal(x)`, exactly as `internal/credstore/credstore.go:430-444` already does for broker file credentials.
- **[low / confidence: high]** `image/Dockerfile:11`, `image/harness/internal/harness/apply.go:75`, `VENDORED.md:33-38` — the deployed harness carries no machine-checkable version. `-trimpath` in a context with no `.git` makes `sh-agent version` print `(devel)` (verified in `styrman-runtime:test`), and `Install` records `commit: ""` because `gitOut(repo, "rev-parse", …)` fails against the `.git`-less mirror; `sh-agent status` inside a project therefore shows a blank commit. An operator investigating a project cannot tell which harness it is running except by trusting a text file the same image ships. Resolve: `ARG HARNESS_COMMIT` fed from `harness/COMMIT`, injected with `-ldflags "-X main.commit=…"`, printed by `version` and stored in the manifest.
- **[low / confidence: high]** `image/Dockerfile:8-11` — hermeticity is incidental rather than enforced. The build stage has default network access and pins neither `GOFLAGS` nor `GOPROXY`; it needs neither today (the module has zero `require`s — `go list -deps ./cmd/sh-agent` yields only the two internal packages, and I reproduced the exact build line with `GOPROXY=off`, exit 0), and `GOTOOLCHAIN=local` correctly turns a future `go`-directive bump into a build error instead of a toolchain download. But the day someone adds a dependency, the image build will quietly fetch it from the public proxy with no `go.sum` in the tree to check it against. Resolve: `ENV GOFLAGS=-mod=readonly GOPROXY=off` on the builder stage (or `RUN --network=none`), so a new requirement fails loudly.
- **[low / confidence: high]** `image/Dockerfile:8` vs `deploy/Dockerfile.server:9` — the toolchain pin is duplicated and unasserted. Both say `golang:1.25.13-trixie` today, but `deploy/vulncheck.sh:19` derives the image it scans by `sed`-ing *only* `Dockerfile.server`; a bump there leaves the harness builder on an older, now-unscanned toolchain while the comment in `image/Dockerfile:5-7` still claims they match. Resolve: assert the two `FROM golang:` lines are equal in `verify-commit.sh` (one `grep`/`test` line), or read the tag for the harness stage from the same single source.
- **[low / confidence: high]** missing `image/.dockerignore` — the runtime image builds with context `image/` (`AGENTS.md:42`), so the repo-root `.dockerignore` (which excludes `.git`, `**/.env`, `*.env`) does not apply to it, and there is no per-context replacement. Today the mirror is clean; the failure mode is one refresh run without `--exclude .git`, which would copy the private harness repo's `.git` — remote URLs and anything in `.git/config` — into `/styrman/harness/.git` inside every tenant's microVM, where every project user can read it. Resolve: add `image/.dockerignore` with at least `**/.git`, `**/.env`, `*.env`, `*.pem`, `*.key`.
- **[low / confidence: high]** `image/Dockerfile:114` (`COPY harness/ /styrman/harness/`) — the seeded corpus lands group-writable. `COPY` preserves the operator's host modes, so in `styrman-runtime:test` the tree is `-rw-rw-r--` files and `drwxrwxr-x` directories owned `root:root` (the binary at `/styrman/bin/sh-agent` is correctly `-rwxr-xr-x root:root`). Group 0 has no members in the guest, so this is inert as built; it is a latent write path if anything later runs group-0 non-root. Resolve: `COPY --chown=root:root --chmod=644` (or a `chmod -R go-w /styrman/harness` in the existing `RUN`).
- **[low / confidence: medium]** removal of the vendor `settings.json` (deleted `image/harness/claude/settings.json`; `plan.go:55-59` plans no vendor settings) — the seeded home loses the `permissions.ask` list that previously required confirmation for `Bash(rm:*)`, `rmdir`, `unlink`, `shred`, `trash`, and `git clean`. Stated honestly: this is a safety rail, not an isolation boundary — the tenant is root inside their own microVM — and the same change also removes the four hook scripts and the statusline command the old harness auto-executed on SessionStart / UserPromptSubmit / PostToolUse / Stop, which is a clear net reduction in what runs unattended in a tenant home. Confidence is medium because a home with no `settings.json` falls back to the vendor's default permission mode, which prompts for Bash more broadly than the old `ask` list did, so the net effect may well be stricter. Resolve: record the posture change (and the reason it is acceptable) in the ADR rather than leaving it implicit in "no settings.json (operator directive)".
- **[low / confidence: high]** `image/harness/internal/harness/repo.go:17-36,65-74,92-103` — `update` ships in the tenant binary and is a pull-and-execute verb. It is not on the boot path (`image/entrypoint.sh:130` calls only `install --repo /styrman/harness --home …`) and is inert against the mirror (no `.git`, so `Behind`'s `git fetch` errors out and `update` exits non-zero). But `install` copies the binary to `~/.local/bin/sh-agent` (`plan.go:124`) and `FindRepo` accepts, without `--repo`, the first directory from the cwd upward that merely contains `context/AGENTS.md` and a `skills/` dir — so `sh-agent update` inside an unrelated checkout will `git pull --ff-only` and then `go run ./cmd/sh-agent install` from *that* tree. It grants no capability the tenant's shell does not already have, which is why this is low and not medium; it does mean a prompt-injected agent has a short, innocuous-looking path to executing fetched code. Resolve: require an explicit `--repo` for `update`, or build the image binary with the `update`/`adopt` verbs disabled behind a build tag.
- **[low / confidence: low]** corpus hygiene — `image/harness/skills/terraform/references/ci-cd-workflows.md:48` and `skills/terraform/references/security-compliance.md:42` are `curl … | bash` / `| sh` installer one-liners, and `image/harness/README.md:8` ships the private repo's clone URL (`git@github.com:kesokaj/styrman-harness.git`) and the operator's `~/workspace` layout into every tenant VM. Neither is a secret; both are prompt material that models imitate and mild operator-infrastructure disclosure in a multi-tenant image. Resolve if cheap: replace the pipe-to-shell examples with a download-verify-run form, and drop `README.md`/`ROADMAP.md`/`TODO.md` from what `COPY harness/` ships (the installer needs `context/ skills/ agents/ commands/ rules/` only).

## Checked

- **Secrets** — 151 mirrored files grepped for `sk-`, `AKIA`, `ghp_`/`gho_`, `xox[baprs]-`, `AIza`, `eyJhbGciOi`, and PEM headers: the only hit is AWS's published documentation placeholder `AKIAIOSFODNN7EXAMPLE` (`skills/terraform/references/security-compliance.md:291`). No `.env`, no key material, no credential-shaped string, no private hostname or IP (only RFC1918 examples in Terraform docs); no operator email; the only operator-machine leakage is the public GitHub handle already in the module path and the `~/workspace/...` layout in `README.md`/`VENDORED.md`. Nothing in the corpus instructs an agent to post, upload, or webhook data anywhere — the only exfil-shaped grep hits are documentation URLs and the two installer pipes above. No secrets echoed by `sh-agent`: it prints paths, counts, and git output only.
- **Injection** — the parsing boundary is markdown frontmatter (`frontmatter.go`, a hand-rolled lenient reader) and the JSON manifest. The frontmatter reader is regex-bounded, allocates nothing unbounded, and cannot execute; its output is a `map[string]string`. The exposure is not injection but the unvalidated flow of `name` and manifest `rel` into `filepath.Join` (finding 3). Shell-outs are `exec.Command` with argument vectors — never a shell string — for `git -C <repo> …` (`repo.go:105-116`), `diff -u …` (`apply.go:173`), and `go run …` (`repo.go:96`); `git` failures are swallowed into empty fields by design (`Inspect`), `diff`'s exit 1 is correctly distinguished from a real failure, and Go's `exec.LookPath` refuses cwd resolution, so no PATH-in-cwd hijack. `writeAtomic` (`apply.go:228-256`) creates 0600 then chmods then renames, so no umask window; `rename(2)` replaces a final-component symlink rather than following it. It does `MkdirAll` through intermediate symlinks, which the home's owner could point elsewhere — same-principal only, per finding 3's reasoning.
- **Authz/authn** — none added. `sh-agent` has no network surface, no listener, no credential handling; it never reads or writes the broker's paths (planned set is `.claude/**`, `.gemini/config/**`, `.local/bin/sh-agent`, `.config/sh-agent/state.json`, and stale removal touches only prior-manifest entries), so credential materialization is untouched. Credentials remain per-user in `credstore` (`GetGit(user, host)`, `userDir(user)`), so the seeded GCP/git skills cannot reach another tenant's credentials; the skill and agent roster is materially the same as the old harness (`bug`/`feature`/`bugs` replace `grill-me`/`skill-repair`), so no new capability class is introduced into tenant prompts.
- **Crypto** — none. No hashing, signing, token handling, session, or JWT/OIDC code anywhere in the new module; nothing hand-rolled. Note the inverse: the absence of any digest over the mirror is finding 2.
- **Dependencies** — `image/harness/go.mod` declares `module github.com/kesokaj/styrman-harness`, `go 1.24.4`, and **zero** requires; there is no `go.sum` because there is nothing to check. `go list -f '{{join .Deps "\n"}}' ./cmd/sh-agent` shows stdlib plus the single internal package — stdlib-first satisfied, nothing to justify. `govulncheck ./...` for this module inside the pinned `golang:1.25.13-trixie` (the same image `deploy/vulncheck.sh` uses, `golang.org/x/vuln/cmd/govulncheck@v1.7.0`): **No vulnerabilities found**, exit 0 — run manually because the script does not yet cover the module (finding 1). `go vet ./...` clean; `go test ./...` ok (`internal/harness` 0.133s). No npm surface in this diff, so `npm audit` was not applicable.
- **GCP surface** — none touched. No IAM binding, service account, key file, `--allow-unauthenticated`, or Secret Manager change; styrman is self-hosted (`.agents/docs/deploy/gcp.md`: not applicable). The corpus's GCP guidance is advice to agents, and it already tells them secrets reach Cloud Run via `--set-secrets`, never `--set-env-vars` (`context/dev-setup.md`).

---

## Round 2 (2026-09-03)

Range reviewed: `c1a6eb9..working tree` (HEAD unchanged since round 1), re-reading
`deploy/verify-commit.sh`, `deploy/vulncheck.sh`, `image/Dockerfile`, the new
`image/.dockerignore`, `image/harness/{SHA256SUMS,VENDORED.md,COMMIT}`, and
`.agents/docs/architecture/DECISIONS.md` (ADR-062). Re-verified against the built
`styrman-runtime:dev` and the private upstream checkout.

### Verdict: FINDINGS (both mediums closed or downgraded; 3 lows still open, 2 new lows)

The two mediums are gone as filed. The gate now sees the third module and the second
image, and the mirror is checkable from the commit alone. Provenance is no longer
unverifiable — only *incompletely* verifiable, and in a direction I can name precisely:
`sha256sum -c` proves nothing about files the manifest does not list.

### Per finding

1. **[medium] gates ignore the third module and the runtime image — CLOSED.**
   `verify-commit.sh:53-61` runs `sha256sum -c --quiet SHA256SUMS` and `go vet ./...`
   in `image/harness` inside the detached worktree, asserts the two Go pins equal
   (`:55-60`), and `:99-113` builds `image/` from the commit and compares
   `sh-agent list`'s counts against the tree. `vulncheck.sh:35` now loops
   `. deploy/caddy/ingress image/harness`. I read the discovery code to confirm the
   count assertion is sound rather than coincidental: `inventory.go:66-101` takes
   skills as `skills/<name>/SKILL.md` and `:114-136` agents as `agents/*.md`, exactly
   what the gate's two `find`s count; a `SKILL.md` with no description makes
   `list` error, the greps miss, and the gate FAILs — fail-closed. Every empty-value
   path (`srv_go` empty, `find` failing under `pipefail`, docker down in the
   `listed=$(…)` substitution) also fails closed. Residual, not worth a finding of its
   own: the gate runs `go vet` but not `go test` for that module, and `gofmt -l`
   (`:86`) still lists only `internal/ cmd/`.
2. **[medium] COMMIT provenance unverifiable — DOWNGRADED to low, still open;
   the upstream-equality half of the rebuttal ACCEPTED.**
   The half that mattered most is closed and I proved it: in a scratch copy, removing
   one mirrored file makes `sha256sum -c --quiet` exit 1, so the partially-staged
   refresh this gate exists for now fails at the gate. `VENDORED.md:23` asserts a
   clean source checkout first. Re-verified today, this machine:
   `git -C ~/workspace/styrman-harness rev-parse HEAD` = `6893e221…` = `COMMIT`,
   `status --porcelain` empty, `diff -rq` against the mirror reports exactly
   `COMMIT`, `SHA256SUMS`, `VENDORED.md`. The mirror is faithful. What remains open
   is narrower and demonstrated: **`sha256sum -c` is blind to additions.** Dropping
   `skills/EXTRA-NOTE.md` into a scratch copy of the tree left `sha256sum -c --quiet`
   at **exit 0** — so an extra file added after a refresh (or a `git add` of a file the
   committed manifest predates) passes the gate and is `COPY`ed into `/styrman/harness`
   and seeded as prompt material into every tenant home. `find . -type f` also excludes
   symlinks, so a symlink added to the mirror is in neither the manifest nor the check,
   and `sh-agent install` reads through it (`os.ReadFile`). The manifest is
   additionally self-attesting — nothing signs it. **[low / confidence: high]**
   Resolve in one line at `verify-commit.sh:54`: compare the file list too, e.g.
   `diff <(find . \! -name SHA256SUMS \( -type f -o -type l \) | sed 's#^\./##' | LC_ALL=C sort) <(cut -d' ' -f3- SHA256SUMS)`.
3. **[low] no `filepath.IsLocal` in the installer — rebuttal ACCEPTED on the merits,
   finding STILL OPEN as a record.** Styrman does not edit the mirror; the fix belongs
   upstream. But the rebuttal's own remedy is not in either tree: neither
   `TODO.md` (its only change this session is ticking the settings-roster item) nor
   `image/harness/TODO.md` / `~/workspace/styrman-harness/TODO.md` at `COMMIT` carries
   an entry for it. Deferred-with-no-record is how a finding evaporates. Same for
   findings 4 and 10 below.
4. **[low] `sh-agent version` is `(devel)`, manifest commit empty — rebuttal ACCEPTED,
   documented.** Confirmed in `styrman-runtime:dev`: `sh-agent version` → `sh-agent (devel)`.
   `VENDORED.md:44-49` ("Known cosmetic gap") and ADR-062's Consequences both state it,
   which is the record I asked for. **New, adjacent [low / confidence: high]:** the
   empty commit is not the only outcome — `gitOut(repo, "rev-parse", "--short", "HEAD")`
   (`repo.go:59,105`) lets git *ascend* out of `--repo` when it holds no `.git`. Run
   against this repo's `image/harness` it returned `c1a6eb9` — styrman-crew's HEAD,
   recorded as the harness commit. Harmless in the image (`/styrman` and `/` are not
   repos and are root-owned), but on the operator's own machine the manifest's `commit`
   field is not merely blank, it can be confidently wrong. Upstream: pass
   `GIT_CEILING_DIRECTORIES` / `--git-dir`, or fall back to the repo's `COMMIT` file.
5. **[low] hermeticity incidental — CLOSED.** `image/Dockerfile:13` builds with
   `GOPROXY=off` alongside `GOTOOLCHAIN=local`; a new requirement is now a build failure.
6. **[low] duplicated toolchain pin unasserted — CLOSED.** `verify-commit.sh:55-60`;
   both `sed`s yield `golang:1.25.13-trixie` today, and an unparseable `FROM` line
   (lowercase `as`, a `--platform=` flag) yields an empty `srv_go`, which the explicit
   `-z` test turns into a FAIL rather than a silent pass. No new failure mode.
7. **[low] missing `image/.dockerignore` — CLOSED.** Present, covering `.git`,
   `**/.git`, `*.env`, `**/.env`, `**/*.pem`, `**/*.key`. Checked the inverse risk too:
   nothing in the `image/` context matches those patterns, so no needed file is silently
   dropped from the image (which the SHA256SUMS check, run in the worktree, would not
   have caught).
8. **[low] corpus group-writable — CLOSED.** In `styrman-runtime:dev`:
   `/styrman/harness` is `755 root:root`, its files `644`, and
   `find /styrman/harness -perm -g+w -o -perm -o+w` returns nothing.
9. **[low] vendor `settings.json` rail lost — rebuttal ACCEPTED, CLOSED.** The remedy I
   asked for was the record, and ADR-062's Consequences now carries it verbatim
   ("Claude Code inside a project runs in its default permission mode…"), with the
   operator's 2026-09-03 directive quoted in the Context.
10. **[low] `update` verb in the tenant binary — rebuttal ACCEPTED, open-deferred.**
    Confirmed the boot path calls only `install` (`entrypoint.sh:140-142`) and that the
    mirror has no `.git`. No upstream record exists (see 3).
11. **[low] corpus hygiene — rebuttal ACCEPTED, closed as won't-fix.** The verbatim-mirror
    argument is now stronger than it was in round 1: `SHA256SUMS` plus the `diff -rq`
    only stay trivial while the mirror is byte-identical to upstream, so local edits
    would cost more than they buy. The two `curl | sh` lines and the clone URL stay
    upstream's call.

### New code paths examined this round

- **`timeout` + `runuser`.** GNU `timeout` without `--foreground` puts the child in its
  own process group and signals the *group*; verified locally that a backgrounded
  grandchild dies with it, so the `runuser` child is reached. Both binaries exist in the
  shipped image (`/usr/bin/timeout`, `/usr/sbin/runuser`). See the phase-2 append for
  the boot-budget consequence.
- **`SHA256SUMS` failure modes.** Missing file → exit 1 (good); added file → exit 0
  (finding 2 above); manifest absent from a commit → the subshell fails → gate FAILs.
  No file names with spaces or newlines and no symlinks in today's 151-file mirror, so
  the `xargs sha256sum` refresh line is safe as it stands.

### Checked (round 2)

- **Secrets** — no new secret-shaped content: the mirror is byte-identical to round 1
  (`COMMIT` unchanged, `diff -rq` clean against upstream). `image/.dockerignore` now
  keeps `.git`/`.env`/`.pem`/`.key` out of the runtime build context.
- **Injection** — no new shell string. The two `sed` pin extractions and the
  `grep -q "Skills (N)"` comparison are literal; `(`/`)` are not BRE metacharacters, so
  `Skills (280)` cannot satisfy a `Skills (28)` probe.
- **Authz/authn** — unchanged; nothing in this delta touches identity, the ingress
  token, or the broker.
- **Crypto** — SHA-256 via coreutils `sha256sum` for integrity only, no hand-rolled
  construction. The manifest is unsigned by design (it is an integrity check against
  the commit, not an authenticity check against the operator).
- **Dependencies** — `deploy/vulncheck.sh` run in full this round, all three modules
  inside `golang:1.25.13-trixie`: `image/harness` → **No vulnerabilities found**; `.`
  and `deploy/caddy/ingress` → the two known docker/docker findings GO-2026-4887 /
  GO-2026-4883 (no fix available, accepted per release); script exit 3 per its
  documented semantics. `image/harness/go.mod` still declares zero requires.
- **GCP surface** — none touched (self-hosted).
