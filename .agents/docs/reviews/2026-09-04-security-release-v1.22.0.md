---
owner: security
date: 2026-09-04
---
# Security review — release v1.22.0 (ui+git improvements, phases 1–6)   (2026-09-04)

Range reviewed: v1.21.0..acb5918

Blast radius followed outward from the diff: `internal/httpapi/git.go` (all
handlers, not only the new six) → `gitContext` → `getProjectFor` → `who` →
`authGuard`/`csrfGuard` (`internal/httpapi/{server,auth}.go`) → `gitExec` →
`orchestrator.Exec`; the guest-writable input those handlers parse (the
workspace's own `.git/config`, refs, `HEAD`, and the guest's `~/.gitconfig`);
the host-side `gitIdentity` read of `HomesDir/<user>/base/.gitconfig`; the
credential broker path the git verbs can trigger
(`image/configs/styrman-git-credential`, `styrman-credentials.sh`,
`internal/credbroker`); the seeded skel (`image/skel/*`, `image/entrypoint.sh`
write-once copy loop, `image/configs/harness-migrate.sh`); the new npm package
and the committed, `go:embed`-ed `web/dist/client` artifact; and the SPA render
path `SidePanelGit.tsx` → `editorPane.ts` → `DiffPane.tsx` → `DiffView.tsx`.

## Verdict: FINDINGS

**First, the thing this review was asked to settle: every phase-5 and phase-6
fix is genuinely present in the committed tree at `acb5918`.** Verified by
reading the committed file, not the plan: `checkout --end-of-options <name>`
on the non-create path (`git.go:578`, with `-b` still used for create, which is
correct because `-b` consumes its argument as a value); `git fetch --all` with
**no** `--prune` (`git.go:472`) and a comment recording why; `guestErrMessage`
in `gitRun` (`git.go:728`) and on the `add -A` failure path (`git.go:318`);
`utf8.ValidString` rejection *before* the rune loop in `validBranchName`
(`git.go:769`); and `GIT_CEILING_DIRECTORIES=<parent of ws>` plus
`GIT_DISCOVERY_ACROSS_FILESYSTEM=0` on every `gitExec` (`git.go:108-109`). The
phase-5 `reveal()` sanitiser is not only in source but in the **shipped
artifact** — `web/dist/client/assets/p._project._term-DQfho7jD.js` carries the
byte-for-byte regex and `<U+…>` replacement, so the committed `dist` is not
stale with respect to the fix.

What a per-phase review could not see is the **sibling that did not get the
fix**. `gitPush` takes the current branch name straight out of the guest
(`git rev-parse --abbrev-ref HEAD`) and puts it into `git push -u origin <br>`
argv with no validation at all, while its sibling `gitCheckout` — reviewed in
phase 6 — gates the identical class of string with `validBranchName`. I
verified on git 2.47.3 that a guest can plant `refs/heads/-foo` with plumbing
(`update-ref` + `symbolic-ref`; porcelain refuses it) and that
`git push -u origin -foo` then consumes the operand as **options**, not as a
refspec: git reports "The current branch -foo has no upstream branch" (the
refspec is gone) and, against a receiver, "fatal: the receiving end does not
support push options" — i.e. `-f` and `-o o` were parsed. The operator's Push
button becomes a force-push whose refspec is decided by guest-writable config.
Second worst, and unrelated: two new source files contain literal NUL bytes and
are therefore **binary to git**, so their contents are invisible in every
`git diff` — including the diff this project's own Definition of Done hands to
`reviewer` and `security`.

## Findings

