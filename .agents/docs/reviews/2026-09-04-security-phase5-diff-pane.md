---
owner: security
date: 2026-09-04
---
# Security review — phase 5, git diff pane + fast commit   (2026-09-04)

Range reviewed: c1fff24..working tree (uncommitted phase-5 changes), plus the three
untracked files `web/src/components/DiffPane.tsx`, `web/src/components/DiffView.tsx`,
`internal/httpapi/git_verbs_test.go`.

Blast radius followed outward from the diff: `internal/httpapi/git.go` (all handlers, not
just the two changed), `gitExec` → `internal/orchestrator/exec.go` (`Exec`, `maxExecOutput`),
`internal/httpapi/server.go` (`csrfGuard`, `getProjectFor`, `writeErr`, `guestErrMessage`,
`firstLine`), `internal/httpapi/auth.go` (`authGuard`), `internal/project/store.go`
(`ValidateName`, the source of the `-C <ws>` path), and the SPA render path
`SidePanelGit.tsx` → `editorPane.ts` → `p.$project.$term.tsx` → `DiffPane` → `DiffView`.

## Verdict: FINDINGS

Nothing here is high or medium. All four questions put to this review come back clean:
`validCommitHash` is sufficient before the value reaches argv, the `--` placement in
`git show --format= --patch --no-color <hash> --` is correct and cannot be made to read or
write outside the repo, `commit {"all": true}` adds **zero** new authority (`POST
/git/stage {"all":true}` has run the byte-identical `git add -A` through the same
`gitExec` since before this diff — the new flag is a fused composition of two calls that
were already available at the same authorization level), and `DiffView` has no HTML sink
at all. What is left is three low-severity blast-radius items, worst first: `gitExec`
never constrains git's repository discovery, so `add -A` operates on whatever repo git
finds by walking **up** from `~/projects/<dir>` if the workspace itself has lost its
`.git`; the new error path uses the weak `firstLine` helper where this same package
already ships a redacting `guestErrMessage`; and `DiffView` renders guest-controlled
text with no control/bidi stripping into the very pane whose purpose is
"read this before you commit everything".

## Findings

- **[low / confidence: medium]** `internal/httpapi/git.go:85-91` — `gitExec` runs
  `git -C <ws> …` with only `HOME`/`USER` in the environment. It sets no `--git-dir`, no
  `GIT_CEILING_DIRECTORIES` and never asserts that the discovered toplevel *is* `<ws>`, so
  git's normal walk-up applies: if `<ws>/.git` is absent (the guest has a root shell in its
  own microVM and can delete it, and a failed provision can leave the tree without one)
  and an ancestor such as `/home/<guest>/.git` exists, every git handler silently retargets
  that ancestor repo. This is pre-existing — `POST /git/stage {"all":true}` already had it —
  but the new `commit {"all": true}` is the first path that *stages and commits* in one
  authenticated request, and the project home is what sits directly above the workspace:
  `.dockerd/<uuid>.img` (a multi-GB loop image), `.styrman/logs/`, and the broker-materialized
  credentials (`.git-credentials`, `.ssh/`, `.claude/`, `.config/gcloud/`). Combined with the
  existing `POST /git/push`, one further click sends that commit to a guest-controlled
  `origin`. No trust boundary is crossed — the actor able to arrange it is the guest, which
  already holds those credentials and can push them itself — so this is an integrity and
  availability footgun with a credential-leak flavour, not an escalation. It is also a
  denial-of-service shape on its own: `add -A` over a home containing the dockerd image will
  burn the 20 s deadline and leave a half-written index. Remediation, one line and cheap:
  add `"--git-dir", ws + "/.git"` (or `GIT_CEILING_DIRECTORIES=<ws>` in `ExecSpec.Env`) inside
  `gitExec`, or gate the `add -A` branch on
  `git rev-parse --show-toplevel` equalling `<ws>` before staging anything.

