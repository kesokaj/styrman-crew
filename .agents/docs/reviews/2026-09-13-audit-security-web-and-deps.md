---
owner: security
date: 2026-09-13
---
# Security audit — frontend (`web/src`) and dependency surface   (2026-09-13)

Range reviewed: working tree at `e56e775708a9062cc6432f5c3278770bed63b685`
(`git status` clean at the start of the review; every `path:line` below is
pinned to that commit).

Surface in scope, as dispatched: `web/src` XSS sinks
(`dangerouslySetInnerHTML`/`innerHTML`, `href`/`src`/`srcdoc` built from data,
`javascript:`/`data:` handling, the URL-bar normalisation in
`web/src/lib/browserInput.ts`), URL building in `web/src/lib/api.ts` (the `u`
template tag and every call site), WebSocket URL and close handling, the preview
iframe / `postMessage`, `localStorage` reads, error and notice text rendered from
server or guest data, and `DiffView`'s bidi/control reveal; plus the dependency
surface — `go.mod` for the root module and `deploy/caddy/ingress` (and the third
module, `image/harness`, which the dispatch did not name), `deploy/vulncheck.sh`,
`npm audit` with and without dev, and the base-image/tool pinning in
`image/Dockerfile`, `deploy/Dockerfile.server` and `deploy/caddy/Dockerfile`.

## Verdict: FINDINGS

No exploitable XSS and no URL injection on the frontend surface. The single
`dangerouslySetInnerHTML` is fed by micromark with `allowDangerousHtml: false`
(and `allowDangerousProtocol` left unset, so its `protocolHref` allowlist drops
`javascript:`); every `/api` and `/ws` path interpolation goes through the `u`
tag; the only `window.open` sits behind xterm's http/https-only link regex with
`noopener,noreferrer`; there is no iframe, no `postMessage` sender or listener
and no `srcdoc` anywhere in `web/src`; and every storage read is re-parsed
through a constructing sanitiser inside a `try`.

The defects are on the **build-time supply chain of the runtime image**.
`image/Dockerfile` resolves the Go toolchain, Node, the Antigravity CLI and
`@anthropic-ai/claude-code` from unpinned, unverified network fetches — two of
them `curl … | bash` as root, one an unpinned `npm install -g` with lifecycle
scripts enabled — in the same file whose own comment at `:55-56` states "Every
remaining tool is pinned by version AND verified by sha256 (audit 2026-09-02)".
That claim is false for four fetches, the image it produces is the one every
project microVM boots into with live brokered credentials, and the
non-determinism it creates is exactly what ADR-053 and `versioning.md`'s
rollback guarantee exist to prevent.

## Findings