- **[medium / confidence: high]** `internal/httpapi/git.go:365-368` (`gitPush`)
  — the branch name is read from the guest (`rev-parse --abbrev-ref HEAD`),
  `TrimSpace`d, and appended to `["push","-u","origin", …]` **with no
  validation**, while the sibling `gitCheckout` runs the same class of string
  through `validBranchName` first. Verified against git 2.47.3 in a throwaway
  local repo: `git branch -- -foo` and `git checkout -b -foo` are refused
  ("not a valid branch name"), but `git update-ref refs/heads/-foo HEAD`
  followed by `git symbolic-ref HEAD refs/heads/-foo` succeeds, and
  `git rev-parse --abbrev-ref HEAD` then prints `-foo`. Feeding that to
  `git push -u origin -foo` makes git parse the operand as short options — the
  refspec vanishes ("The current branch -foo has no upstream branch") and
  against a real receiver git emits "fatal: the receiving end does not support
  push options", proving `-f` (force) and `-o o` (push option) were both
  consumed. Refnames also admit `=`, so the whole long-option surface of
  `git push` (`--repo=`, `--exec=`, `--receive-pack=`) is reachable in
  principle by the same route. Exploitable in principle by the realistic actor
  in this platform — a prompt-injected agent with a shell in the project VM —
  whose payoff is turning the operator's un-confirmed Push click into a forced
  update against the remote, authenticated with the broker's credentials, which
  are shared across every one of that user's projects. Two caveats that keep
  this at medium rather than high: the dash path is only taken when there is no
  upstream, and a guest that can write `.git/config` can already make a plain
  `git push` force-push via `remote.origin.push = +refs/heads/*:refs/heads/*`,
  so the argv route is not the only one. It is, however, the one a code guard
  can close. **Remediation:** reject the value with the existing
  `validBranchName(br)` before it reaches argv (400/409 with a clear message),
  or push the unambiguous form `"HEAD:refs/heads/" + br`, which cannot lead
  with a dash. Do **not** reach for `--end-of-options` here: I verified
  `git push --end-of-options -u origin -foo` is mis-parsed
  (`error: failed to push some refs to '-u'`) — unlike `checkout`, `push` has
  no such primitive.

