---
owner: main
date: 2026-09-09
---
# Handoff — 2026-09-09 — ADR-067 Phase 5: both suites run, and green

Session ID: c3f1df5b-b519-46e7-a42e-7402f8e48f28
Project: /home/kesokaj/workspace/styrman-crew
Branch: main. HEAD `8db7ac6`, pushed. This handoff lands in the commit that
follows it, so the tree state below describes the repo once that commit exists.

**Supersedes `2026-09-08-adr-066-shipped-and-audited.md`.** That handoff's
"Next Steps" are void: they pointed at hardening sshd and proving ADR-066's
LAN allowance on real hardware, and ADR-067 deleted that machinery entirely.

## Context

The session opened as a resume — "retrace the last sessions, what is left to
do" — and found ADR-067 mid-flight: Phases 0-4 committed, Phase 5's test
rewrites sitting uncommitted in the working tree, **written but never run**,
with one vitest failing. Running them is what this session did. Both suites
are now green, and running them turned up three defects in shipped code paths
rather than in the tests.

## Completed

- **Phase 5 boxes 1 and 2 ticked**, and genuinely verified this time:
  `STYRMAN_IT_LIVE_LAN` and both firewall-fixture env vars are gone, and both
  suites ran end to end.
- **Integration 78/5/3 -> 84/0/3** against real Kata. Two fixture bugs
  (BUG-025) and a stale pinned image (BUG-026).
- **Browser e2e 72/17/0 -> 86/0/2, exit 0, no leaked state dirs.** One cause
  behind all seventeen (BUG-021).
- **Three defects in shipped code**: `setProjectLan`'s timeout (BUG-022),
  `GET /ports` 500ing during a rebuild (BUG-023), `run.sh`'s trap abort
  (BUG-024).
- **`api.encoding.test.ts`** — 50 mutation-checked cases closing the
  2026-09-03 path-retargeting finding, including `termSocketUrl`, the one path
  the previous session's `/api` sweep missed.
- **`.gitattributes`** makes `web/dist/client/_shell.html` reviewable without
  touching its bytes.
- **BUG-021 … BUG-026** written with INDEX rows; changelog entry
  `changes/2026-09-09-adr-067-phase5-suites.md`; five TODO items closed, one
  added.
- **Committed and pushed** as `8db7ac6` (35 files, +928/-356).

## Open / In Progress

- **Phase 5's last two boxes need the operator**: the five-step live
  adversarial run written to `testing/2026-09-09-lan-mode-live.md`, and the
  browser criteria walked by hand. Everything else in Phase 5 is done.
- **Phase 6 is untouched** — all six boxes: `verifier`, `reviewer` +
  `security` over the diff, `documentation`, the release entry, MINOR, and
  `/preflight`.
- **A leaked test microVM is running and could not be removed.**
  `styrman-aab3ea11-aae9-4359-bb63-53b7434e3d08` (image `styrman-runtime:test`,
  up since 2026-09-09T12:02:55Z) is a leftover from one of this session's
  FAILED e2e runs, which abort before the UI cleanup section. Confirmed not a
  production project — production's store holds only
  `e441b4b6-3a6b-426c-bd89-e0da5f65ab20` ("test123"). The auto-mode classifier
  denied `docker rm -f` on it. The operator should run:

      docker rm -f styrman-aab3ea11-aae9-4359-bb63-53b7434e3d08
      docker network rm sdb-aab3ea11-aae9-4359-bb63-53b7434e3d08

- **NEW, NOT YET RECORDED AS A BUG — `web/e2e/run.sh`'s cleanup can never
  remove a container.** Its loop keeps a container only if the container's env
  contains `STYRMAN_STATE_DIR=${STATE}`, but **the orchestrator never sets
  `STYRMAN_STATE_DIR` on a project container** — verified by inspecting the
  production project's env, which carries `STYRMAN_USER`, `STYRMAN_UID`,
  `STYRMAN_GID`, `STYRMAN_CRED_BROKER`, `STYRMAN_PROJECT`,
  `STYRMAN_PROJECT_DIR`, `STYRMAN_DOCKER_DISK_GB` and nothing else, and by
  `grep -rn STYRMAN_STATE_DIR internal/orchestrator/ image/` returning
  nothing. So the safety net has always been a no-op and every aborted e2e run
  leaks its microVM. Ordinary green runs are cleaned by the SERVER when the
  suite deletes the project through the UI, which is why this stayed hidden.
  BUG-024's `|| continue` fix is still correct and still needed — it restored
  the `rm -rf` of the state dir — but it did not make this loop work.
  A fix wants a design decision (match `STYRMAN_PROJECT=<uuid>` against the
  ids the run created, or remove `styrman-<PID>` by name the way
  `integration.sh` does), so it was deliberately not made while writing a
  handoff. Candidate BUG-027.
- **`internal/orchestrator/ports.go` is committed with no unit test.** That
  package has no fake Docker client (BUG-003's residual, sized in TODO.md as
  its own task), so the browser e2e is its only cover. Named in the commit
  body and in BUG-023 so Phase 6's reviewer sees it rather than inferring it,
  but it is a real Definition-of-Done gap now sitting in `main`.
- **Phase 3's deferred box is still open**: converge this host's firewall and
  diff `iptables -S` against the pre-change capture. Not attempted this
  session; the classifier also denied reading `iptables -S`, so **the host's
  current chain state is unverified as of this handoff** — do not assume it
  has been converged.

