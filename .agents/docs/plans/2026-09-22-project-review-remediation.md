---
owner: main
date: 2026-09-22
---
# Plan — act on the 2026-09-22 whole-project review   (2026-09-22)

## Why

`/review --project --all --high` at c05219c ran six lenses over the whole
repository and produced
`.agents/docs/reviews/2026-09-22-review-project-merged.md`: 14 findings plus
two coverage FAILs, one rebutted security finding, and a list of what no lens
read. Nothing is on fire — every Tier 1 command is green (311 + 53 Go tests,
178 frontend) and the platform is live at v1.37.0 on the NUC. The findings are
of three kinds, and the plan is ordered by which kind can still surprise us:

1. **Blind spots** — `internal/files/` was read by no lens, and the unexported
   dead-code sweep could not run. A finding there would outrank this entire
   plan, so it goes first, before any edit.
2. **Things that measure the wrong thing** — the release gate's dist freshness
   check compares file names, so a stale embedded asset ships under a green
   gate; the shellcheck list omits the shared fresh-host path. These are the
   BUG-034 family ("silence from your own instrumentation") and the reason the
   review found them at all.
3. **Descriptions that outlived their behaviour** — five copies of a build
   command that now fails, two documents describing deleted mechanisms, a
   retired command presented as current. The global context's rule that a
   behaviour change includes the text that describes it, applied after the fact.

Per-item context, options and trade-offs:
`.agents/scratch/2026-09-22-review-triage.md` (numbered to match the chat
table; ask by number).

## Scope

In:
- Phase 0: the two gaps the review itself named — a security lens over
  `internal/files/`, and a rebuilt `deadcode` binary so the unexported sweep can
  run. Findings from Phase 0 are folded into this plan before Phase 1 starts.
- Phase 1: docs truth — findings 2, 3, M1, 12, 13.
- Phase 2: the gates — findings 1 and 8, each with a must-fail proof.
- Phase 3: hardening that did not travel — findings 4, 5, 6, and the 3-line
  lock split from 9.
- Phase 4: the decisions that are the operator's — findings 7, 10, 11, 15.
- Phase 5: coverage evidence — run Tier 2 and Tier 4 on the live host to turn
  the verifier's "not examined" into a real answer (finding 17).

Out:
- Splitting `internal/httpapi/server.go` (1,405 lines) or
  `internal/orchestrator/orchestrator.go` (1,704). No lens read either for
  cohesion; refactoring an unread file is how a defect hides. Finding 9's lock
  split is in; the file split is not.
- Renaming the ingress module path (finding 14) — a whole-module import churn
  for no functional gain. Deferred to the next change that opens that module.
- Writing unit tests for the 33 `web/src` files before Phase 5 says which of
  them Tier 4 actually covers.
- A unit test for `cmd/wsprobe` (finding 16, struck — see below).
- Any deploy. A release carrying these changes is a separate `/preflight`.

Struck, with the reasoning recorded so a future repo-wide verifier run does not
re-file it:
- **Finding 16** — `cmd/wsprobe/main.go` has no test. It is a WS dev probe that
  `integration.sh` builds and invokes seven times; a unit test for it would test
  the harness, not the product. The verifier is correct to report it and correct
  to be overruled.

Follow-ups noted, not built:
- Ingress module path rename. found in: recon, 2026-09-22.
- `internal/links/store.go:96` `newToken()` predates `crypto/rand.Text()`; the
  fix changes token shape from 32 hex to 26 base32 chars, so it needs a grep for
  anything routing on that shape first. found in: simplify lens, 2026-09-22.
- `internal/httpapi/server.go` and `orchestrator.go` cohesion review. found in:
  recon + simplify, 2026-09-22.

## Phase 0 — close the review's own blind spots

Nothing is edited in this phase. It exists because a finding in
`internal/files/` would reshape everything after it.

- [x] `/review internal/files --security` — **done, FINDINGS**
      (`.agents/docs/reviews/2026-09-22-security-files.md`). Both rule claims
      HOLD: `openRoot` reaches the root as `os.OpenRoot(<base>/<tenant>)` +
      `Root.OpenRoot(rest)` (`files.go:126-135`), with no `os.OpenRoot(path)`,
      `os.ReadDir` or `os.Stat` by name below a mounted home; and no
      `DirEntry.Info()` survives in the package or any caller. **No path escape
      found.** Three new findings instead, carried to Phase 0b.
