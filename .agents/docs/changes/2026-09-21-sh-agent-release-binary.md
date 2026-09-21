---
owner: main
date: 2026-09-21
---
# The harness reaches every project as the sh-agent release binary carried by the runtime image (ADR-074)

## Type
feature

## Summary
styrman-harness rebuilt its delivery around one mechanism — `sh-agent` is a Go binary with the harness embedded, released per signed tag, and a place moves forward with `sh-agent update` — and its R8/R9 retire the two shapes crew still carried: the vendored mirror `image/harness/` (ADR-062) and the host harness channel (ADR-073, v1.35.0). Both are deleted. `image/Dockerfile` now downloads the sh-agent release named by `ARG HARNESS_VERSION` (v1.6.0) from GitHub Releases at build with a docker build secret, verifies `SHA256SUMS.sig` against the harness's signer (`ARG HARNESS_SIGNER`), the version line, the checksum and the binary's own `version`, and keeps only `/styrman/bin/sh-agent`. The entrypoint keys its stamp on `<image version>+<sh-agent release>`, runs `sh-agent install --home <home>` (no `--repo`) when it differs, then the browser MCP registration as before, and never downgrades a home whose manifest names a newer release than the image's — the user's `sh-agent update` inside a project is how a project moves forward between crew releases. The updater gained `retire_harness_units`, run on every tick before `host_converge` and independent of its firewall guards, which removes the channel's unit pair and its four state files from a host that carries them. Operator: "Only add sh-agent binary to the runtime and run install. That's it, don't complicate things." / "Build from the GitHub repo. Not local project."

