---
owner: security
date: 2026-09-13
---
# Security review — audit-fix batch (2026-09-13)

Range reviewed: 7066301..e18d62c

(`git diff 7066301..e18d62c -- . ':!web/dist' ':!.agents'` — two commits, 60741b7 and e18d62c;
50 files, +1200/−324. `git diff --numstat` over the whole range reports no `-`/`-` pair, so
nothing in it is binary and every hunk was readable as text — BUG-007 check done before review,
not after. `web/dist` and `.agents` excluded on the dispatcher's instruction.)

## Verdict: FINDINGS

Nothing in this batch reopens a closed hole, and the two headline items are genuinely fixed and
verified: the credential snippet's `/16` octet arithmetic is gone and the injected
`STYRMAN_CRED_BROKER` is trusted (BUG-054), and the Go/Node pins in `image/Dockerfile` carry the
publishers' own sha256 — I fetched both today and they match byte for byte. The worst thing the
batch *opens* is in the orchestrator: `recreateMissingNetwork` now answers a missing network by
**force-removing the project container** instead of reconnecting it, while still deciding "missing"
from *any* `NetworkInspect` error rather than a not-found one. A transient daemon error or a
context deadline on that inspect therefore destroys a running project's container filesystem
(homes and workspaces survive — both are bind mounts — but tmux sessions, running agents and
anything installed only in the container do not). `errdefs.IsNotFound` is already imported and
used on the same object twenty lines above. Everything else is low: a not-found predicate that
inverts its own log line, a `grep` that can never match the line it guards, a new name-prefix
hiding rule that a guest can occupy, one half of the audit's ingress cookie strip applied to the
wrong block, and U+00AD still missing from the diff pane's invisible-character class.

## Closed by this range

Audit findings the range closes, with the hunk that does it.

| Audit finding | Closed | Hunk |
|---|---|---|
| **high** `styrman-credentials.sh:25-31` — `/16` broker arithmetic, silent (BUG-054) | **yes** | `image/configs/styrman-credentials.sh` — the whole `__sc_ip`/`cut -d. -f1,2`/`.255.254` block deleted; the injected `STYRMAN_CRED_BROKER` is authoritative, and `$? != 0` from either probe now sets `~/.styrman/broker-unreachable`, reported by `profile-motd.sh:12-14`. Pinned by `internal/credbroker/snippet_test.go` (new, +94). |
| companion **medium** `styrman-credentials.sh:29,54-70` — unauthenticated peer chosen by arithmetic | **yes, by construction** | With the derivation gone, the peer is the server-injected bridge address; no address outside the guest's own bridge can be reached by mis-derivation. The "trust whatever answers" property of the broker protocol itself is unchanged (still network-position auth) — see Residuals. |
| **medium** `styrman-update:313-315` — signature gate knows 2 of git's 4 markers | **yes** | `deploy/update/styrman-update` — regex widened to `(SSH SIGNATURE\|PGP SIGNATURE\|PGP MESSAGE\|SIGNED MESSAGE)` with "last must be SSH" kept; plus `GNUPGHOME="$(mktemp -d)"` for the verification so the openpgp/x509 paths cannot succeed even if the parse is ever wrong again. Fixtures in `internal/config/styrman_update_test.go` (new, +124) via the gated `__verify-tag` verb. |
| **medium** `image/Dockerfile:89-91` — Go toolchain unpinned/unverified | **yes, verified** | `ARG GO_VERSION=1.27.1` + `ARG GO_SHA256=63d339f0…8445`, `sha256sum -c -` before `tar`. **I fetched `https://go.dev/dl/?mode=json&include=all` on 2026-09-13: the publisher's sha256 for `go1.27.1.linux-amd64.tar.gz` is `63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445` — an exact match.** |
| **medium** `image/Dockerfile:92` — NodeSource `curl \| bash` as root | **yes, verified** | Replaced by the official tarball: `ARG NODE_VERSION=22.23.2` + `ARG NODE_SHA256=d60acfe0…f307`, `sha256sum -c -` before `tar -C /usr/local --strip-components=1 -xJf`. **I fetched `https://nodejs.org/dist/v22.23.2/SHASUMS256.txt` today: `d60acfe00a2932254bb0ad20e01b0d74397a0875595de719654b214f4b03f307  node-v22.23.2-linux-x64.tar.xz` — an exact match.** The `--strip-components=1 -C /usr/local` extraction is bounded by that check: `set -eux` aborts the layer before `tar` runs on a substituted archive. |
| **medium** `image/Dockerfile:96` — claude-code unpinned | **partly** | `ARG CLAUDE_CODE_VERSION=2.1.270`. The version pin (the rollback-determinism half) is closed; `--ignore-scripts` is deliberately *not* applied, with the reason written down ("its postinstall is what installs the CLI"). See Residuals. |
| `image/Dockerfile:97` — Antigravity `curl \| bash` | **as the audit specified** | The audit's own remediation was "record the accepted risk … instead of a hash that would break every release". Done: the installer is fetched to `/tmp/agy-install.sh` then run (so a truncated fetch cannot execute), and the comment at `:55-65` names it as the one floating install with the reason. Root code execution from an unauthenticated remote script at build time remains — now a decision, not an oversight. |
| **low** base images tag-pinned but not digest-pinned | **yes** | `deploy/Dockerfile.server:9,19` and `image/Dockerfile:8,15` — all four `FROM` lines now carry `@sha256:…`. (Digests not verifiable offline; a wrong one fails the build loudly, not silently.) |
| **medium** `generator.go:264` — bare `HasSuffix` open redirect on the portal | **yes** | `deploy/caddy/ingress/generator.go:278` — `trust login redirect uri domain suffix .%s path prefix /`. Confirmed against `go-authcrunch@v1.1.41/pkg/redirects/redirect_match.go:28,51-64`: the matcher is `strings.HasSuffix(u.Host, cfg.Domain)`, so the leading dot restores the label boundary; `crew.`/`auth.`/`*.public.crew.` all still match and the apex is not a served site (`labels.go:69-73` now refuses a hostname that sanitizes to nothing, so no site block can be `.<domain>`). Regression-guarded by `assertNotContains(…, "domain suffix "+BaseDomain+" path")`. |
| **medium** `bridge.go:196` — `ServeSetup` boots a VM before the Origin check | **yes, both halves** | `internal/terminal/bridge.go:115-120` and `:199-203` call `SameOriginOrAbsent` as the **first statement** of `ServeHTTP` and `ServeSetup`; the check moved to `internal/terminal/origin.go` (new) and `httpapi.browserSocket` now calls the same function. `origin_test.go` constructs the bridge with nil store/orchestrator so reaching a side effect would panic. The generator's `cookie samesite lax` (`:270-273`) closes the cross-*site* half. |
| **low** `auth.go:38`, `server.go:605` — exemptions computed on an unrouted path | **the path half** | `internal/httpapi/auth.go:39` (`isLinkPath(path.Clean(r.URL.Path))`) and `internal/httpapi/server.go:606` (`p := path.Clean(r.URL.Path)` for all three exemptions). The "no identity on the context for `/link/`" half is **not** closed. |
| **low** `generator.go:402-411` — public block forwards `Cookie` | **wrong block** | The strip landed in `writePreviewWildcard` (`:140-142`), not in the `ModePublic` branch the finding cited. See finding 5. |
| **medium** `history.go:230` — `Read` lacks the `IsRegular` guard | **yes** | `internal/history/history.go:233-238` (`rt.Lstat` + `IsRegular` before open) and `:248-250` (re-assert on the opened fd), exactly `searchOne`'s shape. |
| **medium** `files.go:180` — `ContainedPath` dead check-then-use helper | **yes** | Deleted outright (`internal/files/files.go`, −27) along with its test. |
| **medium** `files.go:297` — `Read` discards `io.ReadFull`'s error | **yes** | `internal/files/files.go:270-275` — accepts `io.EOF`/`io.ErrUnexpectedEOF`, returns anything else. The untested fault branch is declared in the comment rather than hidden. |
| **medium** `config.go:170` — `STYRMAN_DOCKER_DISK_GB` unbounded at boot | **yes** | `internal/config/config.go:237-239` fails closed 5–500; `loadOverrides` warns instead of silently ignoring (`:306-311`). |
| **low** `credstore.go:86` — 0700 only on create | **partly** | `internal/credstore/credstore.go:90-94` adds `os.Chmod(baseDir, 0o700)`. The per-user dir made by `atomicWrite` (`:102`) still gets its mode only on create — the other half of the finding. |
| **low** `broker.go:87-92` — store faults reported as "not onboarded", unlogged | **no — the guard is wrong** | See finding 2. |
| **low** `credentials.go:36` — store read failures masked as 404 | **yes, for two of three kinds** | `internal/httpapi/credentials.go:39-45,59-64` — `file` and `env` now 500 + WARN. `case "git"` (`:66`) still swallows the error into the not-found path. |
| **low** `server.go:842` — `createProject` leaks the workspace dir | **yes** | `internal/httpapi/server.go:847,853` — `os.Remove(workdir)` on both failure paths. |
| **low** `browser.go:456` — relay pumps not joined before the slot is released | **yes** | `internal/httpapi/browser.go:466-476` — `sync.WaitGroup` with `defer pumps.Wait()` declared before the pumps start. |
| **low** `uuid.go:12` — `crypto/rand` failure falls back to a quarantined id | **yes** | `internal/project/uuid.go` returns `(string, error)`; `store.go:100-106` quarantines into `.rejected` on the migration path and `Add` (`:161-165`) propagates. v4 version/variant bits unchanged and correct. |
| **low** `config.go:503` — parse errors swallowed | **yes** | `envIntOr`/`envDurationOr` now WARN (`:520`, `:530`). |
| **medium** `format.go:119` — temp file outside the workspace breaks config discovery | **yes, with new surface** | `internal/httpapi/format.go:131-143` (`formatDir`) + `:150-165` (`formatScript`). The confinement holds (see Cleared); two new low defects came with it — findings 3 and 4. |
| **low** `styrman-ctl:646` — `edit KEY=VALUE` | **the injection half only** | `deploy/styrman-ctl:646-652` validates the KEY against `^[A-Z][A-Z0-9_]*$`. Secrets on argv (shell history, `ps`) — the actual finding — is unchanged. |
| **low** `BrowserPane.tsx:327-334` — `url`/`title` cast, not checked | **yes** | `web/src/components/BrowserPane.tsx:329-331` — `typeof t.url !== "string" \|\| typeof t.title !== "string"` return. |
| **low** `api.ts:867-868` — `portUrl` interpolates without the `u` tag | **yes** | `web/src/lib/api.ts:869-870` — `` u`/p/${project}/${term}/port/${String(port)}/` ``. |
| **low** `DiffView.tsx:10` — zero-width family not revealed | **partly** | `web/src/lib/diffReveal.ts:14` adds `​-‏`, `⁠`, `﻿`. **U+00AD is still missing** — finding 6. |
| **low** `web/package.json` — `js-yaml` 1 high | **no** | `web/package-lock.json` is unchanged in this range; `npm audit --omit=dev` still prints `1 high` (see Checked). |
| **medium** `broker.go:61` — every inner container gets the whole credential set | not code; ADR amendment, **not verified** (`.agents` excluded from my diff) |  |
| `styrman-ctl:555`, `host-setup.sh:50,65-68,199-207,224`, `firewall/styrman-firewall.sh:69-70`, `entrypoint.sh:165`, `credstore.go:459` (`safe` collision), `explorer.go:102` (`managedHomeTarget` symlink first component), `bridge.go:369-387` (socket outlives the JWT), `identity.go:14-34` (comment) | **not addressed** | I re-read each at HEAD to confirm: all still present, unchanged. |

## Findings

- **[medium / confidence: medium]** `internal/orchestrator/orchestrator.go:1477-1489` (`recreateMissingNetwork`,
  reached from `ensureRunningLocked:351-360`) — the "is the network missing?" test is
  `if _, err := o.docker.NetworkInspect(...); err == nil { return false }`, i.e. **any** error is
  read as *absent*: a daemon hiccup, an API error, a context deadline on a loaded box. Before this
  range the consequence of guessing wrong was a harmless `NetworkConnect`; this range changes it to
  `ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})` followed by a recursive
  `ensureRunningLocked` that builds a fresh container. So a transient inspect failure now destroys a
  project's container filesystem on what the operator experiences as "I clicked Start". Per
  `AGENTS.md`, the home and the workspace are bind mounts and survive; tmux sessions, running agents
  and anything installed only inside the container do not — which is exactly the loss the LAN-mode
  toggle documents as the price of an *explicit admin action*, here taken automatically. The change
  itself is right (a reconnected container keeps a stale `STYRMAN_CRED_BROKER` — BUG-054's other
  half); only its trigger is too broad. *Remediation:* `if err == nil || !errdefs.IsNotFound(err) { return false }`
  — `errdefs` is already imported and used on the same object at `:362`. Recursion depth is
  separately fine: a failed remove returns false, and the second pass takes the `IsNotFound` create
  branch rather than `ContainerStart`, so the "one level" claim in the comment holds except under a
  network being deleted twice mid-call.

- **[low / confidence: high]** `internal/credbroker/broker.go:93-95` — the new fault log is guarded by
  `if !os.IsNotExist(err) && !errors.Is(err, os.ErrNotExist)`, but `credstore.GetGit`
  (`internal/credstore/credstore.go:137-146`) returns the package sentinel
  `ErrNotFound = errors.New("credential not found")` (`:33`), which satisfies neither predicate.
  I ran the predicate against that sentinel: both are `false`, so the branch is taken. Every
  ordinary *not-onboarded* lookup — the normal answer for any host the user has not added, and git's
  credential helper asks per host on every operation — is now logged at WARN as
  `"cred broker: git credential read failed"`. The fix inverts its own purpose: a real fault is no
  longer distinguishable from routine, which is the BUG-034 family the surrounding code cites.
  No secret is logged (user, host and the error string only — the dispatch's question on this line
  is clean). *Remediation:* `if !errors.Is(err, credstore.ErrNotFound)`, as the audit's own fix text
  said.

- **[low / confidence: high]** `internal/httpapi/format.go:156` — the `.git/info/exclude` idempotence
  probe is `grep -qsx '.styrman-fmt-*'`. That is a BRE: `-*` means "zero or more hyphens", so the
  pattern can never match the literal line `.styrman-fmt-*` that the very next statement appends.
  Measured: `printf '%s\n' '.styrman-fmt-*' > f; grep -qsx '.styrman-fmt-*' f` exits 1. Consequence:
  **every** format request appends another identical line to the workspace's `.git/info/exclude`,
  without bound — an authenticated, repeatable, unbounded write into a git control file, and a
  guaranteed nuisance in the repo the operator is reviewing. *Remediation:* `grep -qsxF` (fixed
  string), which matches.

- **[low / confidence: high]** `internal/httpapi/explorer.go:67-69` with `internal/httpapi/format.go:154`
  — the pair hides **any** entry whose name starts `.styrman-fmt-` from the file explorer's listing,
  and the appended `.git/info/exclude` pattern hides the same names from `git status` and "Stage
  all". The decision is made on the *name*, not on the file the server just created: the formatter's
  own temp file exists for about 100 ms, while a file a guest names itself lives forever and is now
  invisible in both of the panes the operator uses to see what an agent wrote before committing it.
  That is the same class of concern `web/src/lib/diffReveal.ts` is written against — the reviewer
  being shown something other than what is there — arrived at from the other direction. Bounded to
  the guest's own project (no tenancy crossing), hence low. *Remediation:* filter on the exact name
  the server handed out for that request (it already computes `dir`), or drop the explorer filter
  and accept a 100 ms visible dotfile.

- **[low / confidence: high]** `deploy/caddy/ingress/generator.go:421-427` — the audit's public-block
  `Cookie` strip was applied to `writePreviewWildcard` (`:140-142`) but **not** to the `ModePublic`
  branch of the per-app block, which is the code the finding cited (`generator.go:402-411` at the
  audited commit). A container labelled `caddy_public: 'true'` therefore still receives every
  signed-in user's `AUTHP_ACCESS_TOKEN` verbatim — the portal cookie is `Domain=<BASE_DOMAIN>`-scoped
  (`:269`), so the browser sends it — and can replay it against `crew.<BASE_DOMAIN>`. Neither block
  strips `Authorization`. Latent today (the only labelled container is the ModeAuthenticated styrman
  server), which is precisely why the audit wanted it closed before it became load-bearing.
  *Remediation:* emit the same `request_header Cookie …` line plus `request_header -Authorization`
  in the `ModePublic` branch, and extend the new generator test to assert both blocks.

- **[low / confidence: high]** `web/src/lib/diffReveal.ts:14` — the widened `INVISIBLE` class covers
  `​-‏`, `⁠` and `﻿`, but **not U+00AD SOFT HYPHEN**, which the audit finding
  named explicitly and which the `-` range does not reach (0xAD > 0x9F). A soft hyphen
  inside an identifier still renders invisibly in the pane whose stated job is reviewing a change
  before it is committed. *Remediation:* add `­`, and cheaply `⁡-⁤` and
  `︀-️` while the class is open; the cases belong in the existing `diffReveal.test.ts`.

- **[low / confidence: medium]** `image/configs/profile-motd.sh:12-14` — the marker file is written by
  the guest user under the guest-writable home and its content is interpolated into the banner as
  `echo "…($(cat "$HOME/.styrman/broker-unreachable" 2>/dev/null))…"`. On the dispatch's specific
  question: **there is no command injection.** Command-substitution output is not re-scanned for
  `$(…)` or backticks, the substitution sits inside double quotes so there is no word-splitting or
  globbing, and the leading literal text means no `-e`/`-n` is ever parsed as an `echo` flag. What
  does reach the operator's xterm.js unfiltered is arbitrary bytes, raw terminal control sequences
  included, printed by every new login shell and surviving the process that planted them. That is
  strictly weaker than the `~/.bashrc` write the same guest already has, and the whole MOTD is gated
  on `[ -t 1 ]` so no host-side `docker exec` ever parses it into a JSON reply — hence low, not
  medium. *Remediation:* `| tr -cd '[:print:]' | cut -c1-200` on the way out, the shape
  `image/entrypoint.sh:163` already uses on guest output.

- **[low / confidence: medium]** `deploy/update/styrman-update:150-152` — the cleanup trap is built by
  expanding the directory into a single-quoted string at definition time:
  `trap "rm -rf '${gh}'" RETURN`. `mktemp -d` honours `TMPDIR`, so a `TMPDIR` containing a single
  quote breaks the quoting and the remainder of the path is evaluated as root when the trap fires.
  The precondition is control of root's environment on a hand-run `sudo styrman-update` (the unit
  sets `RuntimeDirectory` and no `TMPDIR`), i.e. an attacker who is already root — so this is
  robustness, not a live hole. It is on the record because this same file deleted exactly this
  construction elsewhere after a security review (`:207-211`, "operator-env paths are a root
  shell-injection surface"). *Remediation:* keep the scratch path in a script-scope (not `local`)
  variable and let the trap expand it at fire time, or `mktemp -d -p /run` so `TMPDIR` is not in the
  loop. Trap scoping itself is correct — I confirmed empirically that a `RETURN` trap set inside a
  bash function fires once, on that function's return, and does not leak to later functions.

- **[low / confidence: medium]** `deploy/update/styrman-update:140-142` — the allowed-signers owner
  check was relaxed from `!= "0"` to `!= "$(id -u)"` so the non-root `__verify-tag` test verb can
  pass. I checked every call site: in production `verify_tag` is only reached after
  `[ "$(id -u)" -eq 0 ] || fail "must run as root"` (`:185`), so `id -u` is 0 and today's bar is
  unchanged. The residual is that the check on the file which "decides whose tags this host will
  build and run as root" no longer *says* root: any future path that reaches `verify_tag`
  un-elevated will accept an allowlist owned by that caller. *Remediation:* keep the literal `0` and
  let the test hook pass an explicit override, or assert root inside `verify_tag` unless the hook is
  active.

## Residuals worth naming (no action claimed closed)

- **Portal redirect, internal half.** `.{BASE_DOMAIN}` closes the external open redirect, but every
  host under the base domain is still a trusted post-login target with `path prefix /` — including
  the opt-in public preview hosts `<token>.public.crew.<BASE>`, which serve a tenant's own
  application content. The narrowed primitive is "land the freshly authenticated victim on another
  tenant's app", and the new preview cookie strip means that app no longer receives the session.
  low / medium confidence; tightening would mean `domain regex` or an explicit list.
- **claude-code lifecycle scripts.** Pinning the version does not stop a transitive dependency's
  postinstall from running as root at build time; the image is the one every project microVM boots
  with live brokered credentials. Now a recorded decision rather than an oversight, which is the
  improvement; the blast radius is unchanged.
- **`node`'s SHASUMS256.txt is same-origin with the artifact** — the same caveat the audit raised for
  Kata. Recording the digest in-repo (as done) is the right shape: the network fetch is the
  bootstrap, the repo is the pin.
- **`credentialContent` `case "git"`** still folds a store fault into not-found (`credentials.go:66`),
  and `atomicWrite`'s per-user directory still takes its mode only on create.

## Cleared on inspection

- **`__verify-tag` gate strength.** `install -m 755 … /usr/local/bin/styrman-update`
  (`host-setup.sh:273`), root-owned, **not setuid**, and no sudoers entry anywhere in `deploy/` grants
  it (the only `NOPASSWD` in the repo is `image/entrypoint.sh:55`, inside the guest's own microVM).
  A non-root caller running `STYRMAN_UPDATE_TEST=1 styrman-update __verify-tag …` executes
  `git cat-file`/`verify-tag` entirely as **themselves**, against a repo and an allowlist they name;
  they learn nothing `git` would not tell them directly, and the `[ -r ]`/`stat` messages are no
  better an oracle than `ls`. The hook cannot reach the fetch/deploy flow (it `exit`s), cannot read
  `/etc/styrman/allowed_signers` (`$3` overrides the path), and cannot be reached under the timer
  (`$1` is never `__verify-tag`). `set -euo pipefail` is in force before it. The gate is adequate;
  the two notes above are the residuals, not a bypass.
- **`STYRMAN_FMT_DIR` confinement.** `formatDir` (`format.go:131-143`) refuses absolute paths, cleans
  *relative* (so a leading `..` survives and is caught) and returns `path.Dir`, which cannot be
  absolute; the script re-checks with `realpath -m -- "$PWD/$STYRMAN_FMT_DIR"` against
  `case … in "$PWD"|"$PWD"/*`. `realpath -m` canonicalises symlinks, so a `.git` or a directory
  symlinked out of the workspace makes the case fail and `D` stays `$PWD`. `"$PWD"` in a case
  *pattern* is quoted, so a workspace name containing `*`/`[` is matched literally. The variable is
  never interpolated into the script text — it travels as an env var. The residual (the case tests
  the *resolved* path and `D` then uses the *unresolved* one; and `[ -d "$PWD/.git" ]` follows a
  symlinked `.git`) is guest-on-own-guest: the temp file is created by the guest user inside the
  guest, the host never opens it, and `internal/files`' `os.Root` confinement refuses any escaping
  link on the host side. Not a finding.
- **Ingress cookie regex.** Measured against Go/PCRE semantics:
  `a=1; AUTHP_ACCESS_TOKEN=t; b=2` → `a=1; b=2`; `a=1;AUTHP_ACCESS_TOKEN=t` → `a=1`;
  `XAUTHP_ACCESS_TOKEN=t; b=2` and `a=1; XAUTHP_ACCESS_TOKEN=t` → **unchanged** (the `(?:^|;\s*)`
  anchor means `^` only matches offset 0, so a differently-named cookie is never stripped); and
  `AUTHP_ACCESS_TOKEN_X=t` survives (the `=` must follow the name). The one cosmetic edge is the
  portal cookie *first*: `AUTHP_ACCESS_TOKEN=t; b=2` → `"; b=2"`, a leading empty pair, which RFC
  6265 parsers discard. Caddy's three-argument `request_header` treats the middle argument as a
  regexp, and Caddy's Caddyfile lexer preserves `\s` inside quotes (it only consumes a backslash
  before `"`, `'` or `\`), so the pattern reaches Go's regexp intact.
- **`cookie samesite lax` and the Google callback.** The directive exists and `lax` is a valid value
  (`caddy-security@v1.1.64/caddyfile_authn_cookie.go:92`, validated at
  `go-authcrunch@v1.1.41/pkg/authn/cookie/factory.go:82-89`); two-argument form sets the *default*
  domain config, so it applies. It reaches `AUTHP_ACCESS_TOKEN` (`cookie_get.go:49-52`); the referer,
  sandbox, identity and refresh cookies have their `SameSite` lines commented out upstream and are
  unaffected. The callback still works: go-authcrunch's OAuth provider has no `response_mode`/
  `form_post` anywhere in `pkg/idp/oauth/`, so Google returns via a **top-level GET navigation**,
  which carries Lax cookies; and the subsequent `auth.<base>` → `crew.<base>` hop is same-site, where
  SameSite does not apply at all. Note honestly that Lax therefore does **not** close the *same-site*
  preview-host vector from the audit — `SameOriginOrAbsent` is what does, and it is present.
- **Dotted redirect suffix — apex and ports.** `redirect_match.go:28` matches on `u.Host`, which
  includes a port, so `https://crew.<base>:8443/` fails the suffix — as it already did with the bare
  form; production serves 443 with no port in the URL, so no regression. The apex is not a served
  site (`labels.go:69-73` now refuses an empty hostname, so no block can be `.<domain>`), so refusing
  an apex redirect breaks nothing. Userinfo tricks (`https://x.<base>@evil.com/`) land on
  `u.Host = evil.com` and are refused.
- **`terminal.SameOriginOrAbsent`.** `strings.EqualFold(u.Host, r.Host)` — case-insensitive as hosts
  are; `Origin: null` parses to an empty `Host` and is refused (covered by the table test);
  a longer host (`crew.example.test.x`) is refused; an absent Origin is allowed, correctly, for
  non-browser clients. On the Host question: the ingress does **not** rewrite it for the styrman app
  — `generator.go:426,476` emit a bare `reverse_proxy`, which preserves the incoming Host, and the
  preview blocks set `header_up Host {host}` (`:151,:198`), also the client's. So `r.Host` is
  `crew.<base>` and matches the browser's Origin; the local dev and e2e listeners are
  `localhost:<port>` on both sides. The scheme is not compared (an `http://` Origin on the same host
  passes), which is immaterial behind an HTTPS-only ingress.
- **`path.Clean` before the guard exemptions.** The dangerous direction — a cleaned path that is
  *exempt* while the raw path *routes* to a guarded handler — cannot occur: Go's `ServeMux` compares
  `cleanPath(EscapedPath())` with the escaped path and **301s** anything non-canonical rather than
  routing it, so a routed request already has `Clean(p) == p` and cleaning changes nothing. The
  percent-encoded variant (`/api/%2e%2e/p/…`) is not textually cleaned by the mux, so it routes as
  the literal segment `%2e%2e` and matches no API pattern. The redirect is re-issued by the client
  through the full guard chain. No bypass, and the guards now judge the same string the router does.
- **Dockerfile pins.** Both new sums verified against the publishers today (see the table). The
  `tar --strip-components=1 -C /usr/local` extraction is preceded by `sha256sum -c -` under
  `set -eux`, so a substituted archive aborts the layer before `tar` ever runs.
- **`NewUUID`.** `crypto/rand` only; error propagated instead of a predictable `fallback-<nanos>` id;
  version (`0x40`) and variant (`0x80`) bits still set correctly.
- **`credstore` chmod.** `os.Chmod(baseDir, 0o700)` after `MkdirAll`, failing `New` if it cannot —
  correct shape (the per-user dir is the remaining half, noted above).
- **Broker log line — no secret.** `b.logger.Warn("cred broker: git credential read failed", "user",
  user, "host", host, "error", err)` logs an identity, a hostname and a store error. No token, no
  file body. (The predicate that decides *when* it fires is finding 2.)
- **No new dependencies.** `go.mod`, `go.sum`, `deploy/caddy/ingress/go.{mod,sum}`, `web/package.json`
  and `web/package-lock.json` are **unchanged** in this range — nothing to justify against
  stdlib-first.
- **`api.ts` signal ownership.** Dropping the unconditional `AbortSignal.any` means a caller that
  passes a bare `AbortController` signal loses the 20 s bound. Every in-repo caller passes an
  `AbortSignal.timeout(...)` or combines explicitly (`:506,518,523,627,659`), so no request is
  currently unbounded; the trap for a future caller is named in the comment at `:366`.
- **`.gitignore` / `web/.claude/`.** The two `agent-memory` files removed here were tracked by the
  BUG-006 accident; the ignore rule is the right close-out. No secret in either.

## Checked

- **Secrets** — no credential, token or key literal introduced anywhere in the range. `deploy/.env`
  is untracked and still gitignored; every new `ARG` is a version or a checksum; the new broker log
  line and the new `credentials.go` WARN lines carry identities and error strings only. The MOTD
  marker file contains a timestamp and the broker URL by construction (finding 7 covers what a guest
  can put there instead).
- **Injection** — the format handler's guest script takes its one variable through the environment
  and never interpolates it (cleared above); `styrman-ctl edit` now rejects a non-identifier KEY;
  the updater's git credential helper is unchanged and still reads `GH_TOKEN_FILE` from its own
  environment. Two shell-quoting residuals are on the record: finding 8 (the updater's RETURN trap)
  and `image/entrypoint.sh:165`, which this range did not touch.
- **Authz/authn** — the WebSocket entry points now check the origin before any side effect; the
  guard exemptions are computed on the cleaned path with no reachable divergence from the router;
  the portal's redirect trust is label-bounded and the session cookie is `SameSite=Lax`. Not closed:
  the `/link/` branch still calls `next.ServeHTTP` with no identity on the context, and an upgraded
  socket still outlives the JWT (`bridge.go:369-387`, untouched).
- **Crypto** — nothing hand-rolled in this range. `NewUUID` uses `crypto/rand`; the signature gate
  delegates to `git verify-tag` with an SSH allowlist and now an empty `GNUPGHOME`; session and OAuth
  handling remain entirely inside caddy-security/go-authcrunch.
- **Dependencies** — no manifest changed. `govulncheck` run through the project's own
  `deploy/vulncheck.sh` (the pinned `golang:1.25.13-trixie@sha256:6be3daa1…` builder, i.e. the
  toolchain that actually ships — a bare local run reports 30 extra stdlib findings only because
  this machine's Go is 1.25.0): root module **2** (`GO-2026-4883`, `GO-2026-4887`, both
  `github.com/docker/docker@v28.5.2+incompatible`, both `Fixed in: N/A`, both reached via `init`
  traces), `deploy/caddy/ingress` the same 2, `image/harness` clean. Both predate this range.
  `npm audit --omit=dev` in `web/`: **1 high**, `js-yaml@4.3.1` (GHSA-2883-xcg3-v3hh) through
  `@tanstack/react-start → start-plugin-core → xmlbuilder2`, build-time only, `fixAvailable: true`,
  unchanged by this range — the audit's recommendation to either `npm audit fix` or record the
  acceptance once is still open.
- **GCP surface** — none. This is a self-hosted deployment; `docs/deploy/gcp.md` records it as not
  applicable, and nothing in the range adds an IAM binding, a service account, a Cloud Run flag or a
  key file.
- **Test status at HEAD** — `go test ./...` bare: **rc=0**, all 15 packages ok (run as its own
  statement, status read before anything else, per `.agents/rules/verify-bare.md`).

## Not examined

Named honestly rather than implied.

- **`web/dist/`** — excluded by the dispatcher. The SPA is embedded via `go:embed`, so the built
  bundle is what actually ships; I did not verify that `dist` corresponds to the reviewed `web/src`.
- **`.agents/`** — excluded by the dispatcher. That means **ADR-069 is unverified**: five comments in
  this range cite it (orchestrator, credential snippet, Dockerfile) and I did not confirm it exists
  or says what they claim. The audit's "record the broker's blast radius in ADR-030/032" item is
  likewise unverified for the same reason.
- **The updater end to end.** I read `verify_tag` and the new test verb closely and reasoned about
  call ordering, but I did not execute `internal/config/styrman_update_test.go` in isolation, did not
  construct the four marker fixtures myself, and did not run the updater on a host. In particular I
  did not verify that moving `GIT_CRED_ARGS` and the `git()` wrapper to the top of the file leaves no
  `git` call between there and the `DEPLOY_DIR` assignment (I read the ordering; I did not trace
  every branch).
- **The generated Caddyfile was never fed to a real Caddy.** `cookie samesite lax` and the
  `request_header Cookie … ""` replacement are verified against the vendored libraries' source and
  Caddy's lexer semantics, not against `caddy validate`. An empty third argument is the one token I
  would want a real adapter run to confirm before deploy — a parse failure there is an ingress
  outage, not a security hole, but it is the cheapest thing left to check.
- **The runtime image was not built.** The Go and Node sums are verified against the publishers, but
  no `docker build` was run, so the four new base-image digests, the `agy` install path and the
  `claude-code@2.1.270` resolution are unverified in practice.
- **No integration or e2e run.** `.agents/docs/testing/styrman/integration.sh` (needs real Kata) and
  `web/e2e/run.sh` (needs Playwright) were not executed; in particular the audit's "add one
  integration check that a login shell materializes a credential" — the instrumentation blind spot
  that let BUG-054 ship — I could not confirm was added, since the test tree lives under `.agents/`.
- **`internal/httpapi/projectenv_stats_test.go`, `check_test.go`, `seam_test.go`,
  `snippet_test.go`, `config_test.go`, `clips.test.ts`** — new test files, read only for whether they
  assert the security property their commit message claims, not reviewed line by line.
- **`web/src/components/SidePanelGit.tsx`, `settings.tsx`, `icons.tsx`** — read for the diff's own
  changes (`treeChanged()` in `finally`, dead icon, stale JSDoc); no wider review of those files.
