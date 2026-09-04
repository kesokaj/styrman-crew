---
owner: security
date: 2026-09-04
---
# Security review — phase 6 git verbs (fetch, pull, branches, checkout, log, stash)   (2026-09-04)

Range reviewed: 62dff32..worktree (uncommitted phase 6; the supplied
`phase6.diff` is byte-identical to `git diff HEAD` for all code paths — the
only extra worktree hunks are `specs/styrman/guide.md` prose and one TODO.md
line). Blast radius followed outward from `internal/httpapi/git.go` through
`gitContext` → `getProjectFor` → `who` → `authGuard`/`csrfGuard`
(`internal/httpapi/{server,auth}.go`), `gitExec` → `orchestrator.Exec`
(`internal/orchestrator/exec.go`), the in-guest credential helper
(`image/configs/styrman-git-credential`, `internal/credbroker/broker.go`), and
the guest-writable input this code parses: the workspace's own `.git/config`,
refs, and commit objects.

## Verdict: FINDINGS

No high-severity issue and no exploitable hole today. The worst is a **false
second line of defense**: `git.go:551` appends `--` *after* the user-supplied
branch name and the comment above it claims that terminates options, so "a
future relaxation of that rule cannot turn into option injection here". It
does not — git parses left to right and has already consumed the operand
before it reaches `--`. Verified against git 2.47.3: `git checkout --orphan=x
--` parses `--orphan=x` as an **option** and creates an orphan branch, trailing
`--` notwithstanding. `validBranchName`'s `s[0] == '-'` is therefore the *sole*
gate on the switch path, documented as though it were the second of two. That
is precisely the kind of false comfort that licenses a later regression. The
gate itself holds: I could not find any string that passes `validBranchName`
and then escapes the repository, writes outside it, or reaches the network.

## Findings

- **[medium / confidence: high]** `internal/httpapi/git.go:548-551` — the `--`
  option terminator is appended *after* `body.Branch`, where it protects
  nothing, while the comment asserts it makes a future relaxation of the
  leading-dash rule safe. Demonstrated on git 2.47.3: `git checkout --orphan=x
  --` created branch `x` as an orphan; the operand was option-parsed. Split by
  path: `create:true` → `git checkout -b <name> --` is **already immune**
  because `-b` consumes its argument as a *value* (`git checkout -b --orphan
  --` → `fatal: '--orphan' is not a valid branch name`); `create:false` → `git
  checkout <name> --` puts the operand in the option stream, so only
  `validBranchName` stands between the request body and git's parser. Remediate
  one of two ways: (a) correct the comment to say the leading-dash check is the
  *only* gate, and pin that with the existing `TestValidBranchName` case so it
  is never "simplified" away; or (b) make the claim true by using git's real
  primitive — `git checkout --end-of-options <name> --`, verified to turn a
  dash-leading operand into a name (`fatal: invalid reference: --orphan=zzz`,
  not executed). Note `--end-of-options` cannot help the `-b` form (it would be
  eaten as the branch name), which is fine because that form is already safe.

- **[low / confidence: high]** `internal/httpapi/git.go:451,460` and
  `.agents/docs/specs/styrman/api.md:61` — `git fetch --all --prune` is
  documented and commented as updating "remote-tracking refs without touching
  the working tree". The refspec it obeys is `remote.<name>.fetch` from the
  workspace's `.git/config`, which is **guest-writable**. Demonstrated: with
  `remote.up.fetch = +refs/heads/*:refs/heads/ns/*` and `--prune`, a single
  fetch **deleted the local branches** `ns/precious-work` and `ns/other`. No
  privilege boundary is crossed — a guest with a shell can delete its own refs
  directly — but the realistic actor in this platform is a prompt-injected
  agent inside the VM, and the trigger is the operator's Fetch click, which has
  no confirmation. Remediate by dropping `--prune` (the tidiness it buys is
  small) and correcting both claims. The tempting alternative — enumerate
  remotes and pass an explicit `--refmap` — does work (verified: `git fetch
  --prune --refmap='+refs/heads/*:refs/remotes/up/*' up '+refs/heads/*:...'`
  pruned only remote-tracking refs and left every `ns/*` head intact) but note
  `--refmap` is `fatal` when combined with `--all`, and it would put
  guest-chosen **remote names** into argv, which then need their own
  leading-dash/control gate. Today `--all` keeps remote names out of argv
  entirely; do not lose that while fixing this.

- **[low / confidence: medium]** `internal/httpapi/git.go:688` (`gitRun`) via
  `firstLine` at `git.go:782` — the 409 body carries the first line of
  guest/remote-controlled git output verbatim: no `stripControls`, no
  `redactSecrets`, no length cap, while `guestErrMessage`
  (`internal/httpapi/server.go:575`) exists for exactly this text and applies
  all three. The diff routes four new endpoints through it, two of which
  (`fetch`, `pull`) surface text a *remote server* influences (`remote:`
  sideband lines) and remote names the guest wrote into `.git/config`. It lands
  in a toast (`SidePanelGit.tsx` `run()` → `toast("error", e.message)`), so the
  payoff is UI spoofing via control/bidi bytes plus an unbounded message.
  Confidence is medium, not high, because I verified the obvious credential
  channel is *closed at this git version*: git 2.47.3 already strips userinfo
  from its own `fatal: unable to access '<url>'` line. Pre-existing in
  `gitRun`/`gitPush`; widened here. Remediate: `writeErr(w, 409,
  errors.New(guestErrMessage("git", out)))`.

