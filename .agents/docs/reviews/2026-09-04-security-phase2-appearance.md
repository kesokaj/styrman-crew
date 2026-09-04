---
owner: security
date: 2026-09-04
---
# Security review — phase 2 appearance settings (fonts)   (2026-09-04)

Range reviewed: fe00412..working tree (uncommitted Phase 2 of the v1.22.0 UI plan)
plus the untracked new file `web/src/lib/appearance.test.ts`. Blast radius followed
outward from the two flagged surfaces: the `localStorage` parse in
`web/src/lib/hooks.ts` → every consumer of the resolved font stack
(`CodeEditor.tsx`, `TerminalPane.tsx`, `FileEditor.tsx`, `settings.tsx`,
`lib/terminal.ts`), the CSS-generation path underneath `EditorView.theme`
(`style-mod`), the question of *who can write that storage key* (the same-origin
port-preview proxy at `internal/httpapi/proxy.go`, the public-link handler at
`internal/httpapi/links_api.go`), the resize message the new font effect triggers
(`internal/terminal/bridge.go`), and the new npm package plus the committed
`web/dist` artifact that `go:embed` ships inside the server binary.

## Verdict: FINDINGS

**Nothing here is exploitable, and both of the specific concerns that brought this
diff to review came back clean.** The `localStorage` → CSS path is genuinely
closed, not accidentally closed: only the *key* comes from storage and it is
checked with `Object.hasOwn` against a three-entry object literal, so the string
that actually reaches CSS is always one of three module constants — an attacker
who controlled the stored value could at most select between "Google Sans Code",
"JetBrains Mono", and the system stack. Sizes are `Number.isFinite`-checked,
range-bounded to 8..32 and `Math.round`ed, so `${size}px` can only ever be an
integer literal. The new dependency ships no JavaScript at all and no install
hooks, and I verified byte-for-byte that the committed `dist` fonts are the
upstream files and that the shipped minified bundle contains the exact sanitiser
that is in the source — the embedded artifact is not a doctored build. What is
left is three low-severity hardening items, worst of which is that the "only
constants reach CSS" invariant is held by call-site convention rather than by the
type system: `CodeEditor`'s `fontFamily?: string` is an unconstrained string that
`style-mod` concatenates verbatim into a CSS rule with no escaping, so the guard
is one careless future call site away from being real.

## Findings

- **[low / confidence: medium]** `web/src/components/CodeEditor.tsx:161` (prop
  declaration) and `:139` (`editorFontTheme`) — the `fontFamily?: string` prop is
  typed as an arbitrary string and is interpolated straight into a CodeMirror
  theme. Underneath, `style-mod` builds the rule by raw concatenation
  (`web/node_modules/style-mod/src/style-mod.js:41`:
  `local.push(prop… + ": " + value + ";")`) with **no escaping of the value**, so
  a value containing `;` or `}` would emit additional CSS declarations or close
  the rule and open a new selector. Not exploitable today: the only two call
  sites are `FileEditor.tsx:414` (`FONT_STACKS[editorFontFamily]`, a constant
  selected by an `Object.hasOwn`-validated key) and the read-only credential
  viewer at `settings.tsx:716`, which passes nothing and gets
  `DEFAULT_EDITOR_FONT_FAMILY`. The weakness is that the invariant is enforced by
  convention at the call sites rather than by the type, in a component that is
  `lazy`-imported into the page that renders *secrets*. Remediation: type the
  prop `fontFamily?: FontFamilyPref` (importing the type from `lib/hooks`) and do
  the `FONT_STACKS[...]` lookup inside `CodeEditor`, so no arbitrary string can
  reach `EditorView.theme` no matter what a future caller passes. Same one-line
  treatment applies to `TerminalOptions.fontFamily` in `lib/terminal.ts:104`.

