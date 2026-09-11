---
owner: main
date: 2026-09-11
---

# Browser view — Phase 1: guest image, launcher, MCP registration   (2026-09-11)

Date: 2026-09-11. Phase 1 of a four-phase feature (plan: `.agents/docs/plans/2026-09-11-browser-view.md`) completing the guest-side half: Chromium in the runtime image, the launcher script `/styrman/bin/styrman-browser`, and MCP registration for both agents. No server or SPA changes this phase. Operator approved the R4 sentence and ADR-068 2026-09-11 morning; the exact text went into REQUIREMENTS.md and DECISIONS.md in commit `a9ae789` same day. Code built, booted in a real Kata mvm, three new integration checks passed.

## Summary

**Verification at completion:** Tier 1 (shell lint) clean. Docker build green; image 3.56 GB → 4.32 GB (+760 MB). Tier 2 integration: `passed=87 failed=0 skipped=3` (the three new browser checks all passed; boot→ready 2.75 s including MCP registration). Tier 3 (server) deferred to Phase 1+2 combined. No Go changes in Phase 1; the verifier runs once both phases land. Operators' manual agent walk deferred to Phase 4.

## The code

**Runtime image (`image/Dockerfile`).** One `RUN` step after the agents step and BEFORE the base-package snapshot: apt-get install `chromium fonts-liberation socat` (Debian archive, pinned to `152.0.7977.82-1~deb13u1`; Debian security updates move it like docker-ce) and `npm install -g chrome-devtools-mcp@1.9.0` (ARG); `install` the launcher `image/configs/styrman-browser` to `/styrman/bin/styrman-browser` (0755). Build-time proof: `chromium --version` in the build → 152.0.7977.82 as expected.

**Guest launcher (`image/configs/styrman-browser`, new, 142 lines, bash, shellcheck clean).** Four verbs: `start` (idempotent, probes `/json/version` within 15 s, exit 0 or 1 with log tail), `stop` (kills the process group, exit 0 even if not running), `status` (JSON `{"running":bool,"version":"Chrome/…"|""}`), `mcp` (start quiet then exec chrome-devtools-mcp). State dir `~/.styrman/browser/` (mode 0700): `profile/` (Chromium user data), `pid`, `chromium.log`, `lock`. Launch shape: `socat TCP-LISTEN:9222,fork,reuseaddr,bind=0.0.0.0 TCP:127.0.0.1:9229 &` (because new-headless Chromium ignores `--remote-debugging-address`; verified on this host) then `exec chromium --headless=new --remote-debugging-port=9229 … --user-data-dir=~/.styrman/browser/profile about:blank`. Process group under `setsid -f`, stdio → log; lock fd closed in child with `9>&-` to prevent inheritance; start takes `flock -w 20` around the probe (bounded). Sandbox ON: probed 2026-09-11 in a real Kata `mvm` container (`styrman-runtime:1.27.1`, guest kernel 6.18.28, user namespaces on), no `--no-sandbox` flag → launch answers `/json/version` within 15 s with the flag absent from the live cmdline; under plain runc the launch fails LOUDLY with "No usable sandbox" (expected, not the target).

**Harness seeding (`image/entrypoint.sh`).** Third stamp-gated step (`HR_OK`, graded as ONE unit with `HM_OK`/`HI_OK`): `claude mcp remove --scope user styrman-browser >/dev/null 2>&1 || true; claude mcp add --scope user styrman-browser -- /styrman/bin/styrman-browser mcp` and `agy mcp add styrman-browser /styrman/bin/styrman-browser mcp`. Both commands under `timeout 20`; stamp written only when all three initial steps pass; failure logs `WARN: MCP registration failed for <claude|agy|both>` and the stamp stays unwritten so next boot retries. Verified 2026-09-11: both registries carry `styrman-browser` in a fresh project home; `chrome-devtools-mcp --help` lists `--browserUrl` and it exits 0.