- **[medium / confidence: high]** `web/src/components/DiffPane.tsx` and
  `web/src/lib/diffReveal.test.ts` — both files contain **literal NUL bytes**
  (`DiffPane.tsx` has three, at offsets 1330/1351/1368, used as a separator in
  the cache-key template literal; the test file has NUL/BEL/ESC/DEL as
  control-character fixtures). Git's binary heuristic is "a NUL in the first
  8000 bytes", so `git diff --stat` reports both as `Bin 0 -> N bytes` and
  their contents never appear in a diff — now or in any future change. That is
  not a runtime vulnerability; it is the quiet defeat of a control this repo
  relies on: `AGENTS.md` routes review through `reviewer`/`security` "given the
  diff only", and `.agents/rules/writing-files.md` (BUG-005) exists precisely
  because a change nobody can see in a diff is a change nobody reviews. Two
  files are now permanently exempt, one of which is the **test that guards the
  Trojan-Source sanitiser** — the BUG-005 failure mode ("a deleted test cannot
  fail") with the diff evidence removed as well. **Remediation:** write the NUL
  as a source escape rather than a raw byte (`"\u0000"` in the template, or
  `String.fromCharCode(0)`), which leaves the runtime value identical and makes
  both files plain UTF-8 text again; confirm with `git diff --numstat` showing
  line counts instead of `-`. Optionally add a `verify-commit.sh` step that
  fails when any file under `web/src/` or `internal/` is classified binary.

- **[low / confidence: high]** `internal/httpapi/git.go:339` (`gitCommit`) and
  `:371` (`gitPush`) — the phase-5 finding "switch git.go's four `firstLine(out)`
  call sites to `guestErrMessage`" was applied to **two of four**. `gitRun` and
  the `add -A` failure path now redact; the commit-failure and push-failure
  paths still answer with `errors.New(firstLine(out))`, which applies no
  `stripControls`, no `redactSecrets` and no length cap. `gitPush` is the single
  handler in the file whose first error line is most likely to carry a remote
  URL with userinfo and remote-authored sideband text, and it is the one still
  unhardened — the exact case the phase-5 review named. It lands in a toast, so
  the realistic payoff is UI spoofing with control/bidi bytes plus an unbounded
  message; the credential channel is narrower than it looks because git 2.47.3
  strips userinfo from its own `unable to access` line. **Remediation:**
  `writeErr(w, …, errors.New(guestErrMessage("git push failed", out)))` and the
  same for commit, or fold `redactSecrets(stripControls(...))` and the cap into
  `firstLine` itself so no call site can be missed again.

- **[low / confidence: high]** `web/src/components/DiffView.tsx:10` — the new
  `INVISIBLE` class
  (`\u0000-\u0008\u000b-\u001f\u007f-؜‎‏‪-‮⁦-⁩`)
  is narrower than the server's `stripControls`
  (`internal/httpapi/server.go:1215`, which drops all of Cc **and all of Cf**).
  Not covered: the zero-width space/non-joiner/joiner `U+200B-‍`, word
  joiner `U+2060`, BOM `U+FEFF`, the interlinear annotation controls
  `U+FFF9-￻`, and `U+2028/U+2029` — the last of which CSS treats as a
  forced line break under `white-space: pre`, so a single character can push
  diff content onto a visually unprefixed, uncoloured line in the pane whose
  whole job is pre-commit review. The main Trojan-Source vectors (bidi
  overrides and isolates) *are* covered, so this is a residual, not a hole.
  **Remediation:** widen to the same categories the server uses — e.g. match
  `\p{Cc}` and `\p{Cf}` with the `u` flag, plus `  `, keeping the
  existing TAB exemption — and extend `diffReveal.test.ts` with a case per
  newly covered class.

- **[low / confidence: medium]** `internal/httpapi/git.go:449-451` — the
  untracked branch runs `git diff --no-color --no-index -- /dev/null <path>`
  with `path` taken verbatim from the query string and never validated.
  `--no-index` operates on the filesystem, not the repository, so an absolute
  path is accepted: I confirmed on git 2.47.3 that
  `git diff --no-index -- /dev/null /abs/outside/file` prints the file's full
  contents as an addition. Inside the guest this reads anything the guest uid
  can read (the broker-materialised `~/.claude/.credentials.json`, `~/.ssh/`,
  `~/.config/gcloud/`, `/etc/*`), and a directory argument makes it recurse.
  **No trust boundary is crossed** — the caller is the project's owner, who has
  a root-capable shell in that VM and can read the same bytes through the
  terminal — which is why this is low. What it does break is an invariant the
  rest of the server holds carefully: every other server-mediated read is
  confined by `internal/files`' `os.OpenRoot` roots to `ProjectHome`/
  `ProjectWorkspace` (`explorer.go:34-51`), and this endpoint is a same-origin
  path around that confinement with no path check at all. Pre-existing (the
  branch dates to v1.7.0) but re-touched in this range by phase 5.
  **Remediation:** reject an absolute `path` and any `..` component before the
  exec, and/or resolve it relative to `<ws>` explicitly — the UI only ever
  sends workspace-relative paths.

## Checked

- **Secrets** — clean. Scanned every added line in the range excluding
  `web/dist` for `api[_-]?key|secret|token|password|private[_-]?key|BEGIN …
  PRIVATE|Bearer|AKIA|ghp_|github_pat_|sk-ant-|AIzaSy|xox[baprs]-`: every hit
  is prose inside `.agents/docs/**` (the phase reviews and changelogs
  discussing the credential broker). No `.env`, key, or credential file added;
  `go.mod`/`go.sum` untouched. The rebuilt `web/dist/client` tree carries no
  token-shaped literal. No new logging statement in any handler, and the two
  error paths that echo guest text are covered by finding 3.
- **Injection** — the substance of the review. `gitExec` builds a `[]string`
  argv into `orchestrator.Exec` → Docker `ContainerExecCreate`; there is no
  shell on the path, so option injection and refspec/pathspec reach are the
  exposure, not quoting. Client-supplied strings that reach argv:
  `body.Branch` (gated by `validBranchName`, now including the invalid-UTF-8
  rejection, and delivered via `--end-of-options` on the switch path),
  `?commit=` (gated by `validCommitHash`, hex-only 7..40, which also excludes a
  leading dash and every refspec form), `?limit=` (`Atoi`, clamped 1..200,
  re-serialised with `Itoa`), `body.Op` (a closed map key that never becomes
  argv data), and `?path=`/`body.Path` (always after `--`, hence a pathspec —
  except the `--no-index` branch, finding 5). Guest-supplied strings that reach
  argv: the branch name in `gitPush` (finding 1) and `user.name`/`user.email`
  read host-side from the setup VM's `HomesDir/<user>/base/.gitconfig`; the
  latter is contained because `-c user.name=<v>` fixes the key before the first
  `=`, so a hostile value can only extend the value, and `git config --file`
  performs no command execution. The six new endpoints as a set: `fetch`,
  `pull`, `branches`, `log`, `stash` put **zero** user bytes in argv;
  `checkout` puts exactly one, gated. Fetch + broker combination checked and
  closed: the in-guest credential helper is host-scoped (`GetGit(user, host)`),
  so a guest-planted remote at an unonboarded host gets no token, `--all` keeps
  guest-chosen remote names out of argv entirely, and `gitExec` does not source
  a login shell so the env-tier credentials are absent from the exec
  environment. Note for the record, not as a finding: `git fetch --all` and
  every other verb honour the guest's own `~/.gitconfig` and `.git/config`
  (`core.fsmonitor`, `core.hooksPath`, `ext::` remotes), so each endpoint is a
  trigger for guest-configured code execution *inside the guest, as the guest*
  — no boundary crossed, since the guest already has a root-capable shell.
- **Authz/authn** — clean; no reordering, no bypass, no widening. All six new
  routes are registered on the same `mux` and inherit
  `previewHostRewrite(authGuard(mode, csrfGuard(mux)))` (`server.go:420`); none
  is registered outside it, and none matches `isPreviewPath`/`isLinkPath`/
  `isSharedPath`, so no preview-only or public-link visitor reaches them and
  none is CSRF-exempt. The four POSTs get `csrfGuard`'s Origin check and the
  default 1 MiB `MaxBytesReader` (no path suffix matches the raised-limit
  cases); the two GETs are correctly exempt. Every handler resolves the project
  through `gitContext` → `getProjectFor` → `sp.projects.Get` on the **caller's
  own** per-user store, so a foreign project UUID 404s — no ID is trusted from
  the client, and no user, tenant, home, or container ID is client-supplied.
  `gitStashList` is a read implemented as POST, which is unusual but strictly
  safer here (it keeps the CSRF check). Read endpoints return only bytes
  produced by a `git` process in the caller's own VM.
- **Crypto** — nothing added and nothing hand-rolled. No auth, session,
  password-hashing, JWT or OIDC code anywhere in the range; the only
  comparison that matters is the pre-existing `subtle.ConstantTimeCompare` on
  the ingress token in `auth.go`. The global ban has nothing to bite on here.
- **Dependencies** — one new npm package, `@fontsource-variable/jetbrains-mono@5.3.0`,
  justified against stdlib-first (a font face is not stdlib, and the
  alternative is a Google Fonts CDN, which is strictly worse for a self-hosted
  box) and from a publisher already in the trust set. Supply chain verified
  rather than assumed: pinned in `package-lock.json` with a sha512 integrity
  hash; the installed package has **no `scripts` block, no dependencies, and no
  JavaScript of any kind** (CSS, SCSS, `.woff2`, LICENSE, JSON metadata only);
  every `src: url()` is relative; and all five committed
  `web/dist/client/assets/jetbrains-mono-*.woff2` are sha256-identical to the
  corresponding upstream files in `node_modules`. `npm audit` in `web/`: **0
  vulnerabilities** (0 with `--omit=dev` as well). Go: `go.mod`/`go.sum`
  untouched, the only new import is stdlib `unicode/utf8` and `path`. I did
  **not** run `deploy/vulncheck.sh` in this session (see "What I did not
  check"); the accepted baseline recorded by the phase-2/5/6 reviews earlier
  today is `GO-2026-4887` and `GO-2026-4883` in `github.com/docker/docker`,
  both `Fixed in: N/A`, in both the root and `deploy/caddy/ingress` modules,
  and this diff touches no Go dependency, so that baseline is unchanged by
  construction.
- **GCP surface** — not applicable and not touched. styrman is self-hosted
  (`.agents/docs/deploy/gcp.md` records GCP as N/A). No IAM binding, service
  account, `--allow-unauthenticated`, Secret Manager reference, env-var secret,
  or service-account key file appears anywhere in the range.
- **`image/skel` GOPATH change (`$HOME/go` → `$HOME/.go`)** — no interaction
  found with the credential broker or harness seeding, and no new capability.
  The skel copy loop in `image/entrypoint.sh:84-92` is guarded by
  `[ ! -e "${dst}" ]`, so this reaches new project homes only and never
  rewrites a user's file; `image/configs/harness-migrate.sh` moves a fixed
  inventory of `.claude`/`.gemini`/`.agents` paths and never touches `.go` or
  globs `$HOME/.*`; the broker's file tier
  (`image/configs/styrman-credentials.sh`) writes only paths the broker serves,
  rejects absolute paths and anything containing `..`, and the PATH-writable
  directory set is unchanged in size (`.go/bin` simply replaces `go/bin`
  alongside the pre-existing `.local/bin` and `.npm-global/bin`), so a
  file-tier credential gains no new PATH-shadowing reach. `.go` is not in the
  explorer's `managedHomeTarget` list, which is correct — that list guards
  *destructive* operations on platform-managed subtrees, and a Go module cache
  is ordinary user data. The one real consequence is operational, not security,
  and is already documented at `specs/styrman/runtime-image.md:161-172`:
  existing homes keep `$HOME/go`.
- **Prompt-injected-agent blast radius (the release-level question)** — I found
  no path in this diff by which an agent inside project A reaches project B,
  another user, or the host. All six verbs execute inside A's own microVM as
  A's guest user, over A's own bind mounts; the credential helper is
  host-scoped; the tenant dir is mounted into no guest (ADR-064) and the
  `GIT_CEILING_DIRECTORIES` addition closes the one discovery walk-up that
  could have put the project home under `add -A`. What the release *does* widen
  is what such an agent can make the **operator's own click** do to the
  operator's remote: finding 1 (force-push via a planted `HEAD`), plus the
  pre-existing config-driven equivalent, plus one-click "commit everything"
  from phase 5. None of that crosses a tenant boundary; all of it is worth an
  operator-facing confirmation on Push, which is a product decision rather than
  a finding.
- **Verification tooling run:** `npm audit` (clean, twice), manual sha256
  comparison of the committed fonts against `node_modules`, the new
  `verify-commit.sh` CDN grep re-run by hand over `web/dist/client/assets`
  (**no `url(http…)` — self-hosting holds**), a byte-level scan of the two
  binary-classified source files, a grep of the shipped bundle confirming the
  `reveal()` sanitiser is present in `p._project._term-DQfho7jD.js`, and
  behavioural verification of git 2.47.3's option parsing for `push`,
  `checkout`, `update-ref`/`symbolic-ref` and `diff --no-index` in a throwaway
  local repository under the session scratch directory (no network, nothing
  written into the repo).

## What I did not check

Named honestly rather than implied:

- **`deploy/vulncheck.sh` / `govulncheck` were not run in this session.** The
  canonical run needs the pinned `golang:1.25.13-trixie` builder image and I
  did not invoke Docker. The Go dependency graph is untouched by this range
  (`go.mod`/`go.sum` show no diff), so the accepted `GO-2026-4887` /
  `GO-2026-4883` baseline from this morning's phase reviews carries over — but
  that is inference from an unchanged manifest, not a fresh scan. Run it before
  tagging.
- **`go vet ./...`, `go test ./...`, `web` unit tests and the two integration
  suites were not run here** — that is the `verifier`'s tier, and I did not
  duplicate it. In particular I did not execute `git_verbs_test.go`, so I read
  its assertions rather than watching them pass.
- **`web/dist` freshness** — I confirmed the shipped bundle contains the
  security-relevant `reveal()` implementation and the v1.22.0 API surface, but
  I did not run the full `npm run build` + byte-comparison that
  `verify-commit.sh` performs, so I cannot assert the whole artifact matches a
  fresh build of `acb5918`.
- **Phases 2, 5 and 6 were not re-litigated**, per the brief. I verified their
  fixes are present and looked only for what a per-phase view could not see
  (cross-endpoint consistency, artifact-level questions). Their own findings
  files remain the record for those surfaces.
- **The frontend beyond the diff/git path** — `FileExplorer.tsx` history
  navigation, the recents grid, `explorerHistory.ts`/`recentsPanel.ts` and the
  `settings.tsx` font rows were read for injection sinks and `localStorage`
  parsing shape, but I did not re-derive phase 2's full `localStorage` →
  CSS analysis; I relied on its verification that only module constants reach
  CSS.
- **`web/e2e/run.mjs`** (+335 lines) was not read line by line; it was covered
  only by the secret scan over added lines. It is test-only code that runs on
  an isolated loopback state dir.
- **No dynamic testing of the running platform.** Everything above is source
  reading plus local git-parser experiments; I did not stand up a server, a
  Kata microVM, or the credential broker, so the findings state mechanism and
  reachability, not observed end-to-end exploitation.
