---
owner: main
date: 2026-09-11
---
# Verification — Browser view, Phase 1 (guest image)   (2026-09-11)

Written by main, not the `verifier` agent: the phase changed no Go source, and the
run below IS the project's Tier 2 command on the live tree; the `verifier` is
dispatched over Phases 1+2 together once the server side lands.

## Commands and results

| Step | Command | Result |
|---|---|---|
| Lint | `shellcheck image/configs/styrman-browser`; `shellcheck -S warning image/entrypoint.sh` | clean |
| Build | `docker build -t styrman-runtime:browser-p1 --build-arg STYRMAN_VERSION=dev-<ts> image/` | green; 4.32 GB (was 3.56 GB) |
| Launcher, plain runc, non-root | `start` | exit 1, "No usable sandbox" (runc blocks user namespaces — expected, not the target); group cleaned |
| Launcher, Kata `mvm`, non-root | `start`/`status`/second `start`/`stop` | 0 / running Chrome/152.0.7977.82 / returns at once / ports 9222+9229 closed, no live chromium or socat |
| Sandbox | live cmdline of the Kata run | no `--no-sandbox` |
| Bridge reachability from the host | `curl http://<ip>:9222/json/version` | 200; `Host: example.test` → 500; raw WS upgrade → 101 |
| Tier 2 | `bash .agents/docs/testing/styrman/integration.sh styrman-runtime:test` (image rebuilt by `ensure-test-image.sh`) | `passed=87 failed=0 skipped=3` (README: 90 checks, 3 auto-SKIP on this firewall-hardened host); boot→ready 2.75 s |

PASS. The three new checks ("browser MCP server registered for claude and agy",
"styrman-browser start answers /json/version with the sandbox on",
"styrman-browser stop ends it") all passed inside the booted project. Full
Tier 2 output: `.agents/scratch/integration-browser-p1.txt` (scratch, not
committed).

## Not verified here

- The server relay and the SPA (Phases 2–4).
- The agent path end to end (`claude`/`agy` driving the browser) — the
  operator's manual walk in Phase 4.
