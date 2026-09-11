---
owner: security
date: 2026-09-11
---
# Security review — browser view, phase 1 (guest image)   (2026-09-11)

Range reviewed: 442a21c..e7f7857 (the single commit `e7f7857`, which is HEAD)

Blobs pinned as read (the working tree moved DURING this review — see "Working
tree drift" below; every finding is stated against these blobs):

| path | blob at e7f7857 |
|------|-----------------|
| `image/configs/styrman-browser` | `89dc29a` |
| `image/Dockerfile` | (diff hunk only) |
| `image/entrypoint.sh` | (diff hunk only) |
| `internal/httpapi/proxy.go` | `8fc45e8` |
| `internal/httpapi/shares.go` | `a1fdf56` |
| `internal/orchestrator/ports.go` | `5b9d87e` |
| `internal/config/config.go` | `f7ee88e` |

## Verdict: FINDINGS

**The commit opens an unauthenticated, full-control Chrome DevTools endpoint on
the project's bridge address and ships none of the four refusals ADR-068 D3
promises to protect it.** At `e7f7857` there is no `config.BrowserPort`, no
`previewPortAllowed`, and no port filter in `ListeningPorts` — `git grep
BrowserPort e7f7857 -- '*.go'` and `git grep previewPortAllowed e7f7857 --
'*.go'` both return nothing; the only hits are the *plan's* unticked Phase-2
boxes. Meanwhile every pre-existing port-exposure path accepts any port in
1024–65535: the Ports panel lists it, `proxyPort` proxies it to the owner *and
to any shared Google email*, and `createLink` mints a **no-login, internet-
reachable** TTL link for it. A DevTools client is not a web page — it is remote
control of the browser process, including `file://` reads inside the guest,
which is where the credential broker (R3) materialises the user's gcloud ADC,
ssh key, agy OAuth token and env tokens. Those credentials are shared across
*all* of that user's projects, so one link turns one project into every project.
This is exactly the BUG-035 shape the repo already has a rule for: the mechanism
("9222 is owner-only") exists as prose in an ADR and reaches zero callers.

## Findings

- **[high / confidence: high]** `image/configs/styrman-browser:60` (socat
  publishes `:9222` on `0.0.0.0`) reachable through `internal/httpapi/proxy.go:42-46`,
  `internal/httpapi/shares.go:110-116` and `internal/httpapi/links_api.go:66` —
  **the DevTools port is previewable, shareable and publicly linkable.** All
  three gates validate only `1024 <= port <= 65535`; 9222 passes. `serveLink` is
  `authGuard`-exempt by design (`internal/httpapi/links_api.go:149-151`,
  `auth.go:38`), so a public link needs no Google login at all.
  Scenario, no exploit needed: the browser starts on demand — and the MCP
  registration this same commit adds makes the *first agent tool call* start it
  — after which the Ports panel shows `9222 socat` as an ordinary listening
  port with no warning beside it. The owner shares or links it exactly as they
  would a dev server. The visitor now speaks HTTP and (because
  `httputil.ReverseProxy` passes upgrades through) WebSocket to a browser-level
  DevTools endpoint with no credential of any kind. Note the proxy *removes*
  Chromium's own last defence: `pr.SetURL(target)` rewrites the outbound Host to
  `<guest-ip>:9222`, an IP literal, which is precisely what Chromium's
  anti-DNS-rebinding host check accepts — the check cannot fire on a proxied
  request. Remediation: land the `previewPortAllowed` helper (already specified
  in the plan) in `proxyPort`, `setShares` and `createLink`, and the
  `config.BrowserPort` filter in `ListeningPorts`, **in the same commit that
  first opens the listener** — not a phase later. Until then the image must not
  ship: a released runtime image with the listener and a server without the
  guard is the vulnerable combination, and the two are versioned separately
  (`versioning.md`'s three-image lineup).

- **[high / confidence: medium]** `image/configs/styrman-browser:64` —
  `--remote-allow-origins=*` disables the Origin allowlist on the DevTools
  WebSocket upgrade. This is the control that stops *a web page* (as opposed to
  a process with a raw socket) from driving the browser, and it is switched off
  globally. It converts finding 1 from "an attacker with a TCP socket" into
  "any page loaded in any browser that can reach the port", which is exactly the
  public-link visitor's situation, and also the in-guest Chromium itself when an
  agent browses a hostile page. The flag is not needed for the documented
  design: the server's relay (ADR-068 D5) is a Go WebSocket client that chooses
  its own `Origin` header, so a single fixed origin — or none — suffices.
  Remediation: replace `*` with the exact origin the server's relay sends, and
  assert it in the Phase-2 relay test.

- **[medium / confidence: high]** `image/configs/styrman-browser:59-69` — the
  endpoint has **no authentication at all**, and socat deliberately widens
  Chromium's loopback-only bind to every interface. The launcher's own header
  (lines 7-15) calls 9222 "one trust domain" with the project's processes and
  inner containers; ADR-068 D3 repeats it. That is an assumption, not a control.
  Scenario: a developer `docker run`s an untrusted image inside their project's
  dockerd — the ordinary workflow this platform exists for. That container has
  no volume mounts and, on its own, no path to the home. It *can* reach the
  guest's bridge IP on 9222, and through CDP `file://` navigation it reads the
  project home, i.e. the broker-materialised credentials. A boundary a developer
  reasonably believes in (a container without mounts cannot read `~/.ssh`)
  silently stops existing the moment the browser is running. Remediation: bind
  socat to the bridge address only and add `range=`/`tcpwrap` restricting the
  source to the styrman server's address, or drop socat entirely and have the
  server's relay reach Chromium's loopback listener through `docker exec`
  (ADR-002's existing seam) rather than over the network.

- **[medium / confidence: medium]** `image/configs/styrman-browser:59-69` +
  `image/entrypoint.sh:151-157` — **the agents and the operator share one
  browser profile, and nothing constrains which origins an agent may drive.**
  ADR-068 D1 makes this explicit and the operator approved it, so this is
  recorded as an accepted-risk boundary rather than a defect: once the operator
  signs in to anything in the Browser pane, the session cookie lands in
  `~/.styrman/browser/profile`, and `claude`/`agy` — now registered as MCP
  clients of that same browser — can navigate to that origin and read the
  authenticated response. A prompt injection on any page the agent is asked to
  test is therefore an exfiltration path for every session in that profile. The
  design has no origin allowlist and no separate agent profile. At minimum this
  belongs in the operator-facing guide as "do not sign in to anything in the
  Browser pane you would not hand the agent"; a stronger fix is a second profile
  (or a CDP target allowlist in the relay) so the agent and the operator do not
  share cookies.

- **[medium / confidence: medium]** `image/Dockerfile:113-117` — `npm install -g
  "chrome-devtools-mcp@${CHROME_DEVTOOLS_MCP_VERSION}"` pins the *top-level*
  version only. There is no lockfile and no `--ignore-scripts`, so (a) the whole
  transitive tree resolves by semver at every image build — two builds of the
  same styrman version can contain different code — and (b) any transitive
  dependency's `preinstall`/`postinstall` script runs **as root inside the image
  build**. The resulting binary then runs as the project user with full CDP
  control and read access to the credential-bearing project home. The same
  pattern already exists two lines up for `claude-code`/`agy`, so this is a
  widening of accepted practice rather than a new class — but it is the first
  time the installed package is handed a capability this large. Remediation:
  `npm install -g --ignore-scripts` plus a committed lockfile or
  `--install-strategy=shallow` with an integrity-pinned tarball URL; at minimum
  record the resolved tree (`npm ls -g --all`) into the image so drift is
  detectable.

- **[low / confidence: high]** `image/configs/styrman-browser:44-49` —
  `mkdir -p`, `chmod 700`, `rm -f "${PROFILE}/Singleton*"` and `: > "${LOG}"`
  all operate by name on paths under a guest-writable home and all follow
  symlinks. No privilege boundary is crossed (same uid, and the guest already
  owns everything reachable), so the impact is self-inflicted only — but
  `~/.styrman/` also holds `logs/<projectId>/`, the transcripts the *server*
  reads and serves for history search, and `: > "${LOG}"` through a planted link
  truncates whatever it points at. The repo's own `guest-writable-paths` rule
  asks for this to be reasoned about before the code is written; the launcher's
  comments do not mention it. Remediation: note explicitly why same-uid link
  following is acceptable here, or create the tree with `mkdir` (no `-p`) and
  refuse a `${DIR}` that is not a real directory.

- **[low / confidence: high]** `image/configs/styrman-browser:60` —
  `TCP-LISTEN:...,fork,reuseaddr` has no `max-children`. socat forks per
  connection, unbounded, in a guest whose memory budget ADR-068 D7 already
  raises by ~250 MB for Chromium. Anything that can reach the port (see finding
  3, and finding 1 for the remote case) can exhaust guest PIDs/memory without
  ever speaking CDP. Remediation: `max-children=8` or similar.

- **[low / confidence: high]** `image/entrypoint.sh:151-157` — the harness stamp
  now gates on two commands that write into a **guest-writable** home
  (`~/.claude.json`, `~/.gemini/config/mcp_config.json`), with all output sent to
  `/dev/null 2>&1`. A guest that makes either write fail permanently (an
  unwritable or immutable file) permanently withholds the stamp, so the full
  `harness-migrate` + `sh-agent install` pair re-runs on **every boot**, forever,
  burning up to 40 s of R1's 15 s soft budget with no backoff and no diagnosable
  reason in the log (the WARN names the tool, never the error). Self-DoS only,
  but it is a guest-controlled input deciding a server-side retry loop.
  Remediation: capture the command output to the log instead of discarding it,
  and cap consecutive re-seed attempts.

- **[info]** `image/Dockerfile:113-117` — `chromium`, `fonts-liberation` and
  `socat` are pinned by the Debian archive rather than a digest. The comment
  argues this deliberately, by analogy to `docker-ce`, and for a package whose
  whole value is receiving security updates that is the right trade. Recorded,
  not a finding. The decision *not* to pass `--no-sandbox`, the loud-failure
  rationale at lines 22-25, and `integration.sh:117`'s assertion that
  `--no-sandbox` is absent from the live `/proc/<pid>/cmdline` are all good
  practice and should survive future edits.

## Working tree drift (not part of the reviewed range)

While this review ran, the working tree moved underneath it: `internal/config/config.go`,
`internal/orchestrator/ports.go`, `internal/httpapi/proxy.go`, `internal/httpapi/shares.go`
and a new `internal/httpapi/browser.go` gained `config.BrowserPort` /
`BrowserChromePort`, a `parseListeningPorts` filter, and a `previewPortAllowed`
helper; `image/configs/styrman-browser` lost `--remote-allow-origins=*`. That is
Phase 2 landing, and on inspection it addresses findings 1 and 2 well:

- `previewPortAllowed` is called from `proxyPort` (`proxy.go:49`) and from
  `sharePort` (`shares.go:117`). Because `createLink` and `setShares` both parse
  their port through `sharePort`, the **public link mint is covered transitively**
  — I checked this specifically, having first assumed otherwise. All three
  preview-shaped routes refuse 9222 and 9229 in the current tree.
- `parseListeningPorts` drops both ports, so the Ports panel no longer offers
  them — closing the path by which an owner would have been invited to share it.
- The `--remote-allow-origins=*` flag is gone, and the replacement comment states
  the correct reasoning: the two legitimate clients send no `Origin` header and
  are admitted, while a page rendered inside the browser always sends one and is
  refused.

**The findings are not retired by this, for two reasons.** (a) None of it is
committed or reviewed here; this file's verdict is about `e7f7857`, the commit
that is currently HEAD and that a build would produce today. (b) The ordering
problem stands regardless of how good the Phase-2 code is: the runtime **image**
carries the listener and the **server** carries the guard, and the two ship as
separately-versioned artifacts (`versioning.md`'s three-image lineup). A host
running a new runtime image against an older server binary has the listener and
no refusal. Make that combination impossible — either gate the launcher on a
server-supplied token, or state the minimum server version in the image and
refuse to start below it. Re-review the Phase-2 commit against this file.

## Checked

- **Secrets** — no credential, token or key added by the diff; no `.env` or key
  material committed. `entrypoint.sh`'s new lines interpolate only
  `${USERNAME}`/`${HOME_DIR}` (server config) and pass them as `runuser`
  arguments, not through a shell, so no injection there. The new `sh -c` payload
  is single-quoted with no interpolation. The pre-existing stamp write one line
  below (`sh -c "... '${HARNESS_VER}' ..."`) does interpolate into a double-quoted
  shell string, but `HARNESS_VER` comes from the root-owned `/styrman/VERSION` —
  noted, pre-existing, not in this diff.
- **Injection** — the launcher is `set -euo pipefail`, quotes every expansion,
  and passes `$1..$4` as positional arguments into the inner `bash -c` rather
  than interpolating them; `case "${1:-}"` rejects unknown verbs with exit 2.
  No user-controlled data reaches a shell. The real input surface is the
  *DevTools protocol itself*, which is unauthenticated — findings 1-3.
- **Authz/authn** — the defect is here: the new listener inherits the port
  authorisation model of "any port a dev server might use", which grants a
  shared-email guest preview access (`shares.go:79-84`) and a public-link
  visitor no-login access (`links_api.go:149`). ADR-068 D3's owner-only rule is
  unimplemented at this commit.
- **Crypto** — nothing hand-rolled; no auth, hashing, session or JWT code added.
  Conversely, no authentication was added to the DevTools endpoint *either*,
  which is finding 3.
- **Dependencies** — one new npm package (`chrome-devtools-mcp@1.9.0`, top-level
  pin only, see finding 5) and three new apt packages (`chromium`,
  `fonts-liberation`, `socat`, archive-pinned by design). No Go module change:
  `go.mod`/`go.sum` are untouched by the diff, so the Go module surface is
  unchanged by this commit.
  `govulncheck ./...` **was run** (root module, from the repo root) and reports
  31 vulnerabilities the code calls. **29 of those are an artefact of my local
  toolchain, not of the shipped binary**: they are Go standard library issues
  reported against `go1.25` (my local `go version` is go1.25.0) with fixed
  versions at or below go1.25.13 — and `deploy/Dockerfile.server:9` builds with
  `golang:1.25.13-trixie`, so the released artifact already has them. I did not
  run the project's own `deploy/vulncheck.sh`, which scans inside that pinned
  builder image and is the authoritative path; treat its output, not mine, as
  the answer for the stdlib.
  The remaining **2 are real and toolchain-independent**: `GO-2026-4887` and
  `GO-2026-4883` in `github.com/docker/docker@v28.5.2+incompatible`, both with
  `Fixed in: N/A` (no fixed version published on that line at scan time). They
  are **pre-existing and not introduced by this diff** — recorded here because a
  dependency scan is in scope, not to weight the verdict on the commit. Track
  them for a Docker SDK bump when one lands.
  `npm audit` was **not** run against the new package and could not have been:
  `chrome-devtools-mcp` is a global `npm install -g` inside the image and does
  not appear in `web/package.json`, so no lockfile describes it (that absence is
  finding 5). `npm` 9.2.0 is present locally; `web/`'s own audit surface is
  untouched by this diff and was not re-scanned.
- **GCP surface** — none. The platform is self-hosted (`docs/deploy/gcp.md`:
  not applicable); the diff touches no IAM, service account, Cloud Run flag or
  Secret Manager reference.

## Not examined

- The Phase-2 server code (`internal/httpapi/browser.go`, the CDP relay,
  `web/src/components/BrowserPane.tsx`) — does not exist at this commit.
- The three new integration checks beyond reading check 117's assertion text;
  they were not executed (no Kata guest was booted for this review).
- `chrome-devtools-mcp`'s own source and its transitive tree — not fetched or
  audited; finding 5 is about the *pinning*, not about a known defect in it.
- The runtime image was not built, so no claim is made about what the resolved
  npm tree or the archive `chromium` version actually contains.
- Docs changed by the commit (`specs/styrman/runtime-image.md`, `image/README.md`,
  the plan, the change entry) were read only where they stated a security
  property; per the dispatch, plans and handoffs were not reviewed.

Verdict: **FINDINGS** — two high, three medium, three low. The high pair
(unguarded shareable/linkable DevTools port, and `--remote-allow-origins=*`)
must be closed before any runtime image carrying this launcher is released.