- [x] Rebuild `deadcode` against the go1.27.1 toolchain — **done**. Root module:
      `deadcode ./...` rc 0, zero unreachable functions, corroborating the
      simplify lens's independent hand-check. The silence was validated before
      being believed: a scratch module with a deliberately dead function reports
      it, so the tool can emit. Ingress module: `deadcode: no main packages` —
      it is one library package whose entry points are Caddy registry callbacks
      (`caddy.RegisterModule` in `init()`), so whole-program reachability from a
      main is not applicable and a synthetic main would report every handler as
      a false positive. Recorded as a tool limitation, not a gap.
- [x] Fold any Phase 0 finding into this plan — **done: Phase 0b below**, which
      also changes the plan's order. Phase 0 was written expecting to find
      nothing; it found a per-tenant DoS lever, so its findings now run before
      the docs phase.
- [x] **Checkpoint:** `changes/` entry, commit, push.

## Phase 0b — act on Phase 0's findings (`internal/files`)

Promoted ahead of Phase 1: these outrank a stale sentence. Findings 1 and 2 are
both the BUG-035 shape — a mechanism that exists in the package and did not
reach every caller — which is why they survived four earlier audits of this same
file.

- [x] `internal/files/files.go:191` — `List` does an uncapped `ReadDir(-1)` on a
      guest-owned directory. `ListNames:489-492` caps at `maxListNames` with the
      reasoning already written in the file ("the directory is guest-writable
      and this is polled — an unbounded ReadDir is a per-tenant lever on the
      shared server", security review 2026-09-03). `List` never got it. Bounded
      batched read, with the truncation surfaced to the caller rather than
      silently dropped.
- [x] `internal/files/files.go:232-246` — `sortEntries` is a hand-rolled
      insertion sort, O(n²), measured by the reviewer on a copy: 5k entries
      102ms, 20k 1.57s, 50k 9.83s single-core, in the one process serving every
      tenant. Replace with `slices.SortFunc` over the existing `less`, which is
      already a correct comparator.
- [x] Reproduce the cost before the fix and after it, on the real function
      rather than a copy: a benchmark with 50k entries, recorded as a
      must-improve proof. A fix to a performance finding that was never measured
      on the real path is not verified.
- [x] `internal/files/files.go:446` — `ReadRawNoFollow` opens the parent with
      `rt.Open`, i.e. `OpenFile(name, O_RDONLY, 0)`: the **only** open in the
      package without `O_NONBLOCK`. Lines 154, 186, 296, 353, 454, 484 and 518
      all carry it, several with explicit guest-planted-FIFO comments, and 454
      is the very next open in the same function. A guest replacing `~/.ssh`
      with a FIFO wedges `httpapi/setup.go:247`'s goroutine and its OS thread
      forever. Add `syscall.O_NONBLOCK`; test with a planted FIFO.
- [x] `internal/httpapi/explorer.go:99-111` — `managedHomeTarget` matches the
      first *string* component while `os.Root` follows in-root symlinks; the
      reviewer probed it (`x -> .dockerd`, then `Root.RemoveAll("x/disk.img")`
      succeeded). Same principal throughout, so this is not an escape — but the
      comment reads like a boundary, which is the defect. Either make it one or
      correct the comment to say what it actually does.
- [x] Close the test blind spot the reviewer named: only 4 of 15 root functions
      have a case below a `files.Trust` base, and the cross-tenant wipe test in
      `rmcheck_test.go` runs on the untrusted path — BUG-004's exact blind spot,
      in the test written to prevent it. Add the missing trusted-base cases.
- [x] **Both reviews found a bypass in the first version of the fix**, and it
      is fixed: `managedHomeTarget` inspected only the path's FIRST component,
      so `work/sub/link -> ../../.dockerd` walked straight past it while
      `os.Root` followed the link. `traversesALink` now lstats every component
      but the last. The test was written first and failed against the pre-fix
      code.
- [x] **Security also found one this phase created**: the new cap made a
      truncated listing render as complete, because nothing consumed
      `Truncated`. Fixed in `web/src/lib/api.ts` and
      `web/src/components/FileExplorer.tsx`. Not unit-tested — see Phase 5.
- [x] **Checkpoint:** `changes/` entry, commit, push. `reviewer` over the diff,
      and `security` (this is the confinement boundary). Both returned findings;
      outcomes are appended to each report.

## Phase 1 — docs truth (findings 2, 3, M1, 12, 13)

Runs after Phase 0b, which outranks it. Cheapest fix in the plan. Dispatched to the
`documentation` agent for the files it owns; the `.env.example` line is the main
agent's.

- [x] The ADR-074 build secret, five copies: `README.md:271`,
      `.agents/docs/deploy/local.md:21`,
      `.agents/docs/specs/styrman/guide.md:463`,
      `.agents/docs/specs/styrman/runtime-image.md:7`,
      `.agents/docs/testing/styrman/README.md:128` — each gains
      `GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN`,
      matching `AGENTS.md`'s Setup block, which is already correct.
- [x] `.agents/docs/deploy/local.md:31,36` — drop `STYRMAN_FIREWALL_DIR`
      (deleted with the ADR-067 spool).
- [x] `deploy/.env.example:63` — same variable, claims compose sets it; compose
      has no firewall reference and no code reads it. Main agent, not
      `documentation`.
- [x] `README.md:50-61` — replace the ADR-065/066 root-mediated LAN description
      (incl. SSH-key withholding, whose removal
      `internal/credbroker/broker_test.go:249` asserts) with the ADR-067 model
      `AGENTS.md` already states correctly.
- [x] `README.md:112` — "v1.34.1, deployed" → v1.37.0; extend the release list
      past v1.35.0; remove the retired ADR-073 harness channel and
      `styrman-ctl update --setup --harness-token-file`, which read as live
      instructions.
- [x] `.agents/docs/testing/styrman/README.md:8` — two named test files no
      longer exist; `:130-135` — drop the v1.16.x caveat.
- [x] Re-run the docs lens's own 8 `git grep` commands (end of
      `.agents/docs/reviews/2026-09-22-review-project-docs.md`) and paste the
      output in the change entry. A docs fix that does not re-grep is how the
      next copy survives. **It found two the lens had missed**, one real:
      `.agents/docs/specs/styrman/deploy.md:51` described `styrman-ctl
      install`'s `docker build` without the ADR-074 secret, while
      `deploy/styrman-ctl:490-496` builds `secret_args` and refuses to build
      without a token. Fixed. The other two hits are correct text: `versioning.md:199`
      carries `--secret` on its next line, and the testing README's
      `internal/netpolicy/script_test.go` mention says the file "is gone" and
      names its replacement.
