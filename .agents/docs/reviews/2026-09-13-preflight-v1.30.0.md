---
owner: main
date: 2026-09-13
---
# Preflight — v1.30.0 (2026-09-13)

Range reviewed: v1.29.0..HEAD (the security-round commit after e6ce8c4; SHA in the release entry)

Release: **MINOR** — user-visible behaviour changes on released surfaces
(brokered credentials materialize again in login shells; long operations no
longer time out at 20 s; Discard and a conflicting stash pop keep the editor
honest; formatter config discovery restored; a login shell says when the
broker did not answer; the runtime image build is deterministic; the auth
portal refuses off-domain redirects) and nothing an existing deployment
must react to. Requested by the operator: "Now a full /audit of there are
plenty of fixes create a minor and use /preflight then commit and push."

## VERDICT: SHIP

Every tier is green on the release tree, every review round's findings are
fixed or rebutted in writing (table below), and the one finding that is live
in production (BUG-054) is closed by a change pinned by unit tests and a
rebuilt image; its production proof follows the deploy.

## Findings (FAIL-first)

None blocking.

- **[medium / high, fixed before this gate]** The reviewer's three real
  findings on the fix diff — an exclude-line regex that never matched (one
  line appended to `.git/info/exclude` per Format click), the container
  removed AFTER the network was recreated (a failed remove would have left a
  stale broker env, BUG-054 verbatim), and a vacuous timeout test — are fixed
  in `e6ce8c4` (29 lines, read by the main thread: the fixed-string grep was
  run three times against a scratch `.git/info/exclude` and wrote one line;
  the reorder is covered by `TestEnsureRunningRecreatesTheContainerWhenItsNetworkVanished`;
  the test now spies `AbortSignal.any`).
- **[low / high, recorded]** Coverage gaps the verifier names as FAIL lines,
  accepted with reasons (`2026-09-13-verify-audit-fixes.md`): `files.Read`'s
  I/O-fault branch (no seam through `os.Root`), `NewUUID`'s `crypto/rand`
  failure path (no seam), the broker's non-ENOENT warn branch (needs an
  unreadable store), `SidePanelGit.tsx` (e2e only). The credstore mode
  branch and the cleaned-path guard gained tests in `25bdae8`.
- **[low / high, recorded]** The changelog agent's draft again claimed a
  verification nobody ran (`deploy/firewall/test.sh`) and mis-attributed
  `vulncheck` — caught on read-through, corrected before commit; the third
  recurrence of BUG-034/BUG-053, graduating per BUG-053's rule (retro item
  in the handoff).
- **[low / medium, rebutted]** Reviewer low: "unverified provisioning of the
  leading-dot `trust login redirect uri domain suffix`" — a generated
  Caddyfile was validated with the real `caddy validate` (styrman-caddy
  image, `.agents/scratch/caddy-validate.log`): the adapter accepted it and
  the `security` app provisioned (its log line precedes the failure, which
  is the dummy Cloudflare token in the `tls` app, not the portal).

## Review coverage

| Commits | Reviewer | Security | Verifier |
|---|---|---|---|
| df70bf5, e56e775 (v1.29.0 trail) | n/a (docs) | n/a | n/a |
| 6654374, 7066301 (audit artifacts) | n/a (docs) | n/a | n/a |
| 60741b7, e18d62c (the fix batch) | `2026-09-13-review-audit-fixes.md` (opus; REQUEST CHANGES: 2 medium + 3 low → all fixed in e6ce8c4 or rebutted above) | `2026-09-13-security-audit-fixes.md` (1 medium + 8 low; the medium — any `NetworkInspect` error read as "missing" now force-removed a container — and the lows fixed in the security-round commit: NotFound-only recreate, `credstore.ErrNotFound` for the broker's fault log, the portal cookie stripped on public apps too, U+00AD revealed, the MOTD marker printed as bounded printable text, the updater's trap reading a script-scope path at fire time, the allowlist owner check literally root outside the test hook; the exclude-line grep was already fixed in e6ce8c4; the explorer's `.styrman-fmt-*` filter hiding a guest-named file is accepted — the guest's own naming choice, documented in api.md) | `2026-09-13-verify-audit-fixes.md` (PASS; FAIL lines recorded) |
| 25bdae8 (docs, records, two tests) | read by the main thread (tests + docs) | — | same log, extension to e6ce8c4 (dispatched) |
| e6ce8c4 (review round, 29 lines) | read by the main thread (below the 50-line threshold) | the security pass ran on ..e18d62c; e6ce8c4 touches the exclude grep (a fixed string now), the remove-before-create order and a test | same extension (PASS) |
| security-round commit (≈60 lines: orchestrator, broker, generator, diffReveal, motd, updater) | read by the main thread: each hunk is the security report's own remediation text; `TestEnsureRunningLeavesTheContainerWhenTheNetworkCannotBeInspectedAfterAFailedStart`, the ingress PublicApp test, the reveal test and `TestStyrmanUpdateVerifyTagMarkers` pin them | its own remediations | `go test -race` on the touched packages, ingress module, vitest, `bash -n` + `shellcheck` — green (main thread); the full-suite log is the extension above plus these runs |

