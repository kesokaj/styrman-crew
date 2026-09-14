---
owner: main
date: 2026-09-14
---
# Preflight — v1.31.0 (2026-09-14)

Range reviewed: v1.30.1..cde7ec2 (plus the release commit)

Release: **MINOR** — every project home receives a refreshed agent harness
(styrman-harness `8046e4d5f24b009bd2461ebef28931cade7a4acd`, up from
`6893e2218414617da2bb88259b905ca3882b2659`): `/review` (which replaces
`/audit`), `sh-agent install --vendor`, the rewritten global context and
agents, 27 skills instead of 28 (`gcloud-auth-verification` folded into
`gcloud-setup`), and an install that treats the image's mirror as the plain
copy it is — never a `go build`, never a git call inside the VM (the
harness's BUG-015 fix, 2c8990e, the floor this refresh was pinned to). New
behaviour for a user of the released artifact, nothing removed that worked:
MINOR under the global versioning rule. Requested by the operator through
the harness's mirror-refresh prompt (styrman-harness
`.agents/docs/plans/2026-09-14-mirror-refresh.md`), which names v1.31.0.

Pre-gate commit: `cde7ec2` ("harness mirror: styrman-harness 8046e4d …").
The rest of the range is the v1.30.1 close-out (handoffs, README status).

## VERDICT: SHIP

`deploy/verify-commit.sh cde7ec2` is green on a detached worktree of the
commit — including the new gate line that proves the built image's mirror
carries no `.git` — the integration suite is green on the image built from
this tree, three review rounds converged (round 3 reported only lows, all
fixed or rebutted below), the docs lens reported nothing, and TODO/ROADMAP
are empty by the operator's rule.

## Findings (FAIL-first)

None blocking.

- **[medium / high — REBUTTED, carried to the operator for the harness]**
  Security round 1: the harness's "Root steps on the operator's own Linux
  box … run it — do not hand it back" bullet (`image/harness/context/
  dev-setup.md`) is rendered into every project VM's `AGENTS.md`, where the
  project user already holds `NOPASSWD: ALL`; the `.claude/settings.json` it
  cites does not exist in a guest. Rebuttal: inside a project microVM the
  user already holds sudo and the docker group by design
  (`image/entrypoint.sh:55,57`; one Kata VM per project, R12 isolates
  projects from each other), so the sentence changes when an agent asks
  before a root step inside its own VM, not what it can reach — root in the
  guest reaches nothing a non-root project user cannot (the broker serves by
  bridge IP, not uid; the Docker socket is already the user's). The mirror is
  verbatim by ADR-062 (styrman never edits it; a styrman-side workaround
  belongs at the entrypoint call site, and the 2026-09-03 directive rules out
  a settings.json or hooks there). Round 2: "disposition holds … severity is
  fairly low here"; one overstatement corrected — the host firewall bounds a
  SANDBOX project only; a LAN-mode project reaches the LAN by design
  (ADR-067), root or not. Left: the sentence could be scoped by a
  project-level rule in the harness repo — in the handoff for the operator.
- **[low / high — FIXED in cde7ec2, twice]** Security round 1: dropping
  `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1` from the entrypoint's
  install line is correct (every git call site in the mirror sits behind
  `harness.IsCheckout`, a bare `os.Lstat(<repo>/.git)`; `Inspect` returns
  before any exec on a plain copy — traced independently by the code
  reviewer and the security reviewer), but the no-`.git` property was
  asserted nowhere. Round-1 fix: `test ! -e /styrman/harness/.git` in
  `image/Dockerfile`. Round 2 (security) found it could never fire —
  `image/.dockerignore` strips `.git`/`**/.git` from the build context — and
  the round-2 verifier proved it (a planted `harness/.git` still built, rc 0,
  with and without cache). The mechanism was DELETED, not repaired: the
  Dockerfile and entrypoint comments now name `.dockerignore` as the
  guarantee. Round 3 asked for the one place the release path can prove it:
  `deploy/verify-commit.sh` now runs `test ! -e /styrman/harness/.git` in the
  built runtime image (its line `runtime image's mirror is a plain copy` is
  in the cde7ec2 run below). Recorded as BUG-059 (an assertion trusted
  without being made to fail once).
- **[low / high — FIXED in cde7ec2]** Reviewer round 1: the new integration
  check "boot log records the harness deployed at the mirror's commit" read
  the commit from the container it was checking (a constant against itself)
  and, on a failed inner `docker exec`, a truncated pattern still matched
  (failed open). Now: the commit comes from the host file
  `${REPO}/image/harness/COMMIT`, the version from the container, both
  length-checked, `grep -F … >/dev/null`. Round 2 rebuilt `check()`'s `eval`
  with a docker stub and ran four cases (match → PASS; failed exec, wrong
  commit, missing host file → FAIL): closed.