## Key Decisions

- **Attribution: AGENTS.md wins.** The session carried a harness rule telling
  me to append `Co-Authored-By` and a `Claude-Session` URL to commits, stating
  it replaced earlier attribution guidance; AGENTS.md forbids both. Asked
  rather than resolving it silently, because once pushed the alternative would
  need a history rewrite, which AGENTS.md also forbids. Operator chose
  AGENTS.md — no trailers. `8db7ac6` carries none.
- **Commit and push even though Phase 5 is incomplete**, on the
  backup-and-versioning reading of AGENTS.md. Operator's call.
- **The pinned `:test` tag default stays.** It exists so a test run cannot
  re-stamp the `:dev` tag production boots — a real 2026-09-01 audit finding.
  The image was rebuilt by hand instead; keeping the default and fixing the
  staleness separately is the right split. See BUG-026 and the new TODO item.
- **The destructive toggle checks run LAST rather than being made
  non-destructive.** A toggle rebuilding the container is the design (Phase 4
  already says so in the dialog copy); the suite had simply not been reordered
  to match. Making the checks avoid the rebuild would have been testing
  something other than the feature.

## Corrections

None. The operator gave no behavioural corrections this session; the two
answers recorded under Key Decisions were decisions put to them, not
corrections of a mistake.

## Agent notes

- **The `changelog` agent silently overwrote a TODO.md line.** Its sixth edit
  (add a new inbox item) REPLACED the `run.sh` exit-code item instead of being
  added — and it reported item 6 as added. Caught by `git diff --numstat`
  reading `5 5` when an addition must raise insertions above deletions: the
  exact BUG-005 signal `.agents/rules/writing-files.md` already names.
  Recovered from `git show HEAD:TODO.md`, restored with its DONE annotation,
  and confirmed by diffing the full item list before and after (56 -> 57, none
  lost). **Verified, not speculative.** Worth noting that the existing rule
  caught this only because the numstat was checked by hand — nothing enforces
  it, and a subagent tripped it.
- The `bugs` agent's output was sound: six well-formed records, INDEX
  append-only. Two small corrections made by hand — an unbalanced paren, and
  an index row that conflated the SPA's 20 s abort with Playwright's 30 s
  default. The index is what gets read at resume, so it needed the real
  number.
- Both agents completed inside their limits, unlike the previous session's six
  stalled dispatches. Briefs carrying every fact up front is the plausible
  reason, but that is an inference from two data points, not a finding.

## Environment State

- Branch `main`, HEAD `8db7ac6`, clean, in sync with origin. No new tags —
  nothing released this session; latest tag remains v1.24.1.
- Production: `styrman-server:1.24.1` and `styrman-caddy:1.24.1` running. The
  production project `e441b4b6` ("test123") is STOPPED, still on
  `styrman-runtime:1.23.0`.
- `styrman-runtime:test` **rebuilt 2026-09-09** from current `image/`. It had
  been built 2026-09-04 08:22, before v1.22.0's shell skel, and was failing
  three integration checks that read like a source regression.
- One leaked test microVM still running — see Open, above.
- No leftover `/tmp/styrman-e2e-*` or `/tmp/styrman-itest-*` dirs, which is
  itself the check that BUG-024's fix holds.
- Host firewall chain state UNVERIFIED this session (classifier denied the
  read). ADR-067's converge step has not been run here.
- Nothing running from this session. Scratch under `/tmp/claude-1000/…` is
  disposable.

## Artifacts to Read

- `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md` — **the active
  plan; its unticked boxes are the remaining work.** Phase 5 is at 2 of 4,
  Phase 6 at 0 of 6, plus Phase 3's one deferred converge box.
- `.agents/docs/changes/2026-09-09-adr-067-phase5-suites.md` — what this
  session did and why, with the counts.
- `.agents/docs/bugs/BUG-021` … `BUG-026` — read BUG-021 and BUG-024 before
  touching `web/e2e/`, and BUG-023 before touching
  `internal/orchestrator/ports.go`.
- `.agents/rules/writing-files.md` — the rule that caught the TODO overwrite.
- `REQUIREMENTS.md` R12 (Appendix A of the plan is the approved text).

## Suggested Skills

- `bash` — anything in `web/e2e/` or `deploy/firewall/`.
- `preflight` — Phase 6's last box, once the live run is done.
- Dispatch `reviewer` and `security` for Phase 6: the ADR-067 diff touches the
  firewall, the credential broker and an authz path, and `ports.go` carries a
  behaviour change with no unit cover.

## Next Steps

1. **Remove the leaked test VM** (commands under Open) — it is holding a
   microVM's resources on the host right now.
2. **Phase 5's live adversarial run**: two projects, A in LAN mode and B in
   sandbox, all five steps including the `NET_ADMIN` manual-route attack R12
   names, written to `.agents/docs/testing/2026-09-09-lan-mode-live.md`.
   Operator + AI.
3. **Walk the browser criteria by hand**: flip a project to LAN, watch it
   restart, ping the gateway from a terminal, flip back, watch the ping time
   out. Operator.
4. **Decide the e2e cleanup filter** (candidate BUG-027) before Phase 6, so
   the suite stops leaking microVMs.
5. **Phase 6**, in the plan's order, ending at `/preflight` with a MINOR.
