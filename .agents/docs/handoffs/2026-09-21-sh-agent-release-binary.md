---
owner: main
date: 2026-09-21
---
# Handoff — 2026-09-21 — the harness as the sh-agent release binary in the image (ADR-074); R5 approval is the one open item

Session ID: bbcf8dfa-4c8b-44e6-9dcf-f1ece4171cb2        (same machine: `claude --resume bbcf8dfa-4c8b-44e6-9dcf-f1ece4171cb2`)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

styrman-harness (v1.6.0) rebuilt its delivery around `sh-agent`, a binary with the harness embedded, released per signed tag; its R8/R9 retire vendored mirrors and signed-tag channels. The operator asked crew to follow: "Only add sh-agent binary to the runtime and run install. That's it, don't complicate things." and "Build from the GitHub repo. Not local project." Between crew releases each project moves itself forward with `sh-agent update`.

## Completed

- Built, reviewed, verified, documented and committed as b60d891 (pushed): plan `.agents/docs/plans/2026-09-21-sh-agent-release-binary.md` — Phases 1–3 ticked, Phase 4 ticked except the R5 box; entry `.agents/docs/changes/2026-09-21-sh-agent-release-binary.md` has every file and gate row.
- `image/Dockerfile` downloads the sh-agent release named by `ARG HARNESS_VERSION` (v1.6.0) from GitHub Releases at build with a docker build secret, verifies the signed `SHA256SUMS`, the version header, the checksum and the binary's own `version`; the mirror `image/harness/` and its builder stage are gone.
- `image/entrypoint.sh`: stamp `<image version>+<release>`, `sh-agent install --home`, the never-downgrade guard `harness_home_newer` (both sides canonical), `kept` vs `deployed` pass lines, the manifest read under `timeout 5`.
- The host harness channel (ADR-073) deleted end to end; `retire_harness_units` in `deploy/update/styrman-update` sheds its unit pair and state files on every tick; one token rule at the five image-build sites.
- Tests: `TestStyrmanUpdateRetiresHarnessUnits`, `TestEntrypointHarnessHomeNewer`; Tier 2 rewritten (115 checks, `passed=112 failed=0 skipped=3`); web Tier 1 green; verifier logs `.agents/docs/testing/2026-09-21-verify-sh-agent-release-binary{,-round-2}.md` PASS.
- Reviews: cold read ×2 (`reviews/2026-09-21-review-plan-sh-agent-release-binary{,-round-2}.md`), code `reviews/2026-09-21-review-sh-agent-release-binary{,-round-2}.md`, security `reviews/2026-09-21-security-sh-agent-release-binary.md`, docs lens `reviews/2026-09-21-review-worktree-docs.md` — every finding fixed or rebutted in each `## Outcome`.
- BUG-066 (the test suite's image builder had no build secret), BUG-067 (the guard's `unknown` sentinel; `deployed` logged after a skip).

## Open / In Progress

