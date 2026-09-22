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

- [ ] `/review internal/files --security` — `openRoot`'s own confinement and the
      BUG-004 `DirEntry.Info()` prohibition, which no lens verified. The call
      sites were traced (`orchestrator.go:344/411/708/758`, `files.Trust` at
      `httpapi/server.go:206`) and cover create and start; the package's
      internals were not read.
- [ ] Rebuild `deadcode` against the go1.27.1 toolchain (the installed binary is
      go1.26 and errors out), run it over both modules, and record the result —
      including "nothing found", which is the likely answer and is evidence.
- [ ] Fold any Phase 0 finding into this plan as a new phase task before Phase 1
      begins. If Phase 0 finds nothing, say so in the checkpoint entry: a clean
      result here is the point of the phase, not a wasted phase.
- [ ] **Checkpoint:** `changes/` entry, commit, push.

## Phase 1 — docs truth (findings 2, 3, M1, 12, 13)

Highest severity in the review and the cheapest to fix. Dispatched to the
`documentation` agent for the files it owns; the `.env.example` line is the main
agent's.

- [ ] The ADR-074 build secret, five copies: `README.md:271`,
      `.agents/docs/deploy/local.md:21`,
      `.agents/docs/specs/styrman/guide.md:463`,
      `.agents/docs/specs/styrman/runtime-image.md:7`,
      `.agents/docs/testing/styrman/README.md:128` — each gains
      `GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN`,
      matching `AGENTS.md`'s Setup block, which is already correct.
- [ ] `.agents/docs/deploy/local.md:31,36` — drop `STYRMAN_FIREWALL_DIR`
      (deleted with the ADR-067 spool).
- [ ] `deploy/.env.example:63` — same variable, claims compose sets it; compose
      has no firewall reference and no code reads it. Main agent, not
      `documentation`.
- [ ] `README.md:50-61` — replace the ADR-065/066 root-mediated LAN description
      (incl. SSH-key withholding, whose removal
      `internal/credbroker/broker_test.go:249` asserts) with the ADR-067 model
      `AGENTS.md` already states correctly.
- [ ] `README.md:112` — "v1.34.1, deployed" → v1.37.0; extend the release list
      past v1.35.0; remove the retired ADR-073 harness channel and
      `styrman-ctl update --setup --harness-token-file`, which read as live
      instructions.
- [ ] `.agents/docs/testing/styrman/README.md:8` — two named test files no
      longer exist; `:130-135` — drop the v1.16.x caveat.
- [ ] Re-run the docs lens's own 8 `git grep` commands (end of
      `.agents/docs/reviews/2026-09-22-review-project-docs.md`) and paste the
      output in the change entry. A docs fix that does not re-grep is how the
      next copy survives.
- [ ] **Checkpoint:** `changes/` entry, commit, push.

## Phase 2 — the gates that measure the wrong thing (findings 1, 8)

Both changes are green on arrival, which is their weakness. Each one lands with
a must-fail proof per `review-standards` §2: the check is seen red once, in the
state it is supposed to catch, before its verdict is trusted.

- [ ] `deploy/verify-commit.sh:135-144` — compare a `sha256sum` set, not a
      `find | sort` name set. `web/scripts/normalize-shell.mjs` already makes
      `_shell.html` byte-reproducible; keep the diff-on-failure output.
- [ ] Must-fail proof for the above: edit one byte inside the committed
      `_shell.html`, run the gate, record that it exits non-zero and names the
      file, restore. Record the command and its exit status in the log.
- [ ] `deploy/verify-commit.sh:170` — add `deploy/host-setup.sh`,
      `deploy/styrman-ctl` and `image/entrypoint.sh` to the `shellcheck -S
      warning` list (BUG-068 names the first two; `shared-path-learnings.md`
      makes `host-setup.sh` the shared fresh-host path). All three pass at HEAD.
- [ ] Must-fail proof for the above: introduce a deliberate warning in one of
      the three, see the gate go red, revert.
- [ ] **Checkpoint:** `changes/` entry, commit, push.

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