- [x] `.agents/docs/specs/styrman/api.md` — the listing cap, `truncated`, and
      the symlink-traversal refusal from Phase 0b. Not in the review: this is
      Phase 0b's own describing-text obligation, found while assembling the
      brief.
- [x] **Checkpoint:** `changes/` entry, commit, push.

## Phase 2 — the gates that measure the wrong thing (findings 1, 8)

Both changes are green on arrival, which is their weakness. Each one lands with
a must-fail proof per `review-standards` §2: the check is seen red once, in the
state it is supposed to catch, before its verdict is trusted.

- [x] `deploy/verify-commit.sh:135-144` — compare a `sha256sum` set, not a
      `find | sort` name set. `web/scripts/normalize-shell.mjs` already makes
      `_shell.html` byte-reproducible; keep the diff-on-failure output.
- [x] Must-fail proof for the above: edit one byte inside the committed
      `_shell.html`, run the gate, record that it exits non-zero and names the
      file, restore. Record the command and its exit status in the log.
- [x] `deploy/verify-commit.sh:170` — add `deploy/host-setup.sh`,
      `deploy/styrman-ctl` and `image/entrypoint.sh` to the `shellcheck -S
      warning` list (BUG-068 names the first two; `shared-path-learnings.md`
      makes `host-setup.sh` the shared fresh-host path). All three pass at HEAD.
- [x] Must-fail proof for the above: introduce a deliberate warning in one of
      the three, see the gate go red, revert.
- [x] **Both proofs ran against the REAL gate on a real commit**, not a copy:
      a throwaway local branch carried a content-only edit to
      `web/dist/client/favicon.svg`. The OLD gate ran that commit to
      completion, rc 0, printing "committed assets match a fresh build"; the
      new one fails it at the freshness section. For shellcheck, an SC2034
      planted in `host-setup.sh` is invisible to the old line (rc 0) and
      named by the new one (rc 1).
- [x] **Checkpoint:** `changes/` entry, commit, push.

## Phase 3 — hardening that did not travel (findings 4, 5, 6, 9-lock)

- [ ] Move `env_get` into `deploy/gcp/lib.sh` with `styrman-ctl`'s `is_key`
      guard and awk-variable literal match; delete the fork in
      `deploy/gcp/install.sh`.
- [ ] Move `private_file` into `deploy/gcp/lib.sh` as one version taking an
      owner-policy argument; it has already diverged from `styrman-ctl`'s in 2
      of 5 checks (owner policy, and parent mode `&022` vs `&077`). Decide which
      of the two is correct for each check and say so in the change entry —
      this is a secret-file gate, and "merge them" is not the same as "pick the
      stricter one".