- **[low / confidence: medium]** `web/src/index.css:3-10` — the "self-hosted
  fonts, never a CDN" property is now asserted **only in a CSS comment**, and this
  diff correctly retracts the citation that used to back it (R9 was rewritten on
  2026-08-30 to mean "Deployment target" — I confirmed at `REQUIREMENTS.md:59`,
  so the old comment really was stale). Nothing numbered, tested, or linted stands
  behind it now: a future `@import url(https://fonts.googleapis.com/…)` would ship
  green, and the consequence is not cosmetic — it turns every page load into a
  third-party request that leaks the viewer's IP and referrer off the self-hosted
  box, on a platform whose whole premise is isolation. Remediation: one grep in
  `deploy/verify-commit.sh` next to the existing `dist` freshness step, failing if
  `web/dist/client/assets/*.css` contains an absolute `url(http…)`. I ran that
  check by hand and it is clean today (the only `https://` in the built CSS is a
  `tailwindcss.com` comment).

- **[low / confidence: low]** `web/src/lib/hooks.ts:302-306` — `useAppearance`'s
  docstring calls `APPEARANCE_DEFAULT` "the frozen default object", but it is a
  plain `const` and is not `Object.freeze`d; `storedAppearance()` returns that
  exact reference as the live store value on both the no-key and the throw paths,
  so the module-level `appearance` and the `getServerSnapshot` return value can be
  the same object identity. No mutation path exists today — `setAppearance`
  always rebuilds through `sanitizeAppearance` and reassigns — so this is
  hardening, not a defect. Remediation: `Object.freeze(APPEARANCE_DEFAULT)` makes
  the comment true and turns a future in-place mutation into a throw instead of a
  silently corrupted process-wide default that `useSyncExternalStore` would never
  re-render past.

## Checked

**Secrets** — clean. No credential-, token-, or key-shaped string anywhere in the
diff (`api[_-]?key|secret|token|password|private[_-]?key|BEGIN … PRIVATE|Bearer|
AKIA|ghp_|sk-` over the full diff: zero added or removed lines match). Nothing is
logged: the appearance value never reaches `console`, the API, or the WS. Worth
recording that `settings.tsx` — the file that gained the `FontRows` component — is
also the credential viewer, and the diff deliberately does *not* wire the user's
editor font into it (`CodeEditor` at `settings.tsx:716` still passes no font props
and takes the historical defaults), which is the right call and is documented at
`CodeEditor.tsx:164-171`.

**Injection** — the flagged path is closed. `sanitizeAppearance` reads exactly four
named fields out of the parsed object into a fresh literal, so a `__proto__` own
property (which `JSON.parse` *does* create, unlike assignment) is discarded and
there is no prototype-pollution route. `familyOr` uses `Object.hasOwn`, not `in`,
which the new test file exercises directly with `"toString"`; `Object.hasOwn` on
an object literal is also false for `"__proto__"`, so that key is rejected too.
`clampSize` rejects numeric strings, `null`, `true`, `[]`, `{}`, `NaN`,
`±Infinity` (including `1e400`, which `JSON.parse` yields as `Infinity`) and
anything outside 8..32. `setAppearance` re-sanitises the *merged* object, so the
patch path is guarded identically to the load path. The three `FONT_STACKS` values
are module constants and are the only strings that reach xterm's `fontFamily`, the
CodeMirror theme, and the React `style` prop at `settings.tsx:889`. Verified
against the real minified artifact, not just the source: the shipped bundle
contains `Number.isFinite(e)&&e>=8&&e<=32?Math.round(e):t` and
`Object.hasOwn(Ql,e)?e:t` (`web/dist/client/assets/index-3TzM9aL3.js`).

