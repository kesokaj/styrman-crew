---
owner: reviewer
date: 2026-09-13
---
# Review — httpapi git/explorer/check/format audit (2026-09-13)

Scope: `internal/httpapi/git.go`, `internal/httpapi/explorer.go`, `internal/httpapi/check.go`,
`internal/httpapi/format.go` at commit `e56e775` (audit scan against recorded intent, not a diff).

## Verdict: APPROVE WITH NITS

The security-sensitive parts of this slice (guest argv construction, branch-name/commit-hash
validation, GIT_CEILING_DIRECTORIES, managed-subtree guards, guest-error sanitization) are
careful and well-reasoned, and match the security-review history recorded in their own
comments. The one real defect is a silent functional regression in `/format` (and, to a lesser
extent, `/check`): the temp file that the guest formatter/checker operates on was moved outside
the workspace in commit `46f3ec2` to fix a different, recorded problem (temp files polluting
`git status`), but this defeats the documented purpose of running formatters in-guest at all —
discovering the project's own formatter config — and neither the code's own header comments nor
`.agents/docs/specs/styrman/api.md` were updated to reflect it. No test catches the regression.

## Findings

- **[medium / confidence: high]** `internal/httpapi/format.go:119-128` (via commit `46f3ec2`) —
  `formatScript` creates the temp file under `$HOME/.styrman/tmp`, not under the workspace
  (`WorkDir: ws` only sets the *cwd*, not the file's location). Tools that resolve their
  project config by walking up from the **target file's own path** rather than cwd —
  `prettier` (`.prettierrc`/`package.json` `"prettier"` key), `black`/`ruff format`
  (`pyproject.toml`), `rustfmt` (`rustfmt.toml`/crate root), `shfmt` (`.editorconfig`) — can no
  longer find the project's config, because `~/.styrman/tmp` is not an ancestor of the
  workspace (`~/projects/<dir>`, ADR-027). Every `/format` call for a project that has any of
  these config files silently formats against tool defaults instead of the project's own style,
  which is precisely the case the header comment (`format.go:20-26`, unchanged since before
  `46f3ec2`) and `.agents/docs/specs/styrman/api.md:177` promise it will not do ("piped ... into
  a temp file created in the workspace root ... so formatters discover the project's own
  config"). That promise is now false for the tools that matter most. Fix: create the temp file
  inside the workspace (e.g. a dotfile under `<ws>/.styrman-tmp/`, git-ignored, swept the same
  way) so it sits in the config-discovery path, or explicitly pass each formatter's own
  "search from this directory" flag (not all support one) and update the spec/comments to match
  whichever is actually done.
- **[low / confidence: medium]** `internal/httpapi/check.go:18-21` and `check.go:427-430` —
  the same temp-file relocation applies to `/check`; the top-of-file design comment still
  claims "a temp file in the workspace root ... so checkers find the project's own config"
  while the later comment on `checkScript` says the opposite and explains why. Lower severity
  than format.go because the affected checker here is mainly `shellcheck` (config via
  `.shellcheckrc`, resolved from the file's path) — a missed style/severity override degrades
  the check rather than breaking a save — but it is the same defect and the same stale
  documentation, and `.agents/docs/specs/styrman/api.md:183` repeats the same now-inaccurate
  claim. Fix together with the format.go finding; update both header comments and the spec once
  the actual behavior is decided.
- **[low / confidence: low]** `internal/httpapi/explorer.go` — this file's actual contents
  (file explorer CRUD, transcript history/search, `/settings` + operator-tunable settings,
  admin user add/remove, credential import, cross-project notification polling) span several
  unrelated features under a name that promises only the file explorer. Two engineers working
  the settings/admin surface and the file-explorer surface would be editing the same file. Not
  a functional defect today, but worth splitting along the seams (`settings.go`, `users.go`,
  `notifications.go`) the next time any of those areas changes, per the decoupling criterion.

## Criteria

1. **Dead code** — pass. `firstLine` (git.go) is used by format.go; `guestErrMessage`/`stripControls` (server.go) are used throughout; no orphaned exports found in this slice. The `../`-prefix re-check in `gitDiff` (git.go:538) is unreachable given `scopeRel`'s `path.Clean` on a rooted path, but it's a harmless redundant guard, not dead code left behind by a removed feature — not worth a finding on its own.
2. **Flow tracing** — pass except the finding above: `gitContext` → `gitExec` → guest `Exec` is consistent end-to-end (env, `GIT_CEILING_DIRECTORIES`, timeouts, error propagation via `writeErr`/`guestErrMessage`); `checkFile`/`formatFile` share the same `gitContext` seam correctly. The one break in the chain is `formatScript`/`checkScript`'s temp-file location no longer matching what the callers' surrounding comments and the API spec claim it does.
3. **Standards conformance** — mostly pass. Guest argv is direct-argv (no shell) throughout git.go per the golang skill and the file's own header rule; `validBranchName`/`validCommitHash` are narrowly scoped security gates, not a reimplementation of git's grammar, matching the "simplest correct solution" principle. `explorer.go`'s `importGitCredential` interpolates `body.Host` into a `bash -lc` string rather than argv, but `hostnameRe` (`^[A-Za-z0-9.-]+$`) forecloses every shell metacharacter, so this is a deliberate, narrow exception (login-shell env sourcing needs a shell), not a violation.
4. **Directory layout** — pass for git.go/check.go/format.go (colocated with the rest of `internal/httpapi`, no stray files). `explorer.go`'s grab-bag scope is a layout concern, noted above as low severity.
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth/crypto here; git identity parsing (`gitUserFromConfig`) is a narrow, tested subset of `git config --file` semantics rather than a full reimplementation, and the file says so explicitly. `formatScript`/`checkScript` duplicate a fair amount of shell-fragment plumbing (mktemp/sweep/byte-count check) between the two files; that duplication is exactly why the config-discovery contract drifted out of sync between them — worth factoring into one shared script builder if either changes again, but not a standalone finding.
6. **Decoupling** — pass for git.go/check.go/format.go (each is a single deep module: guest-exec-in-VM formatting/checking, or the git panel). `explorer.go` is the exception noted above.

## Not examined

- `internal/httpapi/server.go`, `provision.go`, `browser.go`, `network.go` and the rest of
  `internal/httpapi` (only referenced in passing to confirm `guestErrMessage`/`stripControls`
  behavior and callers of `firstLine`).
- `internal/files`, `internal/history`, `internal/orchestrator`, `internal/project`,
  `internal/credstore` internals (trusted at their public API boundary; not re-audited here).
- The web frontend consumers of these endpoints (`web/src/...`).
- Whether any formatter in the runtime image actually has a project-level config file in
  practice on this deployment (the finding is about the mechanism being broken, not about
  having reproduced a wrong diff against a live `.prettierrc`).
- `.agents/docs/architecture/DECISIONS.md` ADRs not already cited (ADR-027, ADR-030/031/032,
  ADR-064) — read only as needed to confirm the workspace/home path model used by `scopeFor`
  and `gitContext`.
- Test files (`git_test.go`? none found — no `git_test.go` in this package; `check_test.go`,
  `format_test.go`, `server_test.go` were grepped for specific symbols only, not read in full.
