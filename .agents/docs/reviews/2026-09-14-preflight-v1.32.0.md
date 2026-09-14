---
owner: main
date: 2026-09-14
---
# Preflight — v1.32.0 (2026-09-14)

Range reviewed: v1.31.0..340b045 (plus the release commit)

Release: **MINOR by the operator's explicit call** ("preflight that shit
and push a minor"). By the versioning rule this is a fix to released
behaviour (a PATCH): the terminal-WebSocket boot path now joins the server
to the project's bridge before the socket is upgraded, so the first
terminal of a project left stopped across a server restart has its
credential broker reachable (BUG-060). Nothing added for a user beyond
that, nothing removed. The operator was told and chose MINOR; recorded, not
re-argued.

Pre-gate commit: `d285015` (the fix, its tests, the bug record, the rule
instance, docs). The rest of the range is handoffs and cold-reader briefs.
One edit landed after the gate rows ran: a comment-only correction in
`internal/terminal/ensure_before_upgrade_test.go` answering the release
review's finding 2 — it rides in the release commit and is re-verified by
`deploy/verify-commit.sh` on that commit (Deploy section below).

## VERDICT: SHIP

`deploy/verify-commit.sh 340b045` is green on a detached worktree (every
step, web included); the release review APPROVES over the whole range with
the three fix blobs byte-identical to the earlier reviewed ones; security's
three lows are answered in writing; the docs lens's two findings are fixed
(ADR-071, terminal-protocol.md); trail is append-only and current.

## Findings (FAIL-first)

None blocking.

