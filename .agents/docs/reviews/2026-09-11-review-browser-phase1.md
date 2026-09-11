---
owner: reviewer
date: 2026-09-11
---
# Review — browser phase 1 (ADR-068)   (2026-09-11)

Range reviewed: commit e7f7857, restricted to `image/Dockerfile`,
`image/entrypoint.sh`, `image/configs/styrman-browser`,
`.agents/docs/testing/styrman/integration.sh`.

Blobs read:
- `image/Dockerfile` = f559f2b23133d8f2aa726e91b29156051fea69d9
- `image/entrypoint.sh` = f03d9ce3ed83c7da4afda31091a5781aa1ea3087
- `image/configs/styrman-browser` = 89dc29a6117cb7cd9ef99f67d72162c61eedadc1
- `.agents/docs/testing/styrman/integration.sh` = 22bec531b4d8820aa7edd1c6ce7ebb8a40eb9bf0

## Verdict: REQUEST CHANGES

The launcher's shape (locking, process-group teardown, stamp-gated retry,
"no --no-sandbox") is careful and matches ADR-068. But the Chromium launch
flags in `image/configs/styrman-browser` include `--remote-allow-origins=*`,
which disables the one control that stops a web page *rendered inside this
browser* from hijacking its own DevTools session — a concrete, high-severity
hole in a browser whose whole purpose is to load arbitrary dev-server and
public-site content.

## Findings

- **[high / confidence: high]** `image/configs/styrman-browser:44` (the
  `chromium ... --remote-allow-origins=*` launch flag, inside `launch()`).
  Chrome's `--remote-allow-origins` allowlist exists specifically to stop a
  page loaded *in the browser* from opening a WebSocket to its own
  `/devtools/...` endpoint and driving the CDP session (the classic
  "malicious webpage takes over headless Chrome" vector the flag was
  introduced to close). Setting it to `*` disables that check entirely.
  Concrete failure: the agent or operator navigates this browser to any
  attacker-influenced page (a dependency's dev server, a public site under
  test, a compromised CDN asset) whose JS does
  `new WebSocket('ws://127.0.0.1:9222/devtools/page/<id>')` (target id
  readable from `GET /json/list`, itself reachable since the Host-header
  check only guards the HTTP endpoints, not the WS Origin check this flag
  governs). That page now has full CDP control of the browser process:
  `Page.navigate` to `file://`, `Network.*` to read/inject into every other
  tab and cookie the project's browsing session holds, `Runtime.evaluate`
  in any frame. ADR-068 D3's "one trust domain" analysis covers who can
  *reach the port* (project user + inner containers); it does not cover
  content rendered *inside* the browser reaching the port from the inside,
  which is the scenario this flag was designed to block and which this
  change reopens. Fix: do not set `--remote-allow-origins=*`; if
  `chrome-devtools-mcp`'s connection needs a specific allowed origin,
  allowlist that value (or omit the flag and rely on the loopback-only bind
  chrome-devtools-mcp already needs to work around via socat, connecting the
  MCP client to `CHROME_PORT` directly rather than widening what page content
  is permitted to do).

- **[low / confidence: medium]** `.agents/docs/testing/styrman/integration.sh:112-114`
  (`browser MCP server registered for claude and agy` check). The `claude`
  half asserts the exact `.mcpServers["styrman-browser"].command` value via
  `jq`; the `agy` half only does `grep -q styrman-browser` against
  `mcp_config.json`, which passes even if `agy mcp add` wrote the wrong
  command path or a malformed entry (e.g. a stale/duplicate registration
  under a different command). Given `entrypoint.sh`'s gate already gives
  `agy mcp add` a single, generic pass/fail, this test is the only place
  that could catch a silently-wrong registration and it does not. Suggest
  matching the claude check's rigor (parse the JSON and assert the command
  field) rather than a substring grep.

## Criteria

1. **Dead code** — pass. No orphaned code; the new launcher and Dockerfile
   block are all reachable from the new entrypoint step and MCP registration.
2. **Flow tracing** — pass except the finding above. `image/entrypoint.sh`'s
   new MCP-registration step is correctly folded into the existing
   stamp-gate (`HR_OK` joins `HM_OK`/`HI_OK`; stamp withheld unless all
   three succeed, so a partial failure retries next boot — this repo's own
   BUG-035 rule, honored: the retry is unconditional/idempotent
   (remove-then-add for claude, add-or-update for agy), not gated on "did
   something change this tick"). `styrman-browser`'s lock/pidfile/process-group
   lifecycle traces correctly start→probe→launch→status/stop.
3. **Standards conformance** — pass on `bash` skill norms (`set -euo pipefail`,
   quoting, `shellcheck disable` justified inline); the security gap above is
   the one conformance failure (hand-rolled network exposure decision that
   needed the same scrutiny this file gives auth/crypto elsewhere in the
   repo).
4. **Directory layout** — pass. Launcher in `image/configs/` installed to
   `/styrman/bin/`, consistent with the existing `sh-agent` binary's location;
   integration checks appended to the existing suite file.
5. **Simplicity (DIY-except-security)** — the socat-forwarder-to-loopback
   design is a reasonable, documented workaround for Chromium ignoring
   `--remote-debugging-address`; not flagged. The `--remote-allow-origins=*`
   choice is exactly the kind of network/security-exposure decision this
   project treats as high-stakes elsewhere (see finding above) and was not
   given equivalent scrutiny here.
6. **Decoupling** — pass. `styrman-browser` is a small, self-contained CLI
   with a narrow four-verb interface; `entrypoint.sh`'s new step touches only
   the seeding block it was added to.

## Not examined

`internal/orchestrator/browser.go`, `internal/httpapi/browser.go`,
`web/src/components/BrowserPane.tsx` (out of the path scope given), the
actual behavior of `claude mcp add --scope user` / `agy mcp add` (took the
diff's own comments and the integration test's assertions as the spec, did
not run them), and no live/Kata verification was performed — this is a static
read of the diff only.