- **[low / confidence: medium]** `internal/httpapi/git.go:292` (and, pre-existing, `:312`,
  `:346`, `:420`) — the new `add -A` failure path answers with
  `errors.New(firstLine(out))`, where `out` is raw guest-produced git output. `firstLine`
  (`git.go:461`) only trims and cuts at the first `\n`: no length cap, no
  `stripControls`, no `redactSecrets`. This same package already ships the hardened
  helper for exactly this job — `guestErrMessage` (`server.go:567`) does
  `redactSecrets(stripControls(msg))` then `clampBytes(…, 300)`, and its own comment
  says why ("a token can ride in a remote URL"). The realistic leak is not on `add`
  (its errors are path-shaped) but on the sibling `gitPush` handler in the same
  blast radius, whose first error line routinely carries the remote URL with userinfo.
  The rendering side is safe (React text child, no markup), so the exposure is
  credential text in a JSON error body and in whatever logs it, plus unbounded
  guest-controlled length. Remediation: switch git.go's four `firstLine(out)` call
  sites to `guestErrMessage("<verb> failed", out)`, or make `firstLine` itself apply
  `redactSecrets(stripControls(...))` and the 300-byte cap.

- **[low / confidence: high]** `web/src/components/DiffView.tsx:9-31` — the renderer maps
  guest-controlled diff text to one `<div className="whitespace-pre …">{line}</div>` per
  line. There is no XSS here: React escapes the text child, there is no
  `dangerouslySetInnerHTML`, and the `className` is picked from a fixed set of literals by
  `line.startsWith(...)`, never built from content. Two residuals remain. (a) No
  control-character or bidi stripping. This repo already has `stripControls` and documents
  its purpose as removing "the characters that let untrusted text lie about itself" (Cc and
  Cf, including U+202A–U+202E and U+2066–U+2069). A hostile repository — one an agent
  cloned — can therefore craft a line that *renders* benign while the bytes committed are
  not, in the one pane whose entire job is pre-commit review, immediately beside a
  one-click "Commit everything". Trojan-source-style deception, not code execution.
  (b) `orchestrator.maxExecOutput` caps the diff at 2 MiB, so a file of many short lines
  yields up to ~1M sibling `<div>`s and hangs the tab; and the 2 MiB truncation is
  **silent** — the pane shows a cut-off diff with no marker, which is a bad property for a
  review surface. The component is moved verbatim from `SidePanelGit.tsx`, so neither
  residual is a regression, but the move widens the exposure from a `max-h-64` box to the
  main pane and adds a whole-working-tree diff button that had no previous equivalent.
  Remediation: run each line through the existing `stripControls` equivalent (or render
  bidi/format characters as visible escapes), cap the rendered line count with an explicit
  "diff truncated" row, and have the server signal truncation rather than trimming silently.

## Checked

- **Secrets** — none introduced. No `.env`, key, or credential file in the range;
  `git diff --stat` shows no change to `go.mod`/`go.sum`/`package.json`/`package-lock.json`.
  The only new literal identity is the e2e fixture `e2e@styrman.test` in `web/e2e/run.mjs`,
  a throwaway git author in an isolated state dir, not a credential. Grepped the rebuilt
  `web/dist/client/` bundle for `ghp_`, `github_pat_`, `AKIA…`, `-----BEGIN … PRIVATE KEY`,
  `sk-ant-`, `AIzaSy` — no matches. `crypto.randomUUID()` is used as a *commit message*,
  never as a token, so its CSPRNG properties are not load-bearing here.