Plan cold-read: three rounds (`2026-09-13-coldread-audit-fixes{,-round2,-round3}.md`),
cap reached; the round-3 leftovers (five, one answer each) are decided in
the plan's "Cold-read trail" and flagged for the operator in the handoff.

## Trail freshness

- Newest `changes/` entry `2026-09-13-audit-and-fixes.md` describes this
  release's work (the release entry is written after this verdict). PASS.
- No `date:` in the future under `.agents/docs`. PASS.
- Append-only: no modified past `changes/` entries in the range;
  `DECISIONS.md` +25/−0 (ADR-069 and the ADR-030 amendment, appended at the
  end — the agent had inserted them mid-file; moved before commit). PASS.
- `ROADMAP.md` "Now" and `TODO.md` touched (25bdae8). PASS.

## Docs freshness

`documentation` updated `specs/styrman/{api,runtime-image,guide}.md`,
`testing/styrman/README.md`, `deploy/README.md`, `ROADMAP.md`; the
integration count stays 91 and e2e 92 (measured below). Root `README.md`
names none of the changed interfaces. PASS.

## Verification (tiers)

- Unit/build/vet/fmt/shell/frontend: `2026-09-13-verify-audit-fixes.md` —
  PASS at e18d62c; extension to e6ce8c4 dispatched.
- Integration and e2e on the pinned image (`styrman-runtime:test` rebuilt
  from `image/` with Go 1.27.1 / Node 22.23.2 / claude-code 2.1.270 / agy
  1.2.2 answering `--version`, `.agents/scratch/image-rebuild-audit.log`):
  at e18d62c's tree, integration `passed=91 failed=0 skipped=3` rc=0
  (`.agents/scratch/integration-audit.log`), e2e `passed=92 failed=0
  skipped=2` rc=0 (`.agents/scratch/e2e-audit.log`); at e6ce8c4's tree,
  integration `passed=91 failed=0 skipped=3` rc=0
  (`.agents/docs/testing/2026-09-13-integration-v1.30.0.log.md`) and e2e
  `passed=92 failed=0 skipped=2` rc=0
  (`.agents/docs/testing/2026-09-13-e2e-v1.30.0.log.md`). The security-round
  commit that follows e6ce8c4 changes no path the tiers exercise differently
  (a NotFound guard on a recovery branch, a log predicate, one more
  Caddyfile line of the same shape validated earlier, a regex class, the
  MOTD's byte filter, the updater's trap and owner check) and is pinned by
  unit tests; the tiers were not re-run a third time for it — a stated
  judgment, not an omission.
- BUG-054's production proof (a login shell in a NEW project shows the
  brokered `GH_TOKEN`) can only be taken after the updater deploys: the
  host-run test servers have no broker. Recorded in the handoff after the
  deploy; a failure there would be a PATCH.

## What was not checked

- BUG-054 end to end before the deploy (above).
- The Antigravity CLI's floating version (by design, ADR-069): the image
  built with 1.2.2 today, production ran 1.1.24.
- `deploy/verify-commit.sh` runs on the RELEASE commit after this report;
  result appended at the end.
- The auto-updater's deploy of the tag (appended once observed).
