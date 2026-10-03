---
owner: main
date: 2026-10-03
---
# Release gate — v1.41.1   (2026-10-03)

Range reviewed: v1.41.0..ad5dbf6

A PATCH: it fixes the three lows v1.41.0 shipped with, plus records of the
root work done on dave on 2026-10-03. Nothing in the range blocks.

## Carried (none blocking)
| # | Severity / confidence | Where | Finding | Decision |
|---|---|---|---|---|
| 1 | medium / high | dave `/etc/styrman/gh-token` | The updater token is the operator's broad `gh` login token (`repo` write, `admin:public_key`). (security, finding 1) | left: operator — their choice; replace it with a fine-grained read-only token, then rotate the `gh` login |
| 2 | medium / high | dave `~/.ssh/id_ed25519` | The release-signing key has no passphrase, on a host where agents run with passwordless sudo. (security, finding 2) | left: operator — a passphrase with `ssh-add -c`, or an `ed25519-sk` key |
| 3 | low / high | `deploy/local.md` | Root steps on dave ran on chat consent with no Operator-steps record. (security, finding 3) | left: operator — record standing permission, or keep per-request consent |
| 4 | low / medium | `internal/orchestrator/ports.go` | A late joiner gets what is left of the shared 20 s. (code, finding 1) | rebutted: the failure it reports is the command's (report's Outcome) |
| 5 | low / high | `web/` build chain | `js-yaml` GHSA-2883-xcg3-v3hh, build-time only, carried since v1.40.0. | left: operator |

## Rows
**Verification — met.** `testing/2026-10-03-verify-release-ad5dbf6.md` (`owner: verifier`, `Range reviewed: v1.41.0..ad5dbf6`, `## Status: PASS`):
- `deploy/verify-commit.sh ad5dbf6` → "safe to tag";
- vulncheck rc 3, only the accepted GO-2026-4887/4883;
- 15 `TestStyrmanUpdateStatus*` (none skipped);
- `TestListeningPorts*` 8 × 3 under `-race`;
- Tier 2 `passed=122 failed=0 skipped=3`;
- Tier 4 `passed=99 failed=0 skipped=2`, `web/dist` unchanged.
One skip was added, with its reason (no `en_US.UTF-8` locale); it does not fire on this host. Nothing after `ad5dbf6` except records. No plan is open, so no tier beyond the project's own was named. No failing log is left in the range.

**Reviewer — met.** `reviews/2026-10-03-review-v1.41.0-ad5dbf6-code.md` (opus, COMPLETE, APPROVE WITH NITS). It confirmed the shared wait has no race, with `-race` ×5, and proved `TestListeningPortsParkedRequestsShareOneWait` by mutating it to fail. It probed the shell check under C and UTF-8.

**Security — met.** `reviews/2026-10-03-security-v1.41.0-ad5dbf6.md`, COMPLETE, all 20 release-diff paths read, nothing under `## Not examined`, no high finding. Its two mediums concern dave's host state, not the shipped code; both are the operator's (items 1–2).

**Docs lens.** Not run: no interface, command or deploy step changed. The specs' "plain characters only" sentence is now true in every locale.

**Trail freshness.** Pass: the newest `changes/` entry is this range's. No future dates, no deleted logs. `DECISIONS.md` was only appended to (ADR-082 notes).

## What was not checked
- Fresh-host path unexercised (no new VM).
- A live Settings notice for a failing status (an `ok` status shows nothing).
- The GCP VM: not reachable from this machine; its updater takes the tag itself.

VERDICT: SHIP