- The plan's R5 box (`who: user`) and its final checkpoint: `REQUIREMENTS.md` R5 still describes the ADR-073 channel. The exact replacement is the plan's D6; the ADR-074 draft is `.agents/scratch/adr-074.md` (gitignored — regenerate from the plan's D-rows if the scratch is gone). On approval: apply the R5 edit, append ADR-074 to `.agents/docs/architecture/DECISIONS.md` (mark ADR-062 and ADR-073 superseded), one commit, then close the plan and clear ROADMAP "Now".
- Requirements edits awaiting approval: R5, from "is carried by the **host's verified harness channel**" through "(ADR-030, revised 2026-08-28; was "deployed into the shared home")." replaced with: "is carried by the **base image** as the `sh-agent` release binary with the harness embedded — downloaded at image build from the styrman-harness GitHub release named by the Dockerfile's `HARNESS_VERSION`, its signed checksums verified against the harness's signer (ADR-074, operator approval 2026-09-21; was "the host's verified harness channel … the image's pinned mirror … the fallback", ADR-073). The entrypoint installs it into a project home whose harness differs and never downgrades one the user moved forward; from there each project moves itself forward when its user runs `sh-agent update` in it (the harness's R8/R9), so projects may run different harness releases. The default shell config is carried by the base image and seeded write-once into every project home (ADR-030, revised 2026-08-28; was "deployed into the shared home")."
- Not released: the next release is v1.36.0 (MINOR, plan D7), cut by `/preflight` on the operator's word after the R5 commit.

## Key Decisions

- Plan D1–D11 (`## Decisions` in the plan): the published binary from GitHub at build (D1); the version is the Dockerfile ARG (D2); one token, the crew token granted read on styrman-harness (D3); one stamp, never downgrade (D4); browser MCP after install (D5); v1.36.0 (D7); the content dir left to the operator's `rmdir` (D9); one token rule at every build site (D10); the review fixes (D11). Open: keep the never-downgrade guard — default keep.
- Rejected: a local checkout as a build context; a wrapper script, a pin file, a bump-pins refresh; a `HARNESS_SHA256` content pin (security finding 1, rebutted: the operator's signature on the release's sums is the pin).

## Corrections

- "This sounds excessive. Only add sh-agent binary to the runtime and run install. That's it, don't complicate things." — feedback log 2026-09-21T19:02:36+02:00
- "Build from the GitHub repo. Not local project" — feedback log 2026-09-21T20:06:46+02:00

## Agent notes (unverified)

- The reviewer and the verifier each hit a turn limit once with a stub artifact and finished on a `SendMessage`; the round-2 reviewer's stub sat 20 minutes before its verdict. Budgets of 40/30 were too small for this diff (207 files).
- Live host: `styrman-harness-update.{service,timer}` are installed and disabled from v1.35.0 and `/var/lib/styrman/harness` exists, empty; the retire step removes the units on the first tick after the v1.36.0 deploy that runs the NEW updater (BUG-064's shape: one tick later), and the dir is the operator's `sudo rmdir`.
- The web `node_modules` was absent in this checkout; `npm ci` restored it.

## Environment State

- Branch: main, clean after b60d891 (this handoff commit follows); no new tags.
- Live host unchanged: `styrman-server:1.35.0`; the operator's two projects on their pre-release containers.
- Local images: `styrman-runtime:dev` and `styrman-runtime:test` rebuilt with the release binary (disposable).
- Running processes: none of the session's.

## Artifacts to Read

- `.agents/docs/changes/2026-09-21-sh-agent-release-binary.md` — what changed, every file and gate row.
- `.agents/docs/plans/2026-09-21-sh-agent-release-binary.md` — the design, D1–D11, the one open box.
- `.agents/docs/specs/styrman/deploy.md` § "Agent harness in the image (ADR-074)" — the mechanics, image, boot, host.
- `.agents/docs/reviews/2026-09-21-security-sh-agent-release-binary.md` — the trust-model statement and the two accepted risks.

## Next Steps

1. Operator: approve (or amend) the R5 text above. Then: apply it, append ADR-074, commit, close the plan, clear ROADMAP "Now".
2. Operator, before the release deploy: grant the crew token at `/etc/styrman/gh-token` read access to `kesokaj/styrman-harness` in GitHub's fine-grained token settings — the deploy tick's image build downloads the release with it and stops without it.
3. `/preflight v1.36.0` on the operator's word. After the deploy: `systemctl is-enabled styrman-harness-update.timer` reads not-found after the SECOND tick; `sudo rmdir /var/lib/styrman/harness`; in a project terminal `sh-agent version` → `sh-agent v1.6.0` and `sh-agent update` → `up to date (v1.6.0)` (the plan's user-side e2e).
4. Carried operator items from the v1.35.0 handoff (`.agents/docs/handoffs/2026-09-16-v1.35.0-harness-channel-live.md` item 4) still stand; its items 1–2 (the channel token and the first signed tag) are void — the channel is gone.