- **Injection** — this is the substance of the review and it holds. `gitExec` builds a
  `[]string` argv (`ExecSpec.Cmd`) handed to `ContainerExecCreate`; there is no shell
  anywhere on the path, so quoting is not the exposure — option injection and refspec
  traversal are, and `validCommitHash` (`git.go:446`) closes both. It admits only
  `[0-9a-fA-F]{7,40}`: `len(s)` bounds bytes while `for _, c := range s` iterates runes, and
  every non-ASCII rune fails the hex switch, so no multi-byte string can slip the length
  gate. That rules out a leading `-` (no `--upload-pack=`, no `--output=`), `..`, `...`,
  `^`, `~`, `@{…}`, `:`, `/`, `.`, whitespace, NUL, newline, and every ref name — verified
  against the new table test in `git_verbs_test.go`, which covers each of those classes and
  passes (`go test ./internal/httpapi/` → ok; `go vet` clean). The `--` in
  `show --format= --patch --no-color <hash> --` is correctly placed *after* the revision
  with no pathspec following, so the hash can never be reinterpreted as a path and no path
  argument exists at all — `git show` reads only the discovered repository's object
  database and writes nothing, so it cannot reach outside the workspace by way of this
  parameter. `--no-color` on the command line also beats a repo-local `color.ui=always`,
  keeping ANSI bytes out of the response. Two behaviours considered and dismissed as
  non-issues, recorded for accuracy rather than as findings: the parameter is named
  `commit` but is really *any object name*, so a blob SHA returns that blob's raw contents
  and a branch or tag literally named e.g. `deadbeef` resolves ahead of the abbreviated
  object — both stay inside a repository the caller already owns outright and can read
  through the file explorer, and object IDs are not guessable. Note also that no frontend
  caller sets `commit:` yet (`openDiffInPane` is only called with path/staged/untracked);
  the validated parameter is reachable only by direct API call until Phase 6's log view
  lands. The unchanged `--no-index` branch's deliberate exit-code discard is correct and
  documented. `<ws>` itself is not attacker-shaped: it is built from `project.Dir`, frozen
  to `Name` and validated by `^[a-z][a-z0-9-]{0,30}$`.
- **Authz/authn** — unchanged and sound. Both touched routes are registered on the guarded
  mux (`server.go:267`, `:271`), behind `previewHostRewrite → authGuard → csrfGuard → mux`.
  `authGuard` requires the `X-Styrman-Ingress` shared token compared with
  `subtle.ConstantTimeCompare`, then a non-empty `X-Caddy-Email`, then
  `s.users.IsAuthorized(email)`; `isPreviewPath`/`isSharedPath` cannot match
  `/api/projects/…` (they require `/proxy/` or `/p/…/port/`), so no preview-only guest
  reaches these handlers. The project ID is *not* trusted from the client in any meaningful
  sense: `getProjectFor` resolves it through `sp.projects.Get` on the caller's **own**
  per-user store, so a foreign UUID simply 404s — no IDOR. `csrfGuard` rejects any
  mutating request whose `Origin` host differs from `Host`, which covers the
  cross-site form-POST vector (modern browsers send `Origin` on cross-site form
  submissions); its documented "no Origin ⇒ pass" allowance for non-browser clients is
  pre-existing and unchanged here. On question 3 specifically: forging a
  `commit {"all":true}` yields exactly what forging `stage {"all":true}` followed by
  `commit` already yielded — one request instead of two, no new capability, and nothing
  that is not already reachable by the same authenticated identity.
- **Crypto** — nothing hand-rolled, nothing added. No auth, session, password-hashing,
  JWT or OIDC code in the range; identity still arrives from the ingress and the only
  comparison is the existing constant-time one in `auth.go`. `crypto.randomUUID()` is a
  Web Crypto API call used for a human-readable commit message, not a secret.
- **Dependencies** — no new dependency in either ecosystem (no `go.mod`/`go.sum` or
  `package.json`/`package-lock.json` change), so stdlib-first is satisfied by construction;
  the new UI is React primitives already present and the new server code is stdlib plus the
  existing orchestrator. `govulncheck ./...` on the root module: **exit 3, 2 affected** —
  `GO-2026-4887` and `GO-2026-4883`, both `github.com/docker/docker@v28.5.2+incompatible`,
  both **Fixed in: N/A**; the vendored `deploy/caddy/ingress` module reports the same two at
  `v27.5.1+incompatible`. Both are pre-existing, untouched by this diff, and already on the
  record in `2026-09-02-security-v1.17.3.md` and `2026-09-04-security-v1.21.0.md` with no
  upstream fix available. `npm audit --omit=dev` in `web/`: **found 0 vulnerabilities**.
- **GCP surface** — none. Self-hosted deployment; `.agents/docs/deploy/gcp.md` records GCP
  as not applicable. No IAM binding, service account, `--allow-unauthenticated`, env-var
  secret, or service-account key file appears anywhere in the range.