- **[low / high — FIXED in cde7ec2]** Reviewer round 1: `VENDORED.md` was
  silent on the mirror's own `go test ./...` being red by construction
  (`TestDryRunNeverBuilds` assumes a git checkout — BUG-058, upstream). §
  Refresh now says so and that the gate vets, never tests, the mirror;
  `SHA256SUMS` regenerated (checksums cover VENDORED.md). Closed in round 2.
- **[low / high — FIXED]** Reviewer round 2: the round-1 verifier log and the
  change entry recorded a green run that predated the fixes. The suite was
  re-run on every rebuilt image (12:17 with the Dockerfile guard, 12:33
  without it; both `passed=94 failed=0 skipped=3`), the verifier's round-2
  and round-3 logs record it, and the change entry gained a "Review rounds"
  section.
- **[low / high — FIXED in cde7ec2]** Security round 3: the no-`.git`
  invariant asserted nowhere on the release-gate path — the verify-commit
  line above.
- **[low / medium — FIXED (comment) + expiry recorded]** Security round 3:
  "always a plain copy" is a build-time property written as a runtime one —
  the guest's own root can create `/styrman/harness/.git` in its container.
  Comment now says "built as a plain copy". Expiry condition: a project
  that does so makes its NEXT boot's install take the rebuild path and fail
  non-fatally (`WARN … harness not deployed this boot`, retried; the
  timeout bounds it) — self-inflicted, same uid, its own VM only.
- **[low / medium — FIXED (comment)]** Security round 3: the entrypoint
  comment claimed the capped steps stay "inside the server's 90 s ready
  wait in any case"; four sequential `timeout 20` steps sum to 80 s, inside
  90 s only just. Comment corrected; no code change (each step finishes in
  well under a second; the caps bound a stall).
- **[low / high, low / medium — REBUTTED, carried upstream]** Security round
  1: manifest paths from `~/.config/sh-agent/state.json` joined without
  `filepath.IsLocal` (`apply.go`); frontmatter `name:` becomes a path
  (`inventory.go`/`plan.go`). In this project both run as the tenant, on the
  tenant's own home, inside the tenant's own VM: no privilege or tenant
  boundary crossed; the same class was carried to the harness owner on
  2026-09-03 (TODO history, line 84). Upstream code is never edited in the
  mirror. Rounds 2 and 3: dispositions hold; expiry condition — void the day
  `sh-agent` runs as root in the image.
- **[low / low — REBUTTED]** Security round 1: predictable `Reinstall` build
  directory. Unreachable on a plain copy (`IsCheckout` false; the image is
  built as one by `.dockerignore`). Holds in rounds 2 and 3.
- **[low / low — FIXED, residuals LEFT]** Security round 1: container-sourced
  strings reached `check()`'s `eval` in the new boot-log check. Fixed with
  the reviewer's item above (variables, `grep -F`). Residuals (rounds 2–3):
  `${REPO}` is still interpolated into the eval'd string (the operator's own
  checkout path); `[ -n "$v" ]` would pass a value with an embedded newline;
  `/styrman/VERSION` is writable by the guest's root. Left: the container
  under test is the suite's own, and a hostile value yields a false PASS,
  never execution. A BuildKit edge (`image/Dockerfile.dockerignore` would
  supersede `.dockerignore`) noted; no such file exists.
- **[docs lens — APPROVE, no findings]** Every live claim about the
  harness matches the tree (27/7, the plain-copy wording, 97 checks, the
  dropped guard); only dated records still say "28 skills", by design.

## Review rows

| Row | Artifact | Result |
|---|---|---|
| Reviewer, round 1 (opus) | `reviews/2026-09-14-review-harness-refresh-8046e4d.md` | APPROVE WITH NITS — two lows, both fixed |
| Reviewer, round 2 (opus) | `reviews/2026-09-14-review-harness-refresh-8046e4d-r2.md` | APPROVE WITH NITS — round-1 items closed; one low (stale artifacts), fixed |
| Security, round 1 (opus) | `reviews/2026-09-14-security-harness-refresh-8046e4d.md` | FINDINGS — 1 medium rebutted, lows as above |
| Security, round 2 (opus) | `reviews/2026-09-14-security-harness-refresh-8046e4d-r2.md` | FINDINGS — all dispositions hold; the Dockerfile guard unreachable (deleted) |
| Security, round 3 (opus) | `reviews/2026-09-14-security-harness-refresh-8046e4d-r3.md` | FINDINGS — three lows, all fixed above; round-2 item closed; no `.dockerignore` hole |
| Docs lens | `reviews/2026-09-14-review-harness-refresh-8046e4d-docs.md` | APPROVE — nothing |
| Verifier, round 1 | `testing/2026-09-14-verify-harness-refresh-8046e4d.md` | PASS, 5 steps (the agent hit its turn limit once and was resumed to write the log) |
| Verifier, round 2 | `testing/2026-09-14-verify-harness-refresh-8046e4d-r2.md` | Steps 1–4a PASS; **4b FAIL by design of the step** — it proved the Dockerfile guard could not fire (the round-2 security finding), which is why the guard was deleted |
| Verifier, round 3 | `testing/2026-09-14-verify-harness-refresh-8046e4d-r3.md` | Steps 2–3 PASS (`passed=94 failed=0 skipped=3` on the 12:33 image); **steps 1 and 4 FAIL against the dispatch's own wording**, not the code: the dispatch called the entrypoint delta "one comment clause" while asking for a diff against HEAD (which holds the whole task's change — the dropped env vars, reviewed in rounds 1–2), and the expected `grep -c GIT_CONFIG_GLOBAL` of 0 ignored the new comment quoting the name. The facts the step meant to prove hold: the only non-comment line in `git diff v1.30.1 -- image/entrypoint.sh` is the install line losing the two variables; the exact string `GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM` is absent from the image's entrypoint (verifier round 1, step 4: count 0) |