- **[low / confidence: high]** `web/src/components/SidePanelGit.tsx:435` — the
  client mirror `/^-|[\x00-\x1f\x7f ]/` drifts from the server gate: it omits
  C1 controls (`\x80-\x9f`), the invalid-UTF-8 check, and the 255-byte bound.
  Confirmed **not** a security boundary — the server validates independently at
  `git.go:538` before any git call, and the drift runs in the safe direction
  (client looser → server 400), so nothing the client accepts can bypass the
  server. Recorded so nobody later reads "the UI validates this" as licence to
  trust it. Remediate by aligning the regex and adding a length check, or by
  labelling it explicitly as UX-only.

## Checked

- **Secrets** — clean. No credential, key, token literal, or `.env` added; no
  new logging statement in any of the six handlers, so nothing is echoed. The
  only secret-adjacent strings in the diff are TODO.md prose about the ingress
  session token. The `gitRun` success path returns git's full output (capped at
  2 MiB by `maxExecOutput`) in `{"output":…}`; the frontend never renders it.
- **Injection** — the central question, and the deliberate boundary holds.
  `gitExec` builds a `[]string` argv into `orchestrator.Exec` → Docker
  `ContainerExecCreate`; there is no shell anywhere, so quoting is not part of
  the exposure. Only one user-supplied string reaches argv (`body.Branch`) plus
  one clamped integer (`limit`). Verified empirically on git 2.47.3 that a name
  passing `validBranchName` cannot: restore or escape to a path (`git checkout
  ../../../etc/passwd --` → `fatal: invalid reference`, the trailing `--` forces
  treeish-only); write outside `.git` (`checkout -b '../../../../tmp/pwn'` and
  `'a/../../../../tmp/pwn2'` both refused by check-ref-format, nothing created);
  or reach the network (checkout DWIM consults local remote-tracking refs only).
  Deferring ref-format validity to git is the right call and is pinned by
  `TestValidBranchNameDefersRefFormatToGit`. `stashOps` **confirmed closed**:
  `body.Op` is used only as a map key and the value is a fixed literal slice, so
  the op can never appear in argv as data; unknown ops 400 before any git call;
  `TestStashOpsIsAClosedSet` pins it. `gitLog` puts **no** user bytes in argv —
  `limit` is `Atoi`-parsed, range-checked to 1..200 with an out-of-range value
  falling back to 50, then re-serialised with `Itoa`; `--format` is a literal
  and the NUL-separated parse fails closed (`len(f) != 5` → skip). The `fetch`/
  `pull` network reach is the guest's own, from inside the guest, with a remote
  set the guest already controls and could already use — no new capability;
  `--ff-only` correctly removes the half-merged-tree failure mode, and `--prune`
  is the one flag that widens blast radius (finding 2).
- **Authz/authn** — clean, no tenant crossing. All six routes are registered
  inside `mux`, which is wrapped `previewHostRewrite(authGuard(mode,
  csrfGuard(mux)))` at `server.go:420`; none bypasses the middleware. Identity
  comes from the ingress-verified `X-Caddy-Email` gated by a constant-time
  compare of `X-Styrman-Ingress` (`auth.go:54`), then `IsAuthorized`;
  preview-only guests are refused every non-preview path. `gitContext` →
  `getProjectFor` → `sp.projects.Get` resolves the project **inside the
  caller's own per-user store**, so a foreign project ID 404s. No user, tenant,
  home, or container ID is ever taken from the client. The two new READ
  endpoints leak nothing across tenants: everything they return is produced by
  a `git` process in the caller's own VM. CSRF: the four POSTs pass
  `csrfGuard`'s Origin check and get the default 1 MiB `MaxBytesReader` (no
  path suffix matches the raised-limit cases); the two GETs are correctly
  exempt — safe methods, and cross-origin JS cannot read the JSON.
  Credential blast radius is bounded better than expected: the in-guest helper
  is **host-scoped** (`GetGit(user, host)`, 404 for an unonboarded host), so a
  fetch at a guest-chosen host gets no token, and `gitExec` does not source the
  login shell, so the env tier (`GH_TOKEN` etc.) is absent from the exec
  environment.
- **Crypto** — nothing added. No hand-rolled auth, hashing, session, or JWT
  code anywhere in this diff; the one comparison that matters (ingress token)
  is pre-existing `subtle.ConstantTimeCompare`.
- **Dependencies** — zero new. Go adds `unicode/utf8` (stdlib) only; TypeScript
  adds only intra-repo imports. `go.mod`, `go.sum`, `web/package.json` and
  `web/package-lock.json` are untouched (`git diff --stat` empty). Stdlib-first
  satisfied. `deploy/vulncheck.sh` (govulncheck v1.7.0 inside the pinned
  `golang:1.25.13-trixie`, the toolchain that ships): root module 2 called
  findings, `deploy/caddy/ingress` 2, both `GO-2026-4887` and `GO-2026-4883` in
  `github.com/docker/docker`, **Fixed in: N/A** — the standing accepted pair,
  unchanged by this diff; `image/harness` clean; script exit 0. For the record,
  a bare `govulncheck ./...` on this host's go1.25.0 reports 31 findings, all
  stdlib artifacts of the dev machine's older Go and not present in the shipped
  binary — which is exactly why `vulncheck.sh` exists. `npm audit` in `web/`:
  **0 vulnerabilities**, with and without dev dependencies. `go vet ./...`
  clean; `go test ./internal/httpapi/` passes.
- **GCP surface** — not applicable and not touched. styrman is self-hosted
  (`.agents/docs/deploy/gcp.md`); this diff adds no IAM binding, service
  account, Cloud Run flag, or Secret Manager reference, and no credential file
  reaches disk.