- **[medium / confidence: high]** `image/Dockerfile:89-97` — four unpinned,
  unverified fetches execute as root at image build:
  - `:89` `GO_VERSION=$(curl -fsSL 'https://go.dev/dl/?mode=json' | jq -r '.[0].version')`
    then `:90` `curl -fsSL "…${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz`
    — "whatever is newest today", no checksum, the tarball piped straight into
    `tar` as root without ever landing on disk where it could be verified.
  - `:92` `RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash -` — an
    unauthenticated remote shell script run as root, which is verbatim the
    pattern the comment at `:56-58` says was audited out of this very file for
    OpenTofu on 2026-09-02.
  - `:96` `RUN npm install -g @anthropic-ai/claude-code` — no version pin and no
    `--ignore-scripts`, so every transitive dependency's install lifecycle
    script runs as root at build time. The same file does this correctly one
    block down at `:123`
    (`npm install -g --ignore-scripts "chrome-devtools-mcp@${CHROME_DEVTOOLS_MCP_VERSION}"`),
    with a comment explaining why — so the standard is known and simply not
    applied here.
  - `:97` `curl -fsSL https://antigravity.google/cli/install.sh | bash` — the
    second root `curl | bash`.

  Scenario: a compromise of any one of those four endpoints, or a single
  malicious publish anywhere in the `@anthropic-ai/claude-code` dependency tree,
  executes attacker-chosen code as root during the build and ships it inside
  `styrman-runtime`. Every project microVM then runs that image while the
  credential broker hands it the operator's *current* git/gh/ssh/gcloud/claude
  credentials (ADR-030/032, `AGENTS.md` "sign in once, every project stays
  signed in") — so the blast radius is every user's live credentials, not one
  project, and the compromise is invisible to the reviewer because the
  Dockerfile text does not change. Secondary harm: two builds of the same
  `STYRMAN_VERSION` are not the same image, so the auto-updater's rollback
  cannot restore what actually ran — the precise failure the `hadolint` comment
  at `:50-53` was written against.

  Remediation, all four in the style this file already uses for tofu/yq/hadolint
  at `:75-88`: `ARG GO_VERSION=` + `ARG GO_SHA256=` with `sha256sum -c -` before
  the `tar`; replace the NodeSource script with its pinned keyring + apt source
  (or Debian's own `nodejs` package, as `chromium` is already handled at `:121`);
  `npm install -g --ignore-scripts "@anthropic-ai/claude-code@<version>"`; fetch
  the `agy` binary by pinned version + sha256 rather than running its installer.
  Then correct `:55-56`, which currently reads as an assurance that these four
  do not exist — a reviewer who trusts that line skips the block below it.

- **[low / confidence: high]** `deploy/Dockerfile.server:9` and `:19`, and
  `image/Dockerfile:8` and `:15` — base images are tag-pinned but **not**
  digest-pinned: `golang:1.25.13-trixie`, `debian:trixie-slim`, `debian:trixie`.
  `debian:trixie-slim` is a moving tag by design: it is republished on every
  point release and security rebuild. The vendored ingress next door does the
  opposite and writes down why — `deploy/caddy/Dockerfile:8` and `:23` pin
  `caddy:2.11.4-builder@sha256:e04bd0aa…` and `caddy:2.11.4@sha256:df7f1c2f…`
  with the comment "a tag is mutable, so two builds of the same styrman version
  could otherwise ship different Caddy binaries — which is exactly what the
  no-mutable-tags rule (ADR-053) exists to prevent, one layer down". The same
  argument applies unchanged to the server and runtime images; the rule is
  currently honoured in one of three Dockerfiles. Scenario: a rollback to the
  previous styrman tag rebuilds against a base that has moved and does not
  restore the artifact that actually ran, and a registry-side tag repoint
  changes what ships with no diff to review. Remediation: append `@sha256:…` to
  all four `FROM` lines, bumping tag and digest together as the Caddy file
  already instructs.

- **[low / confidence: high]** `web/src/components/BrowserPane.tsx:327-334`
  (`upsert`) — the guard checks `t.type !== "page"` and
  `typeof t.targetId !== "string"`, then stores `url: t.url` and `title: t.title`
  (`:331`) with no type check, out of a `TargetInfo` that is a **cast** of relay
  JSON (`:45`, and the casts at `:352` and `:354`). Both fields are then rendered
  as React children at `:715` (`{t.title || t.url || "New tab"}`) and inside a
  template at `:718` (`aria-label={\`Close tab ${t.title || t.url}\`}`). A
  non-string `title` in a React child position throws ("Objects are not valid as
  a React child") and takes the pane down for the session. This is **not** XSS —
  React escapes both positions — and the relay is owner-only, so the realistic
  actor is the project's own compromised guest Chromium or a malicious agent
  driving it over MCP: a self-inflicted pane DoS, bounded to one user's own
  project. It is recorded because the file's own standard, stated 26 lines
  earlier at `:301` ("Shapes are checked, not cast: a frame the guest's browser
  sends is guest-controlled input (security review, round 3)"), is fully applied
  to `Page.screencastFrame` (`:296-314`) and `Page.javascriptDialogOpening`
  (`:317-324`) and only half-applied here. Remediation: two more
  `typeof … === "string"` tests in `upsert`, or `String(t.url ?? "")`.

- **[low / confidence: medium]** `web/src/lib/api.ts:867-868` — `portUrl` is the
  one exported URL builder in the file that interpolates without the `u` tag:
  `` `/p/${project}/${term}/port/${port}/` ``. Its two consumers put the result
  straight into an `href` (`web/src/components/SidePanelPorts.tsx:69`) and into a
  copied shareable link (`:91`, `location.origin + portUrl(...)`). `u`'s own doc
  comment at `:428-432` names precisely this weakness — "browsers resolve `..`
  and `?` before sending, so an id containing them can point a clicked action at
  a DIFFERENT /api/projects/... path of the SAME user" — and every other path in
  the file obeys it. Scenario: the route params `$project`/`$term` are decoded by
  TanStack Router, so a victim lured to `/p/a%2F..%2Fb/<term>` yields
  `project = "a/../b"` and an anchor whose href the browser resolves to
  `/p/b/<term>/port/3000/`, i.e. a clicked action pointing somewhere the caller
  did not ask for. Confidence is medium, not high, because the preconditions are
  narrow: the target stays same-origin and same-user (no tenant crossing), and
  the route's own loader calls `u`-encoded endpoints that 404 on such an id, so
  the Ports panel may not render at all on the crafted route — I did not confirm
  the panel is reachable in that state, and did not test it. Remediation is a
  one-line consistency fix: `` u`/p/${project}/${term}/port/${port}/` ``.

- **[low / confidence: medium]** `web/src/components/DiffView.tsx:10` — the
  `INVISIBLE` class covers C0/C1 controls, the bidi overrides/isolates
  (U+202A–U+202E, U+2066–U+2069) and LRM/RLM/ALM, but not the zero-width
  family: U+200B ZWSP, U+200C ZWNJ, U+200D ZWJ, U+2060 WORD JOINER, U+FEFF,
  U+00AD SOFT HYPHEN. "Trojan Source" has an *invisible character* variant
  alongside the reordering one, and the comment at `:1-7` frames this guard as
  "this pane's whole job is reviewing a change BEFORE you commit it, and a
  reviewer must not be shown a reordered line". A zero-width joiner inside an
  identifier is the same class of lie about what the file contains, in the pane
  the operator uses to approve agent-written code before it is committed.
  Confidence medium because the reordering variant — the one that actually
  rewrites reading order — *is* covered, so this is hardening, not a hole.
  Remediation: extend the class with U+00AD, U+200B–U+200D, U+2060 and U+FEFF; the
  cases belong in the existing `web/src/lib/diffReveal.test.ts`.

- **[low / confidence: high]** `web/package.json` / `web/package-lock.json` —
  `npm audit --omit=dev` reports 1 high: `js-yaml@4.3.1`, "maxTotalMergeKeys does
  not limit CPU use for empty merge sources", reached only through
  `@tanstack/react-start → @tanstack/start-plugin-core → xmlbuilder2 → js-yaml`.
  That is a build-time path, never shipped to the browser and never fed operator
  or guest YAML, so practical exposure here is nil — but it is the one non-clean
  line `npm audit --omit=dev` prints, and `fixAvailable: true`. Scenario worth
  naming: it is a *production*-classified entry, so the next reviewer running the
  security row sees "1 high" and must re-derive the same conclusion from scratch.
  Remediation: `npm audit fix` (lockfile-only bump) or an accepted-risk line
  recorded beside the govulncheck acceptances so the decision is made once.

## Checked

- **Secrets** — no credential, token or key literal in `web/src`, in
  `web/package.json`, or in any of the three Dockerfiles (every `ARG` is a
  version or a checksum). `web/src/lib/clips.ts:1-9` deliberately uses
  `sessionStorage`, not `localStorage`, with the reason written down ("copy-on-
  select means selecting an OAuth code or token during setup would land here …
  deliberately not durable") — R10 honoured at the design level, not just the
  grep level. The credential UI (`web/src/routes/settings.tsx:665-745`) holds
  secret values in React state for a read-only viewer and persists nothing
  client-side. `deploy/vulncheck.sh:29-35` bind-shadows `deploy/.env` with
  `/dev/null` before running a downloaded tool, and does so conditionally so a
  clean checkout can still run the gate. Nothing echoes a secret into a log or
  an error string on this surface.
- **Injection** — every `/api/...` and `/ws/...` path in `web/src/lib/api.ts`
  interpolates through the `u` tag (`:433-438`, `encodeURIComponent` per value);
  the file-scope helpers that must append a query string do it **outside** the
  tag with `encodeURIComponent` on each value (`:591`, `:595`, `:598`, `:609`,
  `:628`, `:633`, `:638`, `:642`, `:654`, `:695`, `:699-700`, `:780`), which is
  what the tag's doc comment requires. `browserSocketUrl` (`:877-880`) and
  `termSocketUrl` (`:883-907`) build from `location.protocol`/`location.host`
  plus a `u`-tagged path — no attacker-supplied origin can enter. The one
  exception found is `portUrl` (`:867-868`), filed as a low above. No `eval`,
  `new Function`, `innerHTML`, `outerHTML`, `insertAdjacentHTML`,
  `document.write` or `srcdoc` anywhere in `web/src`.
- **XSS / URL schemes** — exactly one `dangerouslySetInnerHTML`
  (`web/src/components/FileEditor.tsx:408-410`), fed only by
  `micromark(text, { allowDangerousHtml: false })` (`:156`). Verified in the
  installed dependency rather than assumed: `micromark/dev/lib/compile.js:59`
  defines `protocolHref = /^(https?|ircs?|mailto|xmpp)$/i` and applies it at
  `:816` and `:1134` unless `allowDangerousProtocol` is set, which this call site
  does not set — so `[x](javascript:…)` is dropped, and raw HTML is escaped. The
  `.replaceAll("<a href=", …)` rewrite at `:156-159` cannot be reached by file
  content, because micromark has already escaped every literal `<` by then.
  `normalizeUrl` (`web/src/lib/browserInput.ts:186-196`) admits only `http:`,
  `https:` and `about:blank`, refusing any other scheme with a negative lookahead
  that still lets `localhost:8080` through;
  `web/src/lib/browserInput.test.ts:129-131` covers `javascript:alert(1)`,
  `javascript://%0aalert(1)` and `data:text/html,…`. The only `window.open`
  (`web/src/lib/terminal.ts:150-153`) is xterm's web-links handler, whose bundled
  regex is `(https?|HTTPS?):[/]{2}…` with an origin-prefix re-check, opened
  `_blank` with `noopener,noreferrer`. Guest-derived text is rendered as React
  children throughout: the Browser pane's dialog notice
  (`BrowserPane.tsx:317-324`, type and message both type-checked, message cut to
  200 chars), the tab strip, the git panel's output via `toast()` →
  `ui.tsx:397` (`<span>{t.text}</span>`), and `DiffView`. No iframe and no
  `postMessage` in `web/src` at all — the port preview is a **server-side proxy**
  on a same-origin path (`api.ts:867-868`), not an embedded frame, so there is
  no sandbox attribute to get wrong and no frame-messaging boundary to audit.
- **Authz/authn (frontend share)** — the client holds no session material; the
  ingress identity header is the server's source of truth. The 401 /
  `opaqueredirect` path (`api.ts:381-385`, and the upload copy at `:658-667`)
  sets `redirect: "manual"` *after* the caller's spread "so no caller can unset
  it by accident", which is BUG-001's fix still in force; the reload counter in
  `sessionStorage` (`:324`, `:345`, `:399`) is deferred and guarded against the
  documented reload loop. `ProjectNetwork.canEnable` (`:33-35`, `:486`) is read
  from the server rather than inferred — BUG-019's rule, still held. Close codes
  4002 (stopped) and 4003 (revoked) suppress auto-reconnect
  (`api.ts:912-916`, `BrowserPane.tsx:26-35`). Consistent with R8; no
  client-side authorization decision was found that the server does not repeat.
- **Crypto** — none in `web/src`. Nothing hand-rolled: no token minting, no
  hashing, no signature check, no session or JWT handling.
  `web/src/lib/sshKey.ts` only parses and validates a public-key string for
  display; it generates nothing and verifies nothing.
- **Storage parsing** — every read is inside a `try` and re-validated by a
  constructing sanitiser, so no prototype key and no hand-edited value survives:
  `editorPane.ts:274-278` (`sanitizeRecents(JSON.parse(raw))`),
  `hooks.ts:276-281` (`sanitizeAppearance`, re-applied to the merged object on
  every write at `:288`), `recentsPanel.ts:33-39` (`clampRecentsHeight`),
  `Sidebar.tsx:35-40` (`new Set(JSON.parse(raw))`, non-iterable input caught),
  `FileEditor.tsx:72`/`:108` (`"off"`/`"on"` compare), `hooks.ts:118` and the
  inline boot script at `routes/__root.tsx:30` (theme compared against the
  literals `'light'`/`'dark'` before it is applied to `dataset.theme`).
- **Dependencies** — root `go.mod`: 3 direct (`coder/websocket`,
  `docker/docker`, `golang.org/x/sys`). `deploy/caddy/ingress/go.mod`: 2 direct
  (`caddyserver/caddy/v2`, `docker/docker`). Both minimal and justified —
  ADR-045 records the deliberate decision to stay on the pinned docker/docker SDK
  ("moby SDK migration — pinned docker/docker works; churn with zero user
  value"), and Caddy is R8's ingress. `web/package.json` is
  CodeMirror/xterm/TanStack/micromark/Tailwind: no crypto, auth, sanitiser or
  HTTP-client library rolled in, and no addition on this surface to justify.
  `bash deploy/vulncheck.sh` **ran here** and exited **3**: GO-2026-4887 (Moby
  AuthZ plugin bypass on oversized request bodies) and GO-2026-4883, both in
  `github.com/docker/docker@v28.5.2+incompatible`, both **"Fixed in: N/A"**, in
  the root module and the ingress module; `image/harness` clean. Every reported
  trace is a client-side `init` or `client.Client.*` call, not the daemon-side
  AuthZ middleware the advisories describe, and the script's own header
  (`:10-13`) pre-declares exactly these two as an accepted, no-fix-available
  report rather than a build gate. No new or previously unrecorded Go
  vulnerability. `(cd web && npm audit --omit=dev)` and `npm audit`: 1 high each
  — the same build-time `js-yaml` entry filed above — and zero critical, zero
  moderate, zero low, over 301 resolved packages.
- **GCP surface** — none on this surface, as expected: styrman is self-hosted
  and `.agents/docs/deploy/gcp.md` records GCP as not applicable. No IAM binding,
  service account, `--allow-unauthenticated`, Secret Manager reference or
  service-account key file appears in `web/src` or in any of the three
  Dockerfiles. The only Google touchpoints are the `cloud.google.com/apt`
  keyring fetched over TLS into `/etc/apt/keyrings` at `image/Dockerfile:43-45`
  (correct handling) and OAuth at the ingress, which belongs to R8 and to the
  sibling auth audit.
- **Bug index** — `.agents/docs/bugs/INDEX.md` read in full. The entries
  touching this surface are all still honoured in the reviewed code: BUG-001
  (`redirect: "manual"` + `opaqueredirect`), BUG-019 (`canEnable` as its own
  server field), BUG-020 (`LocalNetwork` optimistic-override yielding),
  BUG-022 (`setProjectLan` timeout derived from the server's bound),
  BUG-043/044 (BrowserPane mount latch keyed by identity; hidden pane pushes no
  viewport — the `visible` prop at `:61`, `visibleRef` at `:92`/`:210`/`:446`,
  and `applyViewport`'s `width === 0 || height === 0` guard at `:132`). BUG-007's rule was applied to this
  review: `file(1)` on every reviewed source file reports UTF-8 text, so nothing
  here was read through a "Binary files differ" blind spot.

## Cleared on inspection

- `web/src/lib/api.ts:433-438` `u` tag and all ~60 call sites — every path value
  encoded, every query string built outside the tag with `encodeURIComponent`.
  The one un-tagged builder (`portUrl`, `:867-868`) is filed as a low above.
- `web/src/lib/api.ts:877-907` WebSocket URL construction and `:912-916` the
  close-code contract; no auto-reconnect on 4002/4003.
- `web/src/lib/browserInput.ts:186-196` `normalizeUrl` — scheme allowlist with
  test coverage for the bypass shapes; neither the `:PORT` shortcut nor the
  bare-host fallback can smuggle a scheme past it (an interior newline or tab
  ends up behind the `http://` prefix, not in front of it).
- `web/src/lib/cdp.ts` — the hand-written CDP client. Guest-controlled frames
  are handled defensively throughout: non-string frames dropped (`:105`),
  `JSON.parse` in a `try` with malformed frames dropped not fatal (`:115-118`),
  `null`/non-object rejected (`:119`), `id`/`method`/`sessionId`/`error.code`/
  `error.message` each `typeof`-checked before use, and handlers held in a `Map`
  (not an object), so an event named `__proto__` cannot reach `Object.prototype`.
  Every pending call rejects on close. No finding.
- `web/src/lib/terminal.ts:130-153` — OSC 52 clipboard **read denied**
  (`readText: () => ""`), write allowed only for the `"c"` selection; web links
  restricted to http/https with `noopener,noreferrer`. `:407-413` reads the
  clipboard only on an explicit Ctrl+V keypress, i.e. behind the browser's own
  user-gesture and permission gate.
- `web/src/components/FileEditor.tsx:156-159, 408-410` — markdown preview
  (see Checked → XSS for the micromark verification).
- `web/src/components/BrowserPane.tsx:296-324` — `Page.screencastFrame` and
  `Page.javascriptDialogOpening` parameter validation, and the
  `data:image/jpeg;base64,` canvas draw at `:278` (a `data:` URL assigned to an
  `Image`, decoded onto a canvas — not a navigation and not a script context).
- `web/src/components/DiffView.tsx:25-48`, `DiffPane.tsx:127`,
  `SidePanelGit.tsx` — guest git output and diffs rendered as React children
  throughout; no HTML sink on that path.
- `web/src/lib/toast.ts` + `web/src/components/ui.tsx:380-406` — server and guest
  error strings reach the DOM as `{t.text}` in a `<span>`.
- All `localStorage` / `sessionStorage` readers (enumerated under Checked).
- `deploy/caddy/Dockerfile:8, 23` — digest-pinned on both stages, xcaddy plugins
  pinned by version; the model the other two Dockerfiles should follow.
- `deploy/vulncheck.sh` — builder image derived from the server Dockerfile's own
  `FROM` line (single source of truth), `govulncheck@v1.7.0` pinned,
  `GOTOOLCHAIN=local`, read-only source mount, `.env` shadowed conditionally,
  `go install` rather than `go run` so exit 3 stays distinguishable from a
  broken scan, and worst-exit-status-wins across modules. No defect found.
- `image/Dockerfile:75-88` (tofu / yq / hadolint) and `:120-123`
  (`chrome-devtools-mcp`) — version-pinned, sha256-verified, or installed with
  `--ignore-scripts`. These are the correct pattern; the finding above is that
  four fetches in the same file do not follow it.

## Not examined

- **Server-side Go** beyond the endpoints the frontend calls:
  `internal/httpapi`, `internal/credbroker`, `internal/credstore`,
  `internal/orchestrator`, `internal/files`, `internal/gitrepo` — out of this
  dispatch and covered by the sibling audit files of the same date
  (`2026-09-13-audit-security-auth-ingress-csrf.md`,
  `-audit-security-credentials-broker.md`, `-audit-security-guest-paths-files.md`,
  `-audit-security-root-scripts-image.md`). Claims those files make were not
  re-verified here.
- **The Caddy ingress Go code** (`deploy/caddy/ingress/*.go`) — only its `go.mod`
  and Dockerfile were read; the code is
  `2026-09-13-audit-review-caddy-ingress.md`.
- **`web/e2e/`, `web/scripts/normalize-shell.mjs`, `web/vite.config.ts`** and the
  build pipeline — not read. In particular the committed `web/dist/client` bundle
  was **not** diffed against a fresh build, so "the embedded assets correspond to
  this source" is unverified here; given that `go:embed` ships `dist/client`,
  that gap matters and belongs to whoever owns the build surface.
- **The transitive npm tree beyond `npm audit`** — no manual review of the 301
  resolved packages, no lockfile-integrity check, and no inventory of which of
  them run install scripts on the developer's machine.
- **`image/harness`** (the third Go module) — govulncheck ran on it and reported
  clean; its source was not read.
- **Runtime behaviour** — nothing was executed against a live server. No browser
  session, no DOM inspection, no attempt to reach the crafted-route precondition
  in the `portUrl` finding. All frontend findings are from source reading.
- **Response headers / CSP** — whether the server sets a Content-Security-Policy
  that would blunt a future XSS was not checked; it belongs to the httpapi
  surface. Noted only that `web/src` itself contains no
  `<meta http-equiv="Content-Security-Policy">`, so any CSP must come from the
  server or the ingress.
- **Whether the two accepted govulncheck findings are recorded for this
  release** — `deploy/vulncheck.sh:10-13` pre-declares them as accepted per
  release, but I did not verify that `2026-09-13-preflight-v1.29.0.md` or the
  changelog actually carries that acceptance line for v1.29.0.