- [ ] Extend `internal/config/gcp_scripts_test.go` to cover both helpers through
      the dry-run path, including a key containing `/` (the case the original
      hardening was written for).
- [ ] `deploy/caddy/ingress/generator.go` — emit `request_header -X-Caddy-Role`
      before the three `header_regexp` matchers in `writeAppBlock`, matching
      what every other identity header on that path already does; add a
      generator test asserting the directive is present.
- [ ] `internal/httpapi/server.go` — give `credharvest.go`'s `credPushing` /
      `credPushRerun` their own `credMu` instead of sharing `deletingMu` with
      the project-lifecycle maps. Three lines; no file split.
- [ ] **Checkpoint:** `changes/` entry, commit, push. `reviewer` over the diff
      (Phase 3 crosses 5 files) and `security` (it touches the ingress and a
      secret-file gate).

## Phase 4 — the operator's decisions

Each needs an answer before it is built; the recommendation is mine, the call is
not. Full versions in `.agents/scratch/2026-09-22-review-triage.md` (items 7,
10, 11, 15).

- [ ] **Finding 7** — the R12 `ip_forward` assertion fails open on
      `fs.ErrNotExist` (`cmd/styrman/main.go:50-51`) while every other error
      exits 1. Recommendation: fail closed when containerized, keep the warning
      for a bare `go run`; test both branches. Blocked on the operator: it can
      stop a production boot.
- [ ] **Finding 10** — ~125 lines of converge code for superseded ADR-065/066/073
      (`deploy/update/styrman-update:178`, `deploy/firewall/install.sh:65`,
      `deploy/firewall/styrman-firewall.sh:118`, `deploy/firewall/test.sh:97`)
      with no reachable consumer. Recommendation: keep for now, add a one-line
      retirement note at each block naming the release that will delete them;
      delete once the GCP target is used in anger. Blocked on the operator:
      deleting converge code is irreversible for a host frozen between v1.23
      and v1.35.
- [ ] **Finding 11** — `go mod tidy` promotes `opencontainers/image-spec` from
      indirect to direct (mine, unblocked). Separately, it and
      `golang.org/x/sys` are direct dependencies absent from
      `REQUIREMENTS.md`'s approved list; that file is immutable without
      explicit approval, so the route is: propose the exact edit → approval →
      ADR → `REQUIREMENTS.md` and `ROADMAP.md` in one commit.
- [ ] **Finding 15** — extend the ADR-069 audit note at `image/Dockerfile:47-56`
      by two sentences naming the residual it does not cover: the fetch of the
      `agy` installer is TLS-only, and the manifest sha512 check happens inside
      a script that was not itself verified. No code change — the exception
      stands; the note currently implies more coverage than it has.
- [ ] **Checkpoint:** `changes/` entry, commit, push.

## Phase 5 — turn the coverage unknown into evidence (finding 17)

The verifier's FAIL on 33 of 47 `web/src` files is a *not examined*, not a
*confirmed uncovered*: `testing/styrman/README.md` says Tier 4 covers most of
them, and Tier 4 needs the live host.

- [ ] `bash .agents/docs/testing/styrman/integration.sh` (Tier 2, 115 checks) on
      the NUC — the suite no lens ran this wave.
- [ ] `bash web/e2e/run.sh` (Tier 4) — then map which of the 33 files it
      actually reaches.
- [ ] Assert the explorer's truncation notice in Tier 4: a directory past
      `files.maxListEntries` must render "Some are not shown". Added in Phase
      0b with no unit test, because `FileExplorer.tsx` has none and the project
      has no component-testing infrastructure — standing that up is a
      dependency decision, not a side effect of a security fix. Until this
      box is ticked, that notice is type-checked and linted, not proven to
      render.
- [ ] Record the real uncovered list in `.agents/docs/testing/styrman/README.md`
      and decide per file: unit test, accept as e2e-covered, or accept as
      untested with a reason. No blanket answer.
- [ ] **Checkpoint:** `changes/` entry, commit, push.

## Done when

Every unticked box above is ticked or struck with a reason; the merged review
record has its `## Outcome` section appended (one line per finding: fixed with
file:line, rebutted with a checkable reason, or left with an owner), written in
the same commit as the last fix; `changes/` carries an entry per phase; and a
handoff says nothing is queued.

This plan does not ship anything. A release carrying these changes is a separate
`/preflight`, and `.agents/docs/specs/styrman/versioning.md` decides the number:
on the current contents, a PATCH — every item fixes released behaviour or a
document, and nothing adds behaviour a user of the deployed platform reacts to.
