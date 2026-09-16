---
owner: main
date: 2026-09-16
---
# Handoff — 2026-09-16 — session closed: v1.34.1 live, retro Proposal 1 applied; next is the harness channel, in three sessions

Session ID: 742de840-bb85-4deb-a1e1-fc40fde28110        (same machine: `claude --resume 742de840-bb85-4deb-a1e1-fc40fde28110`)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

After v1.34.0 and v1.34.1 shipped and the retro ran, the operator decided ("Go with your recommendations") to apply retro Proposal 1 here and to build a host-side, signature-gated harness channel so styrman-crew receives styrman-harness releases automatically instead of through a mirror refresh and a crew release each time. This handoff supersedes `2026-09-16-v1.34.1-close.md`.

## Completed

- Retro Proposal 1 applied (f82fc6e): `.agents/rules/tests-describe-behaviour.md`; BUG-061's row graduated; the retro record and `~/.config/sh-agent/retros.jsonl` marked `applied`; `changes/2026-09-16-retro-proposal-1-applied.md`.
- Everything before it: see `2026-09-16-v1.34.1-close.md` and `2026-09-16-v1.34.1-live.md`.

## Open / In Progress

- Nothing in flight in this repo. The harness channel is not started: it is a Feature for a fresh session here (Next Steps 2), after the harness session (Next Steps 1).
- Requirements edits awaiting approval: none yet — the channel session will propose an R5 amendment ("carried by the base image" → "carried by the host's verified harness checkout") and an ADR superseding the delivery half of ADR-062, for approval before code.
- The Claude Code trailer check (v1.34.0 brief): still needs the operator's re-sign-in; command in `2026-09-16-v1.34.1-close.md` § Next Steps 2.

## Key Decisions

- The harness channel design the operator accepted: a host-side timer polls styrman-harness for its newest SSH-signed tag, verifies it against `/etc/styrman/allowed_signers`, checks it out under `/var/lib/styrman/harness/<commit>/`, runs the smoke checks (`sh-agent list` 27/7, `install --dry-run` into a scratch home, no symlinks), and atomically swaps a `current` link (a failed smoke keeps the previous harness); the server bind-mounts `current` read-only at `/styrman/harness`; the entrypoint's re-seed stamp keys on the harness commit; the VM stays a plain copy with no git and no network; `image/harness/` and the refresh procedure go away. argolis-devbox needs only a user timer running `sh-agent update`.
- Do NOT refresh the mirror here in the meantime: the channel removes it.
- Three retro proposals stay queued for the harness (`~/.config/sh-agent/proposals/2026-09-16-retro.md`): the docs lens greps test suites, the verifier waits on the result line once, the preflight security brief reads every path.

## Corrections

- None from the user this session.

## Agent notes (unverified)

- The harness checkout is at `8b95f22`, ahead of the mirrored 4aa19be; the channel's first signed harness tag will carry those commits.
- styrman-harness has no signed release tags yet (its tags, if any, were not checked); the channel needs them signed with the operator's ed25519 key.

## Environment State

- Branch: `main`, clean, in sync with origin after this push. Host: v1.34.1; projects "google" (1.34.1) and "asdasdasd" (1.34.0 until its next stop/start).
- Running processes: none of ours.

## Artifacts to Read

- `.agents/docs/retros/2026-09-16-retro.md` — the four proposals (1 applied, 3 queued).
- `.agents/docs/handoffs/2026-09-16-v1.34.1-close.md`, `…-v1.34.1-live.md` — the release detail and candidates.
- `.agents/docs/architecture/DECISIONS.md` ADR-062 and ADR-072, `REQUIREMENTS.md` R5, `image/harness/VENDORED.md`, `.agents/docs/specs/styrman/versioning.md` — what the channel changes.

## Next Steps

1. **Session A — styrman-harness checkout (first).** `/retro --harness` consumes today's queue (three from here, eight from the morning); apply what is approved, `sh-agent install`; decide the signed-tag release procedure for the harness (same ed25519 key as crew's `allowed_signers`) and record it in that repo. Its own newest handoff is under `.agents/docs/handoffs/` there.
2. **Session B — styrman-crew (this repo), one session for the Feature.** Paste: `Read the handoff at .agents/docs/handoffs/2026-09-16-session-close-harness-channel-next.md and build the host-side, signature-gated harness channel it describes under the feature skill: scope confirmation, the R5 amendment and the ADR for my approval before any code, then the plan file with phases.` Be reachable for the approval gate.
3. **Session C — argolis-devbox (any time).** Paste: `Read this repo's newest handoff, then add a user-level systemd timer that runs sh-agent update on a schedule so the harness stays current here; ask me for the VM access if the deploy guide does not record it.`
4. Operator: the Claude re-sign-in and the trailer check (above).
