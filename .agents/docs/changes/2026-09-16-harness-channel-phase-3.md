---
owner: main
date: 2026-09-16
---
# Harness channel Phase 3: the entrypoint and the integration tier (2026-09-16)

## Summary

Phase 3 of `.agents/docs/plans/2026-09-16-harness-channel.md` (ADR-073): the runtime image's entrypoint now seeds a project home keyed on the image version PLUS the harness commit, and its pass line names where the harness came from (`source channel tag <tag>` or `source image`). Tier 2 delivers a scratch channel the production way before the server starts, asserts the guest boots from it, and swaps it to a second tag while the VM is stopped to prove Docker re-resolves the bind source on the next start. From this phase on, a project seeds from the host's channel whenever the server mounted one; the image's mirror remains the fallback.

## Type

Feature (phase checkpoint)

## Changed

- `image/entrypoint.sh` — `HARNESS_COMMIT` (40 hex from `/styrman/harness/COMMIT`, else `unknown`), `HARNESS_ID="${HARNESS_VER}+${HARNESS_COMMIT}"` is what the stamp `~/.styrman/.harness-version` is compared with and written as; `HARNESS_SRC` is `channel tag <TAG>` when `/styrman/harness/TAG` exists (only a channel export carries one), else `image`; the pass line becomes `agent harness deployed (version …, styrman-harness <sha7>, source <src>; browser MCP registered for claude and agy)`. A stamp written before this release holds the version alone, differs once and re-seeds once.
- `.agents/docs/testing/styrman/make-scratch-harness.sh` — new helper, `<outdir> <image> [<tag>]`: a scratch origin from `image/harness/` (minus crew's `COMMIT`, `SHA256SUMS`, `VENDORED.md`, `.agents`), a scratch ed25519 key as the one `allowed_signers` line (written 0600: `verify_tag` refuses a group-writable signers file even in test mode), `<tag>` signed, the channel's own clone, then `styrman-update harness` in test mode against `<outdir>/{state,clone,content}` with the smoke in `<image>`; a second call with a new tag appends one README line, signs and delivers again (the swap). Last stdout line: the content dir's `current`. Refuses a production path.
- `.agents/docs/testing/styrman/integration.sh` — new section "harness channel" before the server starts (the helper delivers v1.0.0; the server gets `STYRMAN_HARNESS_DIR`); "server start logged the harness channel (tag v1.0.0, its commit)"; the harness section reads the stamp id `HID` from the guest once and adds five checks (guest `COMMIT` equals the host's `current`, the mount read-only even for root, `sh-agent` sha256 equals the channel's binary, no `.git`, boot log `source channel tag v1.0.0`); the mirror-commit check reads the channel's `COMMIT`; both stamp checks compare against version+commit; the stop+delete section swaps the channel to v1.0.1 while stopped and, after the `boot=1` start, asserts the guest mounts the new export and re-seeded from it. Ten checks added, 116 total.
- `.agents/docs/testing/styrman/README.md` — the Tier 2 count line moves with the checks (`passed=113 failed=0 skipped=3`; the ten checks named), the harness paragraph describes the channel and the helper.
- `.agents/docs/specs/styrman/runtime-image.md` — step 6 rewritten: the two sources of `/styrman/harness` (channel export, image mirror as fallback), how the entrypoint tells them apart, the stamp keyed on version+commit and what re-seeds.
- `.agents/docs/plans/2026-09-16-harness-channel.md` — Phase 3 boxes and Checkpoint ticked with the evidence; the helper's signature is `<outdir> <image> [<tag>]` (the image is required, the smoke needs it).

## Verification

`verifier` agent, `.agents/docs/testing/2026-09-16-verify-harness-channel-phase-3.md`: PASS — `bash -n` and `shellcheck -S warning` clean on the three scripts; the image `styrman-runtime:itest-harness` (built by the main agent from the working tree before the dispatch, not rebuilt by the verifier) carries the new entrypoint; the helper delivered v1.0.0 and then v1.0.1 with a different commit by hand; Tier 2 once, `passed=113 failed=0 skipped=3 aborted=0`, the 3 SKIPs the host-firewall ones; `go build`/`go vet` still clean. The main agent's own earlier run printed the same line (`.agents/scratch/itest-phase3.log`). Must-fail state read by hand for the new guest checks: the same image run without the mount shows the mirror's `COMMIT` (`4aa19be…`) and no `TAG`, which the guest-COMMIT and `source channel tag` checks would fail on; the first helper attempt failed on the signers file's mode and the delivery check would have reported it.

Fix on the way (no bug record: a first-attempt correction in a new script): the helper's `allowed_signers` was written under the host's umask (0664) and `verify_tag` refused it; written under `umask 077` now.

## Agents dispatched

`verifier`. Review and security rows are owed once over the whole feature at Done (Phase 4).
