---
owner: security
date: 2026-09-10
---
# Security review — closeout phase 2 (2026-09-10)

Range reviewed: `.agents/scratch/phase2.diff` (27 files, generated 12:09 against `d286f2b`)
plus blast radius: `internal/httpapi/{check,format,setup,provision}.go`,
`internal/project/{store,uuid}.go`, `web/src/routes/settings.tsx`, `.gitattributes`,
`web/dist/client/_shell.html` (byte-level).

## Verdict: FINDINGS — five, all low; nothing crosses a tenant boundary

**Read this first: the artifact is stale.** The working tree has moved since the diff was
generated at 12:09. `web/scripts/normalize-shell.mjs` itself was rewritten at 12:13, and six
more files carry tree changes absent from the diff (`internal/httpapi/check.go`,
`internal/httpapi/format.go`, `web/e2e/run.sh`, `web/src/components/TerminalPane.tsx`, extra
hunks in `internal/httpapi/server.go` and `web/src/lib/api.ts`). The worst item below is a
defect in the *reviewed* version of `normalize-shell.mjs` that the tree has already fixed —
which is the point: whoever ships this must re-diff, because a review of a stale artifact
vouches for bytes that are not the ones being built. Nothing in the range is exploitable
across tenants: the new `provisioning` map is keyed by a server-minted crypto/rand UUID and
read only behind the existing per-user scope lookup, the ssh-key filter rejects every case
asked about, and the e2e drift guard writes nothing that survives the suite.

## Findings

- **[low / confidence: high]** `web/scripts/normalize-shell.mjs:17` *(as reviewed; already
  fixed in the tree)* — the script substitutes `u:0000000000000` into a JavaScript **numeric
  literal** position inside the inline router-payload `<script>` of `web/dist/client/_shell.html`,
  the file `go:embed`s into the server binary. Thirteen digits with a leading zero is a
  *legacy octal literal*, legal only in sloppy mode. Verified with node: `"use strict"; var
  x={u:0000000000000}` raises `SyntaxError: Octal literals are not allowed in strict mode`,
  and the same source as an ES module fails to parse at link time. It happens to work today
  only because TanStack emits that payload as a classic script
  (`<script class="$tsr" id="$tsr-stream-barrier">`) — sloppy mode, so it evaluates to `0`.
  The day the prerenderer emits `type="module"`, or a CSP/bundler wraps the payload in strict
  mode, the entire inline script throws at parse time and the SPA renders blank for every
  user, from a build step nobody would suspect. Remediation: the tree's `u:0` is correct;
  make sure that version is what gets committed and built, and never write a zero-padded
  digit run into a JS numeric position.

