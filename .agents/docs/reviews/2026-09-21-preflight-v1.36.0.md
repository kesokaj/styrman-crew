---
owner: main
date: 2026-09-21
---
# Preflight — v1.36.0   (2026-09-21)

Range reviewed: v1.35.0..5478667 (the gated head; the release commit sits on it and carries the gate's own small fixes, listed under Findings, plus the records)

## VERDICT: SHIP

## Findings (FAIL first; none blocks)

| # | Severity / confidence | Finding | Decision |
|---|---|---|---|
| 1 | medium / high | Docs lens: fresh-host bring-up (`specs/styrman/deploy.md` Bring-up, "Rebuilding a host"; `deploy/remote.md` Prerequisites) ran `host-setup.sh` without saying its image build needs a token before `/etc/styrman/gh-token` exists | fixed now, in the release commit (`sudo GH_TOKEN=$(gh auth token) bash deploy/host-setup.sh …` with the reason) |
| 2 | low / high | Security 1: `styrman-ctl install --gh-token-file F` handed the file to a root `docker build` without `private_file` | fixed now (release commit) |
| 3 | low / high | Security 2: `export GH_TOKEN` survived at the three root build sites | fixed now (release commit) |
| 4 | low / high | Security 3: the sh-agent binary left the `govulncheck` loop with `image/harness` and nothing scans it here | accepted, named in the release entry; follow-up in the handoff (scan the downloaded binary with `govulncheck -mode=binary`) |
| 5 | low / medium | Security 4: an ambient `GH_TOKEN` outranks the vetted root-owned token file | rebutted: designed for the fresh-host bring-up; root's own environment is root's choice (security report `## Outcome`) |
| 6 | low / high | Security 5: `verify-commit.sh`'s `HARNESS_VERSION` regex accepted a non-canonical version | fixed now (release commit; must-fail control in the round-2 verifier log) |
| 7 | low / high | Security 6 (found in the second pass): the retire step leaves `/etc/styrman/gh-token-harness` and `/opt/styrman-harness` on a host that ran the v1.35.0 setup | fixed as docs (release commit): the operator's revoke-and-remove lines in `specs/styrman/deploy.md`; no code path may name those production paths (ADR-052) |
| 8 | low / high | Trail: 327da6b modified `DECISIONS.md` past entries (two ADR headings marked superseded) and appended to the feature entry | recorded; the append is append-only in spirit, the heading marks are the one edit a supersession needs — no precedent existed; left as is |
| 9 | low / — | Verifier coverage gap: the token-selection branches at the four host/dev build sites have no live test beyond `bash -n`, shellcheck and the `styrman-ctl install --dry-run` plan test | accepted, named in the release entry; the Dockerfile path they feed is live-tested (image build, Tier 2) |

Fixing 1, 2, 3 and 6 after the gate's verification row invalidated it; the row was re-run over the delta: `testing/2026-09-21-verify-v1.36.0-release-round-2.md` PASS (shellcheck at the gate's severity, `bash -n`, 19/19 config tests, `go vet`, the regex and its must-fail control).

## Rows

- Reviewer: covered by `reviews/2026-09-21-review-sh-agent-release-binary.md` (APPROVE, three nits fixed) and `…-round-2.md` (one low, fixed) over the tree that became b60d891; 327da6b's nine-line source change (quoting) and the gate's fixes (four one-line shell edits) read by the main agent. No re-dispatch: the covering reports subsume every source commit in the range.
- Security: `reviews/2026-09-21-security-v1.35.0-HEAD.md` — six lows, none blocking; `## Not examined` names no release-diff path (its first draft did; the agent completed the reading: every changed markdown scanned for secrets, private data and instruction-shaped text, the deleted styrman-ctl block read line by line); every finding fixed, accepted or rebutted in its `## Outcome`; prior `reviews/2026-09-21-security-sh-agent-release-binary.md` (two accepted in ADR-074, six fixed).
- Verification: `testing/2026-09-21-verify-v1.36.0-release.md` PASS at 5478667 (Go, caddy module, shellcheck at the gate's severity, web Tier 1; `verify-commit.sh 327da6b` and Tier 2 `passed=112` cited); `testing/2026-09-21-verify-v1.36.0-release-round-2.md` over the gate fixes — PASS.
- Trail freshness: newest entry `changes/2026-09-21-sh-agent-release-binary.md` is this release's work — pass; no future `date:` — pass; append-only — finding 7; ROADMAP/TODO touched in the range — pass.
- Docs lens: `reviews/2026-09-21-review-v1.35.0-HEAD-docs.md` APPROVE with nits — finding 1 fixed, the informational note left (its Outcome).
- Shadowing agents: none under `.claude/agents/` or `.agents/agents/`.
- `REQUIREMENTS.md`: R5 rewritten on the operator's approval in 327da6b; no violation.

## Deploy

The release commit is tagged `v1.36.0` (SSH-signed, per-command config, checked before the push as `specs/styrman/versioning.md` § Release procedure says); the auto-updater on this host deploys it within one 5-minute tick. Live verification is recorded below once the tick has run.

Deployed by the updater's 20:32:41 tick (`journalctl -u styrman-update`): `deploying v1.36.0 as 1.36.0`, runtime image built, deployed-version `v1.36.0` at 20:33:11, no rollback. Live at 20:33–20:35: `styrman-server:1.36.0` and `styrman-caddy:1.36.0` running, the server container `healthy`, its start line `"image":"styrman-runtime:1.36.0"`, `https://crew.styrman.xyz/api/health` answering 302 (the OAuth portal, as production does unauthenticated); `docker run --rm --entrypoint /styrman/bin/sh-agent styrman-runtime:1.36.0 version` → `sh-agent v1.6.0`; the server logs no `harness channel` line. The second tick (20:38:01, the NEW updater) logged `harness-retire: removed /etc/systemd/system/styrman-harness-update.timer` and `…service`; `systemctl is-enabled styrman-harness-update.timer` → `not-found`; no `harness-*` state files under `/etc/styrman/update`. `/var/lib/styrman/harness` does not exist on this host (the v1.35.0 units were installed but its content dir never was), so no `rmdir` is due. Not verified live: a project boot on the new runtime image and `sh-agent update` inside a project — the operator's check, in a project terminal, on the next stop/start of a project (the two running projects keep their old containers until then).

## What was not checked

- No live build of the token-secret paths in `host-setup.sh`, `styrman-ctl install` or the updater's `deploy()` before the deploy tick itself runs the last one (finding 9).
- Tier 2 and `verify-commit.sh` were run on 327da6b / the tree before it; the gate fixes touch neither Go source nor `image/`, and were verified by shellcheck, `bash -n`, the ctl/updater tests and the regex control only.
- Browser e2e (`web/e2e/run.sh`) not run: `web/` is unchanged in the range.
- `govulncheck`: exit 3 on the pre-existing docker/docker v28.5.2 advisories (no fix available), unchanged from v1.35.0.
- The sh-agent binary's own content (finding 4: the sh-agent binary's own content is the harness repository's gate).