Rounds: three, converged — round 3's findings were all low and are fixed or
rebutted above. The round-2 finding landed on the mechanism round 1 had
added; the rule's question ("what is the mechanism for?") answered itself
from the code (`.dockerignore` already enforces the property), so it was
deleted rather than repaired.

## Verification observed by the main agent (bare, exit status read)

- `go test ./...` rc 0 (15 packages ok); `(cd deploy/caddy/ingress && go test ./...)` rc 0; `go vet ./...` rc 0.
- `bash deploy/vulncheck.sh` rc 3 — GO-2026-4887/4883 (docker/docker, daemon-side, no fix; accepted per release by the script's header) in the root and ingress modules; `image/harness`: "No vulnerabilities found".
- `(cd web && npm run check)`, `npm run lint`, `npm test`: rc 0, 0, 0 (178 tests, 15 files).
- Mirror: `diff -rq` against `~/workspace/styrman-harness` (clean, at 8046e4d, `merge-base --is-ancestor 2c8990e HEAD` true, no ignored files outside `.agents/`) → exactly the three `Only in image/harness` lines; `sha256sum -c` OK over 154 files; file list matches; no symlinks; `go run ./cmd/sh-agent list` → `### Skills (27)` / `### Subagents (7)`; `go vet ./...` rc 0.
- Tier 2, `integration.sh styrman-runtime:test`: run 1 `passed=93 failed=1 skipped=3` (the new status check red on SIGPIPE 141 — BUG-057, fixed); run 2 `94/0/3`; run 3 `94/0/3` (image rebuilt 12:17 with the Dockerfile guard); run 4 `94/0/3` (image rebuilt 12:33 from the final tree). Boot→ready 2.98 s on run 2.
- Image (`styrman-runtime:test`): `list` → 27/7; `install --dry-run --repo /styrman/harness --home /check` → `[dry-run] plain copy, not its own git checkout: the running binary would be installed as is`; `status` in a fresh home → `repo       /styrman/harness (plain copy, not its own git checkout; update is refused)`, `drift none — 259 files match the repo`; no `.git` under `/styrman/harness`; the install line in `/styrman/entrypoint.sh` carries no override.
- `deploy/verify-commit.sh cde7ec2` rc 0: build, vet, race tests, ingress module, dependency agreement, mirror integrity + builder pin, `web/dist` freshness, host scripts, gofmt, frontend trio, server image, runtime image ("seeds 27 skills, 7 subagents (styrman-harness 8046e4d)", "runtime image's mirror is a plain copy (no /styrman/harness/.git)"), "OK: commit cde7ec24cf0a builds and tests clean in isolation — safe to tag."

## Trail freshness

- Newest `changes/` entry: `2026-09-14-harness-refresh-8046e4d.md` — this work (with a "Review rounds" section). PASS.
- No artifact under `.agents/docs` carries a future `date:`. PASS.
- `changes/` and `DECISIONS.md` append-only over the range (`git log --diff-filter=M v1.30.1..HEAD` empty). PASS.
- `ROADMAP.md`/`TODO.md`: touched only by the v1.30.1 close-out handoff; both empty by the operator's rule and stay so (report-only). The release commit updates ROADMAP's "newest entry" line.
- Secret scan of the staged pre-gate diff and of the release commit (no gitleaks on this host; the preflight skill's seven grep patterns over added lines): no hits, apart from the first draft of this very line quoting the patterns; no secret-shaped paths.

## What was not checked

- Tier 4 browser e2e (`web/e2e/run.sh`): nothing under `web/` changed; `web/dist` freshness and the frontend trio ran inside verify-commit.
- The mirror's vendored markdown (skills, agents, context) was read by sampling only — for shell, fetched URLs, and instructions that would write outside a project (security round 1: clean); its fidelity to upstream is proven by `diff -rq` and `SHA256SUMS`, its prose is upstream's.
- The mirror's own test suite is red by construction (BUG-058) and is not a gate here.
- A fresh Google sign-in on the deployed version, and opening a project in the browser on the new runtime tag: operator checks after the deploy (versioning.md step 5).
- LAN-mode projects, the Mac/Antigravity side of the harness: out of scope.
- The deploy itself is recorded below once the updater has acted on the tag.