- **[medium / high — ACCEPTED, gate condition]** Release review finding 1:
  the fix's evidence is unit tests against fakes; the real
  `docker network connect` on this path was never exercised, and no suite
  can (needs a project stopped across a containerized server's restart).
  Disposition: the live proof is this release's deploy verification, done
  after the updater deploys 1.32.0 and recorded in "Deploy" below — never
  assumed. If it fails, the release is reported as failed, not shipped.
- **[high / high — FIXED]** Docs lens finding 1: ADR-021 said self-attach
  happens only in the background and "never on a request path"; the fix
  contradicts that on the WS path. ADR-071 appended (append-only,
  ADR-021 untouched) — amends clause (b) for the WS handler alone, names
  the client's reconnect loop as the counterpart of the proxy's 307, states
  the published-port caveat for the dev topology only.
- **[medium / high — REBUTTED]** Security finding 1: `wsBoot` discards the
  attach result. The failure is logged at Warn by `EnsureSelfAttached` and
  reported by the guest's MOTD; failing the terminal would turn a
  credential outage into a shell outage. Same handling as every other
  attach path.
- **[low / high — FIXED]** Release review finding 2: the terminal test's
  comment named the wrong refusal mechanism (hijack) — the bare request's
  missing handshake headers make `websocket.Accept` answer 426 with an
  `Upgrade: websocket` header, which the assertion catches. Comment
  corrected; assertions unchanged.
- **[low / medium — FIXED]** Review of the fix (pre-release) finding 1:
  ordering relative to the upgrade was not pinned → the terminal test.
- **[low / low — FIXED]** Docs lens finding 2: terminal-protocol.md silent
  on the attach-before-upgrade → one sentence added.
- **[low / low — REBUTTED]** Security finding 2: an uncancellable 30 s
  attach inside the handshake — the same handshake already waits up to
  90 s on readiness; memoized 60 s.
- **[low / medium — OUT OF SCOPE, carried]** Security finding 3:
  `ip_forward=0` on the server container is set by compose and asserted by
  nothing at runtime — pre-existing, unchanged by this release; in the
  handoff's Next Steps.

## Review rows

- Code (fix, pre-release): `.agents/docs/reviews/2026-09-14-review-wsboot-attach.md`
  — APPROVE WITH NITS, Outcome written. Blobs pinned there are the ones
  committed in d285015 (confirmed by the release review).
- Security (fix, pre-release): `.agents/docs/reviews/2026-09-14-security-wsboot-attach.md`
  — three lows, Outcome written. The release range adds no auth, IAM,
  secret, input-parsing or dependency surface beyond that diff (the later
  commits are docs, handoffs, one test file); the security row is that
  report, not a new dispatch.
- Code (release range, opus): `.agents/docs/reviews/2026-09-14-review-v1.31.0-340b045-release.md`
  — APPROVE, two findings, Outcome written.
- Docs lens: `.agents/docs/reviews/2026-09-14-review-v1.31.0-340b045-docs.md`
  — two findings, both fixed, Outcome written.
- Verification: `.agents/docs/testing/2026-09-14-wsboot-attach-verify.md`
  (verifier, PASS, two rounds, working tree that became d285015) and
  `deploy/verify-commit.sh 340b045` (below).
- Independent diagnosis: `.agents/docs/plans/2026-09-14-goldfish-broker-unreachable.md`.

## Verification observed by the main agent (bare, exit status read)

- `deploy/verify-commit.sh HEAD` at 340b045 → rc 0, "OK: commit
  340b04503b20 builds and tests clean in isolation — safe to tag": root
  module build/vet/test -race, vendored ingress module, dependency
  agreement (0 divergences), harness mirror SHA256SUMS + builder pin,
  web/dist freshness (committed assets match a fresh build), shellcheck +
  systemd-analyze, gofmt, frontend check/lint/unit tests, server image,
  runtime image (27 skills / 7 subagents, mirror is a plain copy). Log:
  `.agents/scratch/verify-commit-340b045.log` (gitignored).
- After the comment-only test edit: `gofmt -l internal/terminal/` empty;
  `go test ./internal/terminal/ -count=1` → rc 0.
- Earlier in the session, on the tree that became d285015: `go vet ./...`
  rc 0; `go test ./... -count=1` rc 0; the BUG-060 test red before the fix
  (`calls = [EnsureRunning(alice,p1)], want [... EnsureSelfAttached ...]`)
  and green after.

## Trail freshness

- Newest `changes/` entry before this release:
  `2026-09-14-bug-060-terminal-ws-boot-attach.md` — describes the release
  diff. Pass.
- No `date:` in the future under `.agents/docs`. Pass (all 2026-09-14 or
  earlier).
- Append-only: `git log --diff-filter=M v1.31.0..HEAD -- .agents/docs/changes
  .agents/docs/architecture/DECISIONS.md` is empty; ADR-071 is an append.
  Pass.
- ROADMAP.md touched in d285015 (operator checks sentence); TODO.md
  untouched and empty. Report-only.

## What was not checked

- The live path itself before the deploy (finding 1 above): no test
  environment has a containerized server plus a project stopped across its
  restart. Checked after the deploy, below.
- The published-port topology ADR-021 was written against: production has
  no published server port; a dev compose with one was not exercised, so
  "the client's reconnect loop retries a disturbed handshake" is reasoned
  from `bridge.go`'s ordering and the TerminalPane's retry loop, not
  observed.
- `deploy/vulncheck.sh` was not re-run for this release (no dependency
  change since v1.31.0's gate).
- The e2e (Playwright) and integration (real Kata) suites were not re-run:
  no web change (dist freshness proven by verify-commit) and no change to
  the paths those suites cover; the Go unit gate covers the changed code.

## Deploy (observed after the tag)

Release commit `3730551` (gate report, release entry, ADR-071, README/ROADMAP,
terminal-protocol.md note, the test-comment correction) verified by
`deploy/verify-commit.sh 3730551` on a detached worktree (rc 0, Go
build/vet/test -race, ingress module, dependency agreement, harness mirror,
shellcheck, gofmt, server image, runtime image; `SKIP_WEB_FRESHNESS=1`
because `web/` is untouched since the full run at 340b045). Tagged
`v1.32.0`, SSH-signed with `git -c gpg.format=ssh -c user.signingkey=…
tag -s` (no git config changed); `git tag -v` against a root-read copy of
`/etc/styrman/allowed_signers`: `Good "git" signature … ED25519 key
SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU`, the key that signed
v1.31.0. Branch and tag pushed 13:19:19Z.

Updater tick 15:20:30 local (`journalctl -u styrman-update`): "v1.32.0
signature OK (allowed signer)", "deployed=v1.31.0 latest-stable=v1.32.0",
"deploying v1.32.0 as 1.32.0", "building runtime image
(styrman-runtime:1.32.0)", "building + starting the stack (server +
ingress) at 1.32.0", then 15:20:56 "[styrman-update] OK — v1.32.0 is
live". No rollback, no latch.

`styrman-ctl status` 15:21: server healthy on `styrman-server:1.32.0`
(`styrman-caddy:1.32.0`), deployed v1.32.0 sha
`3730551f223103345bd3001a6b7e4b1c0d032f34`, updater clone `/opt/styrman` at
3730551, firewall timer active (7 FWD rules, 1 IN), 0 running microVMs, 0
project dirs, verdict OK. Server log 13:20:41Z: "credential broker up
(bridge-only)" `:8091`, "styrman listening" with `styrman-runtime:1.32.0`,
"adopted state" 0 containers. In-container probe
`curl -fsS http://localhost:8080/api/health` → `{"status":"ok"}`; the public
URL answers 302 to an anonymous request (the ingress sign-in redirect).
`styrman-runtime:1.32.0`: `VERSION` 1.32.0, harness mirror 154 files.

**Live proof of the fix — NOT yet observed, and not observable on this
host today without a further server restart:** the condition is a project
STOPPED across a server restart, opened from the UI. At deploy time there
were 0 projects, so every project created from now on carries its
creation-time attach until the server next restarts; a create → stop →
open cycle on 1.32.0 would pass on 1.31.0 too and proves nothing. The
discriminating check is: create a project, stop it, restart the server
(the next release's tick, a host reboot, or `sudo deploy/styrman-ctl`),
then open it from the UI — its first terminal must show no "credentials:
the broker did not answer" line and `gh auth status` must succeed. Until
then the release is verified as deployed and healthy, and the fix as
unit-tested and reviewed — exactly what the release review's finding 1
said. The operator was told which check closes it.

Not observed here (operator, versioning.md step 5): a fresh Google sign-in
showing 1.32.0 in Settings.