- **[low / confidence: medium]** `web/scripts/normalize-shell.mjs:12` — `/u:(\d{13})/.exec(text)`
  rewrites the **first** match anywhere in 7.6 KB of inlined script, with no assertion that
  exactly one match exists and no anchor to the router payload. A minified property named `u`
  holding an epoch-ms literal is exactly the shape being matched, so a future TanStack build
  can silently get an unrelated numeric literal zeroed instead — in a file the Go binary
  embeds and serves to every user, and which no reviewer reads because it is one 7.6 KB line.
  Remediation: `const all = [...text.matchAll(/u:(\d{13})/g)]; if (all.length !== 1) exit 1;`,
  anchor on the surrounding `,s:"success"` / `__root__` context, and assert the output length
  equals the input length minus the shortening (the `u:0` form no longer preserves size, so
  the log line's byte count is the only remaining evidence and nothing checks it).

- **[low / confidence: medium]** `internal/httpapi/check.go:410` and `internal/httpapi/format.go:120`
  *(tree-only — NOT in the reviewed diff)* — the check/format temp file moved from the
  workspace root to `$HOME/.styrman/tmp`, created by an in-guest `mkdir -p "$D"`. That is a
  new guest-writable tree nested under a mounted home, sitting beside `~/.styrman/logs/<projectId>/`,
  which the **host** reads as transcripts. A guest process (an agent that took a prompt
  injection, say) can pre-plant `~/.styrman/tmp` as a symlink to
  `~/.styrman/logs/<projectId>`; `mkdir -p` succeeds through a symlink-to-dir, and every
  editor keystroke debounce then drops a `mktemp` file full of editor content into the
  transcript directory for the history indexer to ingest. Blast radius is the tenant's own
  history — the host side opens through `os.Root` and cannot be walked out of — so this is
  pollution, not escape. But `.agents/rules/guest-writable-paths.md` asks for exactly this
  question to be answered *before* the code is written, and the diff answers it nowhere.
  Remediation: use a per-call `mktemp -d` under `/tmp` inside the guest (no stable name to
  pre-plant), or record the analysis in the comment beside `$D`.

- **[low / confidence: high]** `web/src/lib/sshKey.ts:44` and `:59` — `\p{Zs}` is deliberately
  outside `\p{C}`, so NBSP (U+00A0) and IDEOGRAPHIC SPACE (U+3000) inside the comment field
  are **accepted**, reach the `title` attribute and the clipboard verbatim, and are then
  collapsed by `abbreviateKey` — the row displays `me host` (two words) while the copied
  bytes are one token `me<U+00A0>host`. Strong-RTL letters (Hebrew, category Lo) and unbounded
  combining marks (Mn) are likewise accepted and do reorder / overflow the rendered row
  without being `\p{C}`. Not reachable today because the server already sanitises the same
  line: `publicKeyLine` (`internal/httpapi/setup.go:307`) runs
  `strings.Join(strings.Fields(stripControls(comment)), " ")`, and Go's `strings.Fields`
  splits on NBSP and U+3000 too — so this only bites if a second producer of the `ssh` tool's
  `details` ever appears. Remediation: either state in the comment that Zs/RTL/Mn are
  knowingly out of scope for the client-side second line, or reject anything outside
  `[\x20-\x7e\t]` in the comment and stop enumerating Unicode categories.

- **[low / confidence: high]** `web/src/lib/editorPane.ts` (`__stateForTest`) — a test seam
  exported from a production module and shipped in the browser bundle. It reads only
  in-memory pane state the user already owns, so there is no confidentiality impact; it is an
  API-surface smell rather than a vulnerability. Noted because "not for production callers"
  in a doc comment is not a boundary.

## Verified clean (the specific questions asked)

**(a) `web/src/lib/sshKey.ts` — reject set.** Compiled the actual module (`tsc` -> `node`,
probe written with `String.fromCodePoint` only, no raw control bytes anywhere) and exercised
42 cases. Rejected: RLO U+202E (in the comment and before the type), ZWSP U+200B (comment and
inside the base64), BOM U+FEFF mid-line, soft hyphen U+00AD, LRI U+2066, PDI U+2069, NEL
U+0085, NUL, ESC U+001B, lone surrogate U+D800, private-use U+E000, unassigned U+0378,
U+2028, U+2029, a bare LF or CR hiding a second key on one detail, a leading `command="..."`
authorized_keys option, a private-key header, and anything over `MAX_KEY_LEN`. TAB is
accepted as a separator and inside the comment, as designed, and `abbreviateKey` still splits
on it (`/\s+/`), so a TAB cannot hide a field in the row. A leading or trailing BOM is eaten
by `.trim()` before the test, and the returned value no longer contains it. The type enum is
byte-identical to the server's `keyTypeRe` (`internal/httpapi/setup.go:330`):
`ecdsa-sha2-nistp999`, `ecdsa-sha2-EVIL`, `sk-EVIL@openssh.com`, `sk-ssh-ed25519@evil.com`
and `ssh- AAAA` all reject; `ssh-{ed25519,rsa,dss}`, `ecdsa-sha2-nistp{256,384,521}`,
`sk-{ssh-ed25519,ecdsa-sha2-nistp256}@openssh.com` accept. Only the first passing detail is
returned, so a rejected detail cannot suppress a good one. What reaches `title` and the
clipboard is `pubKey` — the same validated line — rendered by React into an attribute
(escaped), and `navigator.clipboard.writeText(pubKey)`
(`web/src/routes/settings.tsx:206,343`); no other value reaches either.

**(b) `internal/httpapi/server.go` — the `provisioning` map.** No cross-tenant influence and
no cross-tenant read. The key is the project UUID, which a client cannot choose: `Store.Add`
overwrites `p.ID` with `NewUUID()` (crypto/rand, `internal/project/uuid.go:12`) and the
create request struct has no ID field. Both readers reach the map only after the per-user
scope lookup (`listProjects` via `s.who`, `getProject` via `s.getProjectFor`), so a tenant
can only ask about IDs already in their own store; a 122-bit collision would yield a cosmetic
badge and nothing else. No entry can leak: `defer s.provisioning.Delete(p.ID)` is registered
**first** inside the goroutine, so it runs **last** (after the recover defer) on every exit —
normal return, the delete-in-flight early return, the `EnsureRunning` error return, and a
panic — and the body is bounded by `context.WithTimeout(..., 3*time.Minute)`. `Store` happens
before the `go` statement, so the client's first GET after the 201 cannot see a gap. Residual
is liveness only: if `EnsureRunning` parks on the per-project mutex past the context (a mutex
is not context-aware) the owner sees `provisioning` until the goroutine returns. `deriveState`
is a pure function with a table test (`state_test.go`), precedence deleting > provisioning >
live, and the mirrored UI precedence in `projectBadge.ts` is covered by vitest. Badge titles
carry guest output, but it passes `redactSecrets(stripControls(...))`
(`server.go:653-676`) and lands in a React-escaped attribute.

**(c) `web/e2e/run.mjs` — the drift guard.** Writes nothing into the workspace, and nothing
survives the suite. The posted `path` is used only for `filepath.Base`/`filepath.Ext`
(`check.go:339-341`); the file the guest actually creates is a random `mktemp` name — under
`$HOME/.styrman/tmp` in the tree, under the workspace root in the reviewed version — removed
by `trap ... EXIT` *and* an explicit `rm -f`. No `drift-guard.<ext>` is ever created anywhere,
and `content: ""` means nothing is stored. Guest-side residue only: the `~/.styrman/tmp`
directory and, from the `.py` case, `/tmp/styrman-pyc` (PYTHONPYCACHEPREFIX). It runs against
the suite's own throwaway project and state dir. One functional gap, not a security one: the
guard catches drift in a single direction. `Dockerfile`/`Containerfile` are matched
server-side by BASENAME (hadolint) and are absent from `CHECKABLE_EXTENSIONS`, so
`FileEditorBody`'s new `CHECKABLE_EXTENSIONS.has(extOf(file.path))` gate silently drops the
lint gutter for Dockerfiles and the guard cannot see it.

**(d) `web/scripts/normalize-shell.mjs` — corruption and reach.** The latin1 round trip is
byte-exact and cannot mangle UTF-8: verified on the real file,
`buf.equals(Buffer.from(buf.toString("latin1"), "latin1"))` is true, the documented NUL
survives, and the current output contains zero bytes above 0x7F. The path is a fixed
`new URL("../dist/client/_shell.html", import.meta.url)` — no argv, no env, no cwd
dependence, so it cannot be pointed at another file. A missing timestamp exits 1 and fails
the build closed. The defects are the two findings above (octal literal, unanchored
first-match), not the encoding.

**(e) `web/src/components/FileEditor.tsx` — the abort signal.** No change to what content
goes where: same endpoint, same `{path, content}` body, same project scope; the signal only
cancels. Cancellation propagates the right way — the handler derives its 30 s context from
`r.Context()` (`check.go:353`), so an aborted fetch also kills the in-guest exec, lowering
guest load rather than raising it. The companion `discardDraft` / `setLoaded(null)` in
`EditorPane.tsx` drop a local buffer; nothing is transmitted.

## Checked

- **Secrets** — none added. Test fixtures are base64 padding, not keys; no `.env`, no
  credential-shaped path in the range. `normalize-shell.mjs` logs a build timestamp only.
  Guest output reaching `provisionError` -> badge `title` still passes
  `redactSecrets(stripControls(...))`.
- **Injection** — the only new shell surface is tree-only and uses a literal
  (`D="$HOME/.styrman/tmp"`); the sole caller-derived token reaching `bash -c` is still
  bounded by `safeSuffix` (`check.go:60`). The e2e body is JSON-encoded by Playwright. React
  escapes every new attribute; no `dangerouslySetInnerHTML` added.
- **Authz/authn** — unchanged. No route added or reordered; every new read sits behind the
  existing per-user scope lookup. Project IDs stay server-minted.
- **Crypto** — nothing hand-rolled, nothing added. The one crypto dependency in the analysis
  is `crypto/rand` behind `project.NewUUID`, which is what makes the provisioning key
  unguessable.
- **Dependencies** — no new package; `normalize-shell.mjs` uses `node:fs` only (stdlib-first
  respected) and `package.json` gained one script line, no dependency.
  `npm audit` (web/, with and without dev): **1 high** — `js-yaml@4.3.1`, GHSA-2883-xcg3-v3hh
  (CPU exhaustion on merge keys), reached via
  `@tanstack/react-start > @tanstack/start-plugin-core > xmlbuilder2`. Build-time transitive,
  pre-existing, untouched by this diff, but the build now runs one more script in that same
  toolchain.
  `govulncheck ./...` (local `go1.25.0`): **31 called vulnerabilities** — 29 stdlib
  (`net/http`, `crypto/tls`, `crypto/x509`, `net/url`, `os`, `encoding/asn1`,
  `net/http/httputil`, ...) all fixed at or below go1.25.13, plus GO-2026-4887 and
  GO-2026-4883 in `github.com/docker/docker@v28.5.2+incompatible`, both "Fixed in: N/A".
  Production builds in `golang:1.25.13-trixie` (`deploy/Dockerfile.server:9`), so the shipped
  binary clears the 29; the **local** toolchain does not, which means `go run ./cmd/styrman/`
  and the self-hosted `web/e2e/run.sh` server run on a toolchain with 29 known stdlib issues.
  Pre-existing and out of this diff's scope; note it, do not attribute it here. I did not run
  `deploy/vulncheck.sh` (docker-in-pinned-image path) — the local run above is what this
  section rests on.
- **GCP surface** — none. Self-hosted deploy; no IAM, service account, Cloud Run flag or
  Secret Manager change anywhere in the range.
- **BUG-007 hygiene** — all twelve changed/new text files carry zero raw control bytes
  (checked byte-wise); `git diff --numstat` shows numeric columns for every one of them, no
  "binary". The new sshKey tests use `\uXXXX` escapes, correctly. The one NUL in the range is
  the framework's, inside `_shell.html`, and `.gitattributes` already forces a textual diff
  for it. This review file is likewise escape-only.

## Not examined

- **Tree changes absent from the reviewed artifact**, beyond the two temp-file moves I did
  read: the extra `internal/httpapi/server.go` hunk in `New()` that wires
  `stillAuthorized = users.IsAuthorized` for `AuthMode == "header"`, the matching
  `CLOSE_REVOKED` (4003) additions in `web/src/lib/api.ts` and
  `web/src/components/TerminalPane.tsx`, and `web/e2e/run.sh`'s new invocation of
  `.agents/docs/testing/styrman/ensure-test-image.sh` (a script run with `bash` that I did
  not read). That revocation path deserves its own pass against the current tree.
- The 15 rebuilt `web/dist/client/assets/*.js` bundles — minified, not read. Only
  `_shell.html` was verified byte for byte.
- `web/e2e/run.mjs` outside the new drift-guard block.
- The vendored Caddy ingress module (`deploy/caddy/ingress`) — untouched by this range.
- No test suite was run (`go test ./...`, `vitest`, the e2e or integration tiers): that is
  the verifier's tier, not this review's.

## Response (main, 2026-09-10)

- Stale artifact — acknowledged; the tree kept moving through Phases 3–4
  while this review ran. The commits are by file name per phase; Phase 6's
  final verifier and reviews run on the committed tree.
- **Finding 2 (low) — fixed** before this review landed (`u:0`).
- **Finding 3 (low) — fixed.** The script now requires EXACTLY one
  `u:<13 digits>` match and exits non-zero otherwise, so it can never
  rewrite something it does not understand.
- 4 and 5 — noted; `govulncheck` on the local toolchain is the known
  unconfirmed figure (TODO, struck today as tracked by `vulncheck.sh`).