**Authz/authn** — untouched by this diff; no route, guard, middleware, or identity
code changed. The load-bearing adjacent question is *who can write
`styrman.appearance.v1`*, since the store trusts whatever is in it. Answer:
same-origin script only, and the two places project-controlled content is served
are both fenced off. The same-origin port preview sets
`Content-Security-Policy: sandbox allow-scripts allow-forms allow-popups
allow-modals allow-downloads` with **no `allow-same-origin`**
(`internal/httpapi/proxy.go:206-214`, N22), so a project's own web app runs in an
opaque origin where `localStorage` access throws; public links are served on their
own token subdomain, a separate origin by construction
(`internal/httpapi/links_api.go:168-171`). So the store's input is not
attacker-reachable without a pre-existing same-origin XSS in the SPA itself — at
which point the font store is not the weak link. Also checked the one server-side
effect the new code triggers: the font effect calls `onResizeRef.current?.()`,
which ships `{"type":"resize","cols":…,"rows":…}` into the guest, but
`controlMessage.Cols/Rows` are `uint16` (`internal/terminal/bridge.go:63-67`), so
the decode itself bounds them and an out-of-range value fails `json.Unmarshal` and
is ignored. The client-side `cols < 10 || rows < 3` floor
(`TerminalPane.tsx:290`) still applies. No new authority anywhere.

**Crypto** — none touched, none hand-rolled. No auth, hashing, session, or
JWT/OIDC code in the diff; nothing in scope for the global ban.

**Dependencies** — one added: `@fontsource-variable/jetbrains-mono@5.3.0`,
resolved from the public npm registry and pinned in `package-lock.json:587-593`
with a sha512 integrity hash. Justified against stdlib-first: a font face is not
something the stdlib supplies, and the alternative to bundling it is loading it
from a Google Fonts CDN, which is strictly worse for a self-hosted box. Supply-chain
surface is close to the minimum possible for an npm package: **zero runtime
dependencies, no `scripts` block (so no `preinstall`/`postinstall`/`prepare`
hook), and no JavaScript in the package at all** — the contents are `index.css`,
`wght.css`, `wght-italic.css`, an `index.d.css.ts` type stub, two `.scss` files,
`LICENSE` (OFL-1.1), three JSON metadata files, and twelve `.woff2` binaries.
Every `src: url(...)` in its CSS is relative (`./files/…`); the only `https://`
string anywhere in the package is inside a Sass `@warn` deprecation message. The
package is from the same `@fontsource-variable` publisher already trusted for the
two existing brand faces, so this adds no new maintainer to the trust set.
*Artifact integrity, verified rather than assumed* (the built `dist` is committed
and `go:embed`-ed into the server binary, so it is part of the shipped supply
chain): all five committed `web/dist/client/assets/jetbrains-mono-*.woff2` are
sha256-identical to the corresponding upstream files in `node_modules`; the sixth
subset (cyrillic-ext, 2 028 B) is absent because Vite inlined it as a base64
`data:font/woff2` URI under its asset-inline limit, which accounts for the
apparent 6-declarations/5-files mismatch; every `src:url()` in the built
`index-Cmz7gMfZ.css` is a relative `/assets/…` path — **no CDN, self-hosting
holds**; and `_shell.html` differs from HEAD only in asset hash names. Tooling:
`npm audit` → **found 0 vulnerabilities**. `bash deploy/vulncheck.sh` (govulncheck
inside the pinned `golang:1.25.13-trixie` builder, the canonical run per
`AGENTS.md`) → root module and `deploy/caddy/ingress` each report the two accepted
`github.com/docker/docker` vulnerabilities `GO-2026-4887` and `GO-2026-4883`
(`Fixed in: N/A`), which are the pre-existing tracked baseline recorded at
`TODO.md:62` and unchanged by this diff (**no Go file is touched**);
`image/harness` → no vulnerabilities. Caveat worth recording so it is not
misread later: a bare `govulncheck ./...` against *this host's* toolchain
(go1.25.0) additionally reports 31 stdlib findings — those are the local
toolchain, not the shipped binary, which is built with 1.25.13; the pinned-image
run is the authoritative one. Supporting verification: `npm run check` (tsc -b),
`npm run lint` (oxlint) and `npm test` all green, 33/33 tests across four files.

**GCP surface** — not applicable and not touched. styrman is self-hosted
(`.agents/docs/deploy/gcp.md` records GCP as N/A); no IAM binding, service
account, Cloud Run flag, Secret Manager reference, or credential JSON appears
anywhere in the diff.