## Files
- `image/Dockerfile` — the download/verify `RUN --mount=type=secret,id=gh_token`, `ARG HARNESS_VERSION=v1.6.0`, `ARG HARNESS_SIGNER`; the `harness-builder` stage, `COPY harness/` and the mirror `chmod` removed.
- `image/harness/**` — deleted (the ADR-062 mirror).
- `image/entrypoint.sh` — stamp `<VERSION>+<release>`, `install --home` without `--repo`, the never-downgrade guard as the pure function `harness_home_newer` (both releases canonical, the home's strictly newer; an unreadable image release takes a WARN path with no stamp), the manifest read only as a regular file under `timeout 5`, the pass line `agent harness deployed (version …, sh-agent vX.Y.Z; …)` or, for a skipped install, `agent harness kept (…)`; comments rewritten.
- `deploy/update/styrman-update` — the `harness` mode and every `harness_*` function removed (about 260 lines), the unit-pair blocks in `self_install` and `host_converge` removed, `retire_harness_units` added with the `__retire-harness-units [--check]` test hook (gated on `STYRMAN_UPDATE_TEST=1`), called before each of the three `host_converge` calls, `TRUST_UID=0` pinned on the production path; the deploy build passes `--secret id=gh_token,src=${GH_TOKEN_FILE}` (or `env=GH_TOKEN`).
- `deploy/update/styrman-harness-update.{service,timer}` — deleted; `styrman-update.service` loses `STYRMAN_HARNESS_REPO_DIR`.
- `deploy/styrman-ctl` — `--harness-token-file`, `HARNESS_REPO_DIR`/`HARNESS_ORIGIN`, `setup_harness_channel`, the `harness` status line and JSON block, the two harness units in `UNITS[]` removed; the install build passes the secret (`--gh-token-file` → `src=`, else `GH_TOKEN`, else the installed token file; a `--dry-run` without one plans the `src=` form and says so).
- `deploy/host-setup.sh` — the channel content dir and unit install removed; step 7's build takes the secret (`GH_TOKEN`, else `/etc/styrman/gh-token` after the root/0600 check, else FATAL).
- `deploy/docker-compose.yml` — `STYRMAN_HARNESS_DIR` removed.
- `deploy/verify-commit.sh` — the mirror section (sums, vet, toolchain pin, list-count match, `.git` absence) replaced by one check: the built image's `sh-agent version` equals the Dockerfile's `ARG HARNESS_VERSION`; the build passes the secret (`GH_TOKEN`, else the token file, else `gh auth token`).
- `deploy/vulncheck.sh` — `image/harness` dropped from the module list.
- `internal/config/config.go` — `HarnessDir` / `STYRMAN_HARNESS_DIR` removed.
- `internal/orchestrator/orchestrator.go` — the channel binds, the recreate-rule comparison and `containerSpec.harnessBinds` removed (`slices` import gone); `internal/orchestrator/harness.go`, `harness_test.go` deleted.
- `internal/config/harnessdir_test.go`, `styrman_harness_test.go`, `styrman_ctl_harness_test.go` — deleted; `styrman_ctl_test.go` — the `STYRMAN_HARNESS_REPO_DIR` seam removed, the install-plan build line expects the secret; `styrman_update_test.go` — `TestStyrmanUpdateRetiresHarnessUnits` added (removal, idempotence, the refused hook, `--check`, an untrusted unit dir); `entrypoint_guard_test.go` — `TestEntrypointHarnessHomeNewer` runs the entrypoint's guard function by extraction.
- `.agents/docs/testing/styrman/integration.sh` — the channel's fifteen checks removed, twelve added (the image's release, the home's binary, the manifest, the roster equals `list`, the boot line, the stamp, the seven-check never-downgrade case): 115 checks, `passed=112 failed=0 skipped=3`; `make-scratch-harness.sh` deleted; `ensure-test-image.sh` passes the secret without exporting the token.
- Docs: `README.md`, `AGENTS.md` (Setup Commands build line, Architecture bullet, Key Files), `image/README.md`, `deploy/README.md`, `deploy/caddy/VENDORED.md`, `.agents/docs/specs/styrman/{deploy,versioning,runtime-image,guide,api}.md`, `.agents/docs/deploy/remote.md`, `.agents/docs/testing/styrman/README.md` (Tier 2 count line, Tier 5 rewritten).
- Plan `.agents/docs/plans/2026-09-21-sh-agent-release-binary.md` (D1–D10; Open: the never-downgrade guard, default keep); recon `plans/2026-09-21-harness-delivery-recon.md`.

## Verification
- Cold read: `reviews/2026-09-21-review-plan-sh-agent-release-binary.md` (round 1, REQUEST CHANGES: three gaps, fixed), `…-round-2.md` (APPROVE, design ready, implementation ready).
- Phase 1: `docker build` with the secret exit 0; `sh-agent version` in the image → `sh-agent v1.6.0`; `list` → Skills (27) / Subagents (7); wrong-signer build fails (`Could not verify signature.`); `HARNESS_VERSION=v1.5.0` builds v1.5.0; entrypoint smoke in a scratch home: the boot line, `~/.local/bin/sh-agent`, `state.json` `embedded:v1.6.0` (the plan's `sed` prints it; the negative control prints nothing); the never-downgrade walk logged `left alone` with binary and manifest unchanged.
- Phase 2: `go build`/`go vet`/`go test ./...` exit 0; `TestStyrmanUpdateRetiresHarnessUnits` PASS; the pre-change script answers the hook with `unknown flag` (control).
- Phase 3: Tier 2 `passed=111 failed=0 skipped=3 aborted=0` (114 checks); web Tier 1: check/lint/test exit 0, 178 tests.
- Agents dispatched: explorer (recon `plans/2026-09-21-harness-delivery-recon.md`), reviewer (cold read ×2; code review `reviews/2026-09-21-review-sh-agent-release-binary.md`, APPROVE with three nits, all fixed; round 2 `reviews/2026-09-21-review-sh-agent-release-binary-round-2.md` (REQUEST CHANGES on one low: the `verify-commit.sh` token-file branch lacked the ownership bar — fixed, its Outcome written); docs lens `reviews/2026-09-21-review-worktree-docs.md` APPROVE, no findings; bugs (BUG-066 `ensure-test-image.sh` had no build secret, BUG-067 the guard's `unknown` sentinel and the `deployed` line after a skip)), verifier (`testing/2026-09-21-verify-sh-agent-release-binary.md` PASS; delta log `testing/2026-09-21-verify-sh-agent-release-binary-round-2.md` PASS), security (`reviews/2026-09-21-security-sh-agent-release-binary.md`, worst medium: two rebutted/accepted in ADR-074, six fixed with tests), documentation (living docs).
- After the fixes: `go test ./...` exit 0; Tier 2 `passed=112 failed=0 skipped=3 aborted=0` (115 checks, the `kept` check green).

## Notes
- R5 rewritten and ADR-074 appended on the operator's approval of the exact text in plan D6 (the same commit as this entry).
- The channel's empty content dir `/var/lib/styrman/harness` on the live host is the operator's `sudo rmdir`; the updater names no production path of its own (D9).
- The crew token at `/etc/styrman/gh-token` must be granted read on `kesokaj/styrman-harness` before the next release deploy, or the deploy tick's image build fails at the download.
- Next release: v1.36.0 (MINOR), cut by `/preflight` on the operator's word.

Addendum (2026-09-21, the follow-up commit): R5 rewritten on the operator's approval ("Go") and ADR-074 appended to `DECISIONS.md`; ADR-062 and ADR-073 marked superseded; the plan closed; ROADMAP "Now" cleared.