**Docs (`image/README.md`, `.agents/docs/specs/styrman/runtime-image.md`).** New "Browser" section: package, launcher verbs, port 9222, the socat forwarder, why `--no-sandbox` is not used and what breakage looks like, MCP registration and its stamp gate.

## Defects found and fixed during Phase 1

(1) Lock file descriptor inherited by chromium so a second `start` blocked forever waiting on `flock -w 20`. **Fixed:** close fd 9 in the child with `9>&-` and keep the bounded wait. Caught by second `start` hanging under Kata probe.

(2) `stop` skipped the group kill when the leader had already died. **Fixed:** signal the group unconditionally with `kill -s TERM -- -$$` and wait for all to exit.

## Verification

**Static.** `shellcheck image/configs/styrman-browser` clean; `shellcheck -S warning image/entrypoint.sh` clean.

**Build.** `docker build -t styrman-runtime:browser-p1 --build-arg STYRMAN_VERSION=dev-<ts> image/`: green. Image 3.56 GB → 4.32 GB (+760 MB; more than the ≈400 MB estimate because chromium's dependency set is large).

**Launcher in plain runc (non-root).** `start`: exit 1, "No usable sandbox" logged and process group cleaned. Expected; runc blocks user namespaces.

**Launcher in real Kata mvm (non-root).** `start`/`status`/second-`start`/`stop`: 0 / `{"running":true,"version":"Chrome/152.0.7977.82"}` / returns at once (flock unblocks) / ports 9222 and 9229 both gone, no live chromium or socat. Sandbox check: live cmdline contains no `--no-sandbox`. Bridge reachability from host: `curl http://<ip>:9222/json/version` → 200; with `Host: example.test` → 500 (Chromium rejects it); raw WebSocket upgrade to the reported `webSocketDebuggerUrl` → 101 and ready. All verified 2026-09-11 on a throwaway Kata container.

**Integration (Tier 2).** `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test`: `passed=87 failed=0 skipped=3` (README: 90 checks, 3 auto-SKIP on this firewall-hardened host; the three new checks: "MCP registered for claude and agy", "start answers /json/version with sandbox ON", "stop leaves status not running" all passed). Boot→ready 2.75 s with the MCP registration seeding inline. Full output: `.agents/docs/testing/2026-09-11-verify-browser-phase1.md`.

## Agents dispatched

- **reviewer** (×3, r1–r3): `.agents/docs/reviews/2026-09-11-coldread-browser-view-r{1,2,3}.md` included findings over the phase architecture, the launcher implementation, and hardening.
- **security** (over the full feature scope, Phases 1–4 as one diff): deferred to the final phase gate.
- **verifier**: deferred to Phase 1+2 together once the server half lands (this phase changed no Go source; the run recorded above IS the project's Tier 2 command).
- **changelog** (this entry).

## Files changed

New:
- `image/configs/styrman-browser` (160 lines, bash)
- `.agents/docs/specs/styrman/runtime-image.md` (Browser section)

Modified:
- `image/Dockerfile` (one `RUN` step before base snapshot)
- `image/entrypoint.sh` (third seeding step, `HR_OK` gate)
- `image/README.md` (Browser paragraph)
- `.agents/docs/testing/styrman/integration.sh` (+3 checks)
- `.agents/docs/testing/styrman/README.md` (counts 87→90)
- `.agents/docs/plans/2026-09-11-browser-view.md` (Phase 1 ticked, Phase 2–4 notes added about firewall caveat)

Committed:
- `a9ae789` (R4 + ADR-068 + ROADMAP Now section, 2026-09-11 morning, operator approval)
- (This phase, not yet pushed)

## Requirements

R4 amended 2026-09-11 with the operator's explicit approval (commit `a9ae789`): "A Browser view showing the project's own in-guest Chromium, shared with the agents for testing what they build, is a testing surface and not an IDE (ADR-068, operator approval 2026-09-11)."

R3 (per-project isolation): browser profile isolated under `~/.styrman/browser/profile` per project home.

ADR-068: Browser view — one headless Chromium per project microVM, driven by the operator through a CDP screencast and by the agents through an MCP server (accepted, 2026-09-11).
