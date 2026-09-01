# Review — harness vendor sync v1.15.0   (2026-09-01)

Scope: the uncommitted working-tree change (71 paths). Vendored harness CONTENT
(`image/harness/**`, minus `VENDORED.md`) reviewed only for mirror fidelity and
integration, per ADR-046 and the dispatch. Everything else — `image/entrypoint.sh`,
`image/Dockerfile`, `internal/**`, and all styrman-side prose — reviewed normally.

## Verdict: REQUEST CHANGES

The mirror itself is clean: `diff -rq image/harness /home/kesokaj/workspace/devshell_prompt/harness`
reports exactly one difference (`Only in image/harness: VENDORED.md`), upstream is
clean at `6132b97`, and all 25 deletions correspond to upstream deletions in that
same commit — nothing styrman-specific was destroyed by the `rsync --delete`.
The problem is what the integration review did **not** measure. The harness deploy
runs synchronously on the container boot path before `/run/styrman/ready`, and its
last step shells out to `agy -p "/skills"`, which takes a **hard 60 s** whenever the
home has no Antigravity OAuth token — i.e. on **every newly created project**, whose
home is an empty `MkdirAll` seeded only from the image. Measured, not inferred. That
blows the REQUIREMENTS.md R1 latency SLA ("new project → agent prompt visible < 5 s,
hard limit < 15 s") by 4x and directly contradicts ADR-045's "~5–10 s Kata cold
boot". It is pre-existing upstream behaviour, not introduced by this sync — but the
dispatch asked whether the entrypoint's assumptions still hold, and on latency they
do not, and none of the "already verified" evidence covers the case. Beyond that:
there is no `v1.15.0` git tag, so the documented deploy path cannot reproduce the
version the docs claim shipped; the sync procedure that must preserve `VENDORED.md`
is written down nowhere as a runnable command; and the verifier DoD artifact is
missing.

## Findings

- **[high / confidence: high]** `image/entrypoint.sh:116-135` (+ `image/harness/antigravity/setup.sh:238-271`, out-of-scope to edit) —
  The write-once harness seeding blocks the boot path for **~60 seconds** on a home
  with no agy OAuth token. Measured in a throwaway container off the shipped image:
  `bash /styrman/harness/setup.sh` with `HOME` set to an empty dir → `rc=0`,
  **60 s**; the isolated call `agy -p "/skills"` → `rc=1`, **61 s**; with
  `--network none` → still **60 s** (a fixed internal timeout, not a network stall).
  A new project home is created empty (`internal/orchestrator/orchestrator.go:262-264`)
  and the broker only materializes `~/.gemini/antigravity-cli/antigravity-oauth-token`
  from a **login shell** (`/etc/profile.d/zz-styrman-credentials.sh`), which has not
  run at entrypoint time — so every new project pays it. Contrast the one live
  re-seed used as evidence (project `928e709a`): that home already carried a valid
  token from its 1.13.1 era, so the whole entrypoint finished in ~6.5 s
  (created `06:52:19.75` → `agent harness deployed` `06:52:26.22`). The evidence set
  therefore measured the fast case only.
  Impact: `waitReady`'s 90 s budget (`orchestrator.go:303,578`) absorbs it, so
  nothing fails — it just silently violates R1 (< 5 s target / < 15 s hard limit) and
  ADR-045's stated boot-latency assumption. `integration.sh:71-74` polls readiness in
  a `seq 1 60` loop with no latency assertion, which is why it never surfaced.
  Fix (styrman-side, no edit inside `image/harness/`): invoke the harness with a PATH
  that excludes `/usr/local/bin`, so the setup script's `command -v agy` guard skips
  the self-check entirely. Nothing `setup.sh` needs lives there — `rsync`, `python3`,
  `git`, `jq`, `bash` are all in `/usr/bin`; `/usr/local/bin` holds only `agy`, `go`,
  `yq`, `helm`, `tofu`, `styrman-*`. Concretely, change the inner call to
  `runuser -u "${USERNAME}" -- env HOME="${HOME_DIR}" PATH=/usr/bin:/bin:/usr/sbin:/sbin bash "${HSRC}/setup.sh"`.
  Then re-measure a cold create end to end and record it in the verification log.
  (Alternative: move the whole block to the background and stamp the marker on
  completion — more invasive, and it races terminal creation. Do not wrap it in
  `timeout`: the agy self-check is the LAST step of `antigravity/setup.sh`, so a
  timeout there kills the run before `claude/setup.sh` ever executes, and the marker
  is never written — a permanent re-deploy loop on every boot.)

- **[medium / confidence: high]** repo root — **No `v1.15.0` git tag exists.**
  `git describe --tags --always` → `v1.14.2-3-gc57abb2`, and that is exactly what the
  documented deploy path feeds to `STYRMAN_VERSION`
  (`README.md:128`, `AGENTS.md:39`, `.agents/docs/specs/styrman/guide.md:301`,
  `deploy/host-setup.sh:319`). The changelog and `ROADMAP.md` assert the image was
  built at `STYRMAN_VERSION=1.15.0` and `/api/version → 1.15.0` — true of the
  hand-built local image (`/styrman/VERSION` in `styrman-runtime:dev` reads `1.15.0`,
  confirmed), but not reproducible from the repo by anyone following the docs. A
  rebuild via the documented command would stamp `v1.14.2-4-g<sha>`, re-seed every
  home again, and report a version that matches no document. Tag `v1.15.0` as part of
  this release.

- **[medium / confidence: high]** `image/harness/VENDORED.md` + `.agents/docs/architecture/DECISIONS.md` (ADR-046) —
  The one procedure that keeps the mirror correct is not written down. ADR-046 says
  "brought in only by a full re-copy from `devshell_prompt/harness`" and VENDORED.md
  says "refresh by re-copying"; neither records that the re-copy is an
  `rsync -a --delete`, which **deletes `VENDORED.md` itself** — the very file both
  documents designate as the styrman-only exception. It survived this sync because
  the author remembered to restore it; the next sync has nothing to remind them.
  Record the exact command in VENDORED.md, e.g.
  `rsync -a --delete --exclude VENDORED.md ~/workspace/devshell_prompt/harness/ image/harness/`.
  Note also that upstream now ships a second, unrelated `image/harness/shared/skills/VENDORED.md`,
  so any `--exclude VENDORED.md` must be anchored (`--exclude /VENDORED.md`) or it
  will also skip the upstream one.

- **[medium / confidence: high]** `.agents/docs/testing/` — **The verifier DoD row is
  unfilled.** There is no `2026-09-01-verify-*.md`; the verification evidence lives
  only inside the changelog's "Verification" section. `.agents/docs/reviews/` has
  nothing newer than 2026-08-27 (this report is the first). Per the global
  constitution the verifier artifact is due before any completion claim or commit, and
  `/preflight` will not pass without it. Related and worth deciding now: the
  `trail-check.sh` this release ships looks for `testing/<date>-verify-*.md`
  (`trail-check.sh:114-115,127`), a filename convention this repo has never used — so
  the gate will report an owed verifier row on styrman-crew from day one. Adopt the
  convention or document the divergence.

- **[low / confidence: high]** `image/entrypoint.sh:113-115` and `:124` — Comments are
  now stale: "deploys into `~/.claude` and `~/.gemini`" and "the HOME deploy
  (`~/.claude`, `~/.gemini`) still lands". The harness deploys to a **third** location,
  `~/.agents/bin/` (`claude/setup.sh:213-230`, `antigravity/setup.sh:194-212`), and
  that location is the one carrying the executable gates the Claude hooks invoke.
  `image/README.md`, the runtime-image spec, VENDORED.md, the changelog and ADR-046
  were all updated for the third location; the code comment right next to the call
  site was not. Update both comments.

- **[low / confidence: high]** `.agents/docs/changes/2026-09-01-harness-upgrade-v1.15.0.md:21` —
  File counts are wrong and internally inconsistent: "19 modified, 19 added, 25
  deleted (136 → 150 files)". Actual: **20** modified (`git status --porcelain -- image/harness | grep -c '^ M'`),
  **39** added files (19 is the count of untracked *entries*; `git ls-files --others --exclude-standard image/harness | wc -l` = 39),
  25 deleted. The stated 136 → 150 only reconciles with 39 added
  (136 − 25 + 39 = 150), so the entry contradicts itself.

- **[low / confidence: high]** `.agents/docs/changes/2026-09-01-harness-upgrade-v1.15.0.md:33` —
  "Both exit 0 silently outside a git repo or a project with no `changes/` dir" is
  true of hook mode only. `trail-check.sh --check` **prints and exits 1** outside a
  git repo (`trail-check.sh:147-151`); `/preflight` and `/handoff` call it in exactly
  that mode. Narrow the claim to hook mode.

- **[low / confidence: medium]** `internal/httpapi/setup.go:230-252` — The Settings
  tool cards report the harness roster (`Skills`, `Agents`, `Commands`) from
  `s.cfg.UserHome(u.Username)` — the **base/setup home**, which the entrypoint never
  seeds because the setup VM runs with `STYRMAN_MINIMAL=1`
  (`orchestrator.go:565` → `entrypoint.sh:116`). Confirmed on the host:
  `/var/lib/styrman/homes/ostling-simon/.claude/agents` does not exist. So the
  headline of this release — 28 skills, 6 subagents — can never surface in the UI;
  the three fields are permanently empty (`omitempty`). Pre-existing, not caused by
  this change, but this release is the moment it matters. Either point the lookup at a
  project home or delete the three fields as dead reporting.
  Secondary: if it is ever pointed at a seeded home, `listNames` (`setup.go:167-178`)
  only strips a `.md` suffix, so the re-seed's backup files would be listed as
  agents/commands — verified present on a real home:
  `~/.claude/agents/testing.md.bak.20260901-065226`. Filter `*.bak.*` if the lookup
  is repointed.

- **[low / confidence: high]** `ROADMAP.md:17-18` — A code-span path is split across a
  newline: `` `.agents/docs/specs/styrman/ `` / `` runtime-image.md` ``. Markdown
  renders that as `.agents/docs/specs/styrman/ runtime-image.md` — an unusable path.
  Keep the span on one line. Also, "Now" describes work that is complete at commit
  time, so it is stale the moment it lands; either phrase it as the released state or
  leave "Now" empty per ADR-045.

- **[low / confidence: medium]** `image/harness/VENDORED.md:20-21` — "So a harness
  downgrade of a home only happens on a version bump" reads as the opposite of what is
  meant (the paragraph is about retiring dropped content on *upgrade*). Replace
  "downgrade" with "change" or "re-seed".

## Criteria

1. **Dead code — pass.** No styrman-side dead code introduced. All 25 vendored
   deletions match upstream deletions in `6132b97` (verified per file with
   `git log --diff-filter=D` in the upstream repo); no styrman file references the
   removed `code-review`/`skill-creator` skills or the `testing` agent — the surviving
   `/code-review` mentions in older changelogs/handoffs refer to the vendor built-in
   slash command, which the constitution explicitly leaves untouched. No empty
   directories, no ignored-but-expected files (`git status --ignored -- image/harness`
   is clean). Closest thing to dead code is the always-empty Settings roster in
   `setup.go` (finding above), which is pre-existing.
2. **Flow tracing — fail (one high finding).** Traced `entrypoint.sh:116-135` →
   `harness/setup.sh` → `antigravity/setup.sh` + `claude/setup.sh` → the three deploy
   targets, and the exec-bit chain (`setup.sh` requires `-x` on the vendor scripts;
   all three are `100755` in git and preserved by `COPY`/`cp -r`). Error handling is
   sound: `set -euo pipefail` in every script, a failure leaves the marker unwritten so
   the next boot retries, and the entrypoint's `|| log WARN` keeps it non-fatal. The
   marker logic (`~/.styrman/.harness-version` vs `/styrman/VERSION`) is correct and
   the mktemp indirection is still required (`claude/setup.sh:76` writes back into its
   own tree). What does **not** hold is timing: the deploy is synchronous ahead of
   `/run/styrman/ready` (`entrypoint.sh:266`) and costs ~60 s on an unauthenticated
   home. Downstream, `internal/backup/backup.go:22` excludes only `.dockerd`, so
   `~/.agents/` is archived and restored intact — verified by reading `Create`/`Extract`;
   nothing else in `internal/` or `cmd/` enumerates or prunes home subdirectories.
3. **Standards conformance — fail (R1).** No Go or TypeScript changed, so the
   `golang`/`typescript` skills do not apply; `go build`/`go vet`/`go test` were
   reported clean and no source is in the diff to re-litigate. The REQUIREMENTS.md R1
   latency SLA is violated by the boot-path finding — high severity and blocking per
   the review contract. Everything else conforms: no new dependency, AMD64-only image
   unchanged, no secrets, no auth/crypto surface touched.
4. **Directory layout — pass.** `image/harness/` remains the single vendored tree;
   `VENDORED.md` sits at its root where a reader looks first; the changelog is in
   `.agents/docs/changes/` with the correct `YYYY-MM-DD-<slug>.md` shape; ADR-046 is
   appended to `DECISIONS.md`; `ROADMAP.md` is at the repo root per the constitution.
   Nothing landed anywhere surprising.
5. **Simplicity / DIY-except-security — pass.** No new dependency. Guest prerequisites
   for the new harness all verified present in `image/Dockerfile:14-18`: `rsync`,
   `python3`, `jq`, `git`. `image/Dockerfile:71-73` still `COPY harness/ /styrman/harness/`,
   takes `ARG STYRMAN_VERSION`, and stamps `/styrman/VERSION` — the spec's line
   citation is accurate. No hand-rolled auth/crypto/session code anywhere in the diff;
   `claude/settings.json`'s `permissions` block is byte-identical to the previous
   version (only the two new hook events were added), so the guest permission posture
   is unchanged. The tree grew to 1.6 MB — irrelevant to image size or the mktemp copy.
6. **Decoupling — pass, with the process seam flagged.** The mirror boundary is the
   right seam: one styrman-only file, one upstream source of truth, one delivery
   mechanism (`STYRMAN_VERSION`). The `~/.agents/bin/` gate location is genuinely
   vendor-neutral and both setup scripts converge on it without either owning it.
   The weakness is procedural, not structural: the sync command is unrecorded
   (medium finding), so the seam depends on operator memory rather than on anything
   executable.

## Verification of the mirror (evidence)

- `diff -rq image/harness /home/kesokaj/workspace/devshell_prompt/harness` → `Only in image/harness: VENDORED.md`.
- Upstream `devshell_prompt` at `6132b97d25d4d02d2137d332f2de0d39232ca0fa`, working tree clean.
- `git log --oneline -- image/harness` in styrman shows exactly one prior commit
  (`878354d`) — the tree has never been hand-edited here, so ADR-046 codifies
  existing practice rather than asserting a new one.
- Roster claims check out: 28 `SKILL.md` files, 6 agents in both
  `claude/agents/` and `antigravity/plugins/harness/agents/`, `shared/AGENTS.md` /
  `CLAUDE.md` / `GEMINI.md` all 69 lines. Old tree: 23 skills, 4 agents. 136 → 150 files.
- Live re-seed confirmed on `928e709a`: stamp `1.15.0`, 6 agents with
  `testing.md.bak.20260901-065226`, `~/.agents/bin/{resume-context.sh,trail-check.sh}`
  both present and `0755`, plus their `.bak` pair. Project `36782cc5` is still stamped
  `1.13.1` on an older image ID — expected and covered by ADR-046 (stopped-container
  upgrade path, `orchestrator.go:205-224`).

## Was the existing verification sufficient?

Mostly yes, one decisive gap. The clean-container deploy test, the 1.13.1 → 1.15.0
live re-seed, the in-guest gate runs, `go build`/`vet`/`test`, integration 57/0/3 and
the `/api/version` check together establish that the sync is faithful and functional.
None of them measured **wall-clock boot latency on a home without an agy token**,
which is the one case that regresses against R1 — and the live re-seed specifically
used an already-authenticated home, which hides it. Add a cold-create timing check and
record the result in a `testing/2026-09-01-verify-*.md` log.
