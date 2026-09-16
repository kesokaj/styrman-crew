---
owner: security
date: 2026-09-16
---
# Security review — harness-refresh-4aa19be   (2026-09-16)

Range reviewed: working tree, image/harness (a81ef4b..4aa19be upstream)

Repository HEAD at review time: `4957dfe`. The mirror's own attestation is sound
(see Checked). The working tree MOVED during this review: when it began only
`image/harness/**` was modified; partway through, `REQUIREMENTS.md` and
`image/entrypoint.sh` appeared as modified too, both carrying the new exception
text. Findings below are against the tree as of the later state; the two
concurrent edits are Finding 1's subject.

## Verdict: FINDINGS

Nothing in the refreshed Go code is unsafe on its own — the settings write is
bounded, content-preserving, symlink-refusing, atomic, and cannot fail a boot
(twelve probe shapes run against scratch homes confirm it). The finding is
what the write means here: styrman's `REQUIREMENTS.md` R5 said "no hooks,
scripts, or settings files" and `image/entrypoint.sh` said "NO settings.json
for either tool"; this refresh makes `sh-agent install` create or edit
`/home/<project-user>/.claude/settings.json` in EVERY project home on the first
boot after an image version change. Both documents have been rewritten in the
same uncommitted working tree to permit it, citing **ADR-072 — which does not
exist** in `.agents/docs/architecture/DECISIONS.md`. A non-negotiable that is
immutable without the operator's approval has been relaxed by the same change
that needs the relaxation, and the only record of the approval is the sentence
that asserts it.

## Findings

- **[medium / confidence: high]** `REQUIREMENTS.md:48-52`, `image/entrypoint.sh:113-121`,
  `image/harness/cmd/sh-agent/main.go:477` (`attributionSet`) — **the refresh
  reverses a recorded operator directive, and the ADR that is supposed to carry
  the reversal is not on disk.** R5 previously read "no hooks, scripts, or
  settings files — ADR-062, operator approval 2026-09-04" and the entrypoint
  comment "NO settings.json for either tool (operator directive 2026-09-03)".
  Both are now edited (uncommitted) to allow one key and both cite "the
  exception approved 2026-09-16 (ADR-072)"; `grep -rn ADR-072` outside the
  mirror matches only those two new sentences — `DECISIONS.md` has no ADR-072.
  The harness's own R4 exception (upstream 42ba2f5) is the harness's document,
  not styrman's, and cannot carry styrman's requirement. What the write does to
  a project home, concretely: `image/entrypoint.sh:163` runs
  `sh-agent install --repo /styrman/harness --home /home/<user>` under
  `runuser -u <user>` on every image-version change; `attributionSet` then
  creates `/home/<user>/.claude/settings.json` with
  `{"attribution": {"commit": "", "pr": ""}}` when it is absent, or textually
  inserts that key into the user's existing file when the key is absent. That
  file is Claude Code's user-scope settings — the same file that carries
  `permissions.allow/deny`, `hooks`, `env` and `apiKeyHelper` — so an
  image-shipped binary now edits the guest's security-relevant config on every
  bump, which is the surface the 2026-09-03/09-04 directives closed.
  Remediation: write ADR-072 (decision, the exact scope — one key, one path,
  absent-only, never on re-seed) and land it in the same commit as the R5 and
  entrypoint edits, with the operator's approval quoted; do not commit the R5
  edit without it. If the approval is not in hand, the alternative that keeps
  R5 intact is to leave the key to the operator and treat the install-time
  write as out of scope for the image.

- **[low / confidence: high]** `image/entrypoint.sh:163` — **every settings
  advisory is discarded, so a refused or failed write is invisible.** install's
  stdout goes to `>/dev/null`, and `attributionSet` is advisory by design: it
  never returns an error and never changes install's exit status. So
  `settings: … created with …`, `… is a symlink; add the key to its target by
  hand`, `… could not be written (…)`, `… is larger than 1 MiB …` and
  `settings: temp file … left behind` all vanish, the boot is graded pass by
  the "agent harness deployed" line, and the stamp is written. A home whose
  Claude settings were silently modified and one where the modification was
  refused look identical in the ops panel's log. The MCP block three lines
  below already has the right shape (`2>&1 >/dev/null` captured into `HR_OUT`,
  one `log` line, control-stripped). Remediation: capture install's stdout the
  same way and `log` the lines matching `^settings:` (they contain no file
  content by construction — `readSettings`/`attributionOff` return fixed
  problem strings — but pass them through the same `tr -cd '[:print:]'` cut the
  MCP line uses, since the path they print comes from `${HOME_DIR}`).

- **[low / confidence: high]** `image/harness/cmd/sh-agent/main.go` (the
  `os.CreateTemp` + `os.Rename` tail of `attributionSet`) — **a settings.json
  the user deliberately made read-only is replaced anyway.** Verified in a
  scratch home: mode `444`, key absent → the key was inserted and the mode
  restored to `444`. Rename needs only write permission on `~/.claude`, so
  `chmod 444` on a frozen `permissions.deny` list buys nothing against this
  path. Content was preserved in every shape probed, so this is about the
  expectation the mode sets, not about loss. Remediation: skip the edit when
  the file is not writable by the running user and print the same advisory, or
  say in the ops/deploy doc that the mode is not honoured.

- **[low / confidence: medium]** `image/harness/cmd/sh-agent/main.go`
  (`readSettings` → build → `CreateTemp` → `Rename`) — **the insert is a
  read-modify-write with no lock: a concurrent writer's change in that window
  is silently reverted.** In today's wiring there is no concurrent writer: each
  project home is bind-mounted into exactly one container
  (`internal/orchestrator/orchestrator.go:1048`, `spec.homePath + ":/home/" + spec.guestUser`),
  the entrypoint runs install before any user process in that container, and
  styrman's server never writes `~/.claude/settings.json` (it only tests
  `.claude/.credentials.json` for existence and lists `.claude/{skills,agents,commands}`).
  But Claude Code itself writes this file (user-scope `/permissions`), so the
  window is real the moment install is run by hand in a live session or moved
  off the boot path. Confidence medium because no concurrent writer exists in
  the current wiring. Remediation: none needed while install stays on the boot
  path; if it ever moves, take an `O_EXCL` lock file next to the target.

- **[low / confidence: high]** `image/harness/cmd/sh-agent/main.go`
  (`attributionSet`'s `Lstat` guard) — **only a symlinked settings.json is
  refused; a symlinked `.claude` DIRECTORY is followed.** Verified: with
  `~/.claude` → a directory outside the home, install created `settings.json`
  in the link target. No boundary is crossed (the write runs as the same uid
  that owns the home, and install already writes its whole tree through that
  link), so this is informational — recorded because the function's own comment
  reads as a blanket "never written through", which holds for the file only.

- **[low / confidence: high]** `image/configs/harness-migrate.sh:52` + the new
  create path — **on the one-time migration the user's old settings.json is
  moved to the backup and a fresh one appears in its place.** Migrate's
  predicate (`~/.styrman/.harness-version` present AND
  `~/.config/sh-agent/state.json` absent) is true exactly once; it moves
  `.claude/settings.json` into `~/.styrman/harness-backup-<ts>/` (never
  deletes), and install then creates a new file holding only the attribution
  key. On every later re-seed the manifest exists, migrate exits early
  ("already migrated"), and the file is not moved again — so there is no loop
  and no repeated loss. The migrate block's own output IS kept in the boot log
  (only install's is discarded), so the move is recorded; the residual is that
  a user who later looks at the home sees a settings.json and has no signal
  that their previous one — with any `permissions` in it — is in the backup.
  Remediation: none required; if the ops doc describes the migration, say that
  a settings.json seen after a migration is a new file, not the old one.

- **[informational / confidence: high]** `image/harness/cmd/sh-agent/main.go`
  (`attributionHint` after the >1 MiB refusal) — a settings.json larger than
  1 MiB is correctly refused untouched, but the advisory that follows reads
  `… is not valid JSON`, because `readSettings` truncates at 1 MiB and the
  truncated prefix does not parse. Misleading, not unsafe; no byte of the file
  is echoed either way.

### What the insert does NOT do (verified, not assumed)

Built the mirror's binary (`go build ./cmd/sh-agent`) into
`.agents/scratch/security-harness-refresh/` and ran `install` against scratch
homes there. Twelve shapes: `{}`, a pretty-printed file with
`permissions.deny/allow` + `env`, a compact one-line file, a CRLF file, a
tab-indented file, a file with no trailing newline, a value containing `}`
inside a string, a file that already has `attribution`, malformed JSON, a
top-level array, bare `null`, and no file at all. Result in every case: other
keys preserved byte for byte, the key appended before the closing brace, mode
preserved, `json.Valid` re-check before the rename, no `.settings-*.json` temp
left behind. A present `attribution` of any value was left alone. Malformed,
non-object and oversize files were refused untouched with one advisory line and
no content echoed. A symlinked settings.json was refused and its target
(`{"secret":"do-not-touch"}`) was unchanged, link intact. A FIFO at the path
returned rc 0 without blocking. `--dry-run` wrote nothing. An unwritable
`~/.claude` left the file unchanged (install itself then exits 1 for its own
reasons, which withholds the stamp and retries next boot — pre-existing
behaviour). So: **a user-owned settings.json carrying `permissions` is not
damaged by this code.**

## Checked

- **Secrets** — no credential, token or key shape in the added lines
  (`git diff -- image/harness | grep '^+'` against api[_-]key/secret/token/
  password/PRIVATE KEY/AKIA/ghp_/sk- matches only prose about hand-rolled
  crypto). `attributionSet`, `readSettings`, `attributionHint` and
  `attributionOff` echo a path, a constant fragment and fixed problem strings —
  never a byte of the file's content, by construction and confirmed on a
  malformed and a non-object probe. The file the installer creates is `0644`
  and holds one constant key; the credential broker's
  `~/.claude/.credentials.json` (`internal/httpapi/credharvest.go:66`) is a
  different file and is untouched by this path.
- **Injection** — the settings path is fixed
  (`filepath.Join(home, ".claude", "settings.json")`, `ClaudeRoot = ".claude"`
  in `plan.go:15`), `home` comes from `${HOME_DIR}` in the entrypoint, never
  from user data; the temp name comes from `os.CreateTemp`. Nothing from the
  file reaches a shell or the boot log. `harness-migrate.sh` still strips
  control characters from every name it logs (`plain()`), and install's output —
  the only channel this change adds — is discarded. `adopt.go`'s new `Abs` +
  `validName` + `within` checks close the relative-path deletion primitive
  (BUG-037) and `inventory.go`'s `checkManagedTree` refuses symlinks at every
  managed directory, with `LoadSkills` now returning the refusal instead of
  swallowing it (BUG-036); both are hardening, not risk.
- **Authz/authn** — no authenticated surface in the diff. install runs as the
  project user (`runuser -u "${USERNAME}" -- env HOME=… --home "${HOME_DIR}"`,
  `image/entrypoint.sh:161-163`), never as root; `image/Dockerfile` only builds
  and copies the binary, it never runs install. No host-side styrman code calls
  `attributionSet`, so there is no confused-deputy path from the guest to the
  host: a symlink the guest plants can only redirect a write the guest's own
  uid could already perform.
- **Crypto** — nothing hand-rolled anywhere in the range. No primitive,
  protocol, token or session format is implemented or composed; the only
  `crypto` in the diff is prose narrowing the harness's own rule (stdlib
  `crypto/*` used as designed is not hand-rolling), which tightens rather than
  weakens the ban.
- **Dependencies** — `image/harness/go.mod` is `module …/styrman-harness; go 1.24.4`
  with no `require` block; no import outside the standard library in `cmd/` or
  `internal/`. `bash deploy/vulncheck.sh` ran (pinned
  `golang:1.25.13-trixie@sha256:6be3daa…`, exit 3): **image/harness — "No
  vulnerabilities found"**; the server and ingress modules report the two
  already-accepted docker/docker findings **GO-2026-4887** and **GO-2026-4883**
  (no fix available, accepted per the script header), unchanged by this
  refresh. Log kept at `.agents/scratch/security-harness-refresh/vulncheck.log`.
- **Attestation** — `image/harness/COMMIT` = `4aa19be7eeb5b15e4b876832bd81807cd5d91b37`,
  equal to `git -C ~/workspace/styrman-harness rev-parse HEAD` with a clean
  upstream tree; `sha256sum -c SHA256SUMS` in the mirror: 155/155 OK, and the
  file list matches the tree exactly (155 files on disk, 155 lines);
  `diff -rq --exclude .git --exclude .agents --exclude .gitignore` against the
  upstream checkout at that SHA prints exactly the three expected mirror-only
  lines (COMMIT, SHA256SUMS, VENDORED.md); `find image/harness -type l` → 0
  symlinks (what `deploy/verify-commit.sh:111-129` re-checks at release time).
  New tests use `t.TempDir()` throughout — no probe file lands in the tree
  (upstream BUG-044's failure mode is not reproduced here).
- **GCP surface** — none in range; this change is container-local.

## Not examined

- **The other prose in the range beyond a gate scan.** I read the full diffs of
  `context/AGENTS.md` and `agents/security.md`, and scanned only the changed
  lines of `skills/{preflight,review,review-standards,feature,golang,handoff,init-project}`,
  `skills/init-project/assets/agents-tree/docs/deploy/local.md`, and
  `agents/{verifier,reviewer,documentation,changelog,explorer}.md`. Every change
  I read tightens a gate (shadow-agent detection before dispatch, preflight
  blocking on a missing security report, the verifier's `-count=1` and
  unconditional guide duty, `Not examined`/`Not verified` no longer satisfiable
  with a bare "nothing"); I did not read the unchanged remainder of those files,
  nor `README.md`/`ROADMAP.md`/`REQUIREMENTS.md` inside the mirror.
- **`image/harness/cmd/sh-agent/main_test.go` and the three other test files** —
  I scanned their added lines for filesystem writes only, not for what they
  assert; the mirror's tests are not run by `deploy/verify-commit.sh` by design.
- **A real container boot.** Every behavioural claim about `attributionSet`
  above was produced on this Linux host with a binary built from the mirror and
  run against scratch homes under
  `.agents/scratch/security-harness-refresh/homes/`, NOT inside a styrman
  project container and not through `image/entrypoint.sh`. The entrypoint's own
  behaviour (the `>/dev/null`, the 20 s timeout, the stamp) is read from the
  script, not observed. A temp file left behind by `timeout 20` killing install
  between `CreateTemp` and `Rename` is reasoned from the code, never reproduced.
- **`deploy/verify-commit.sh` end to end** — I read its harness section
  (lines 111-129, 212-235) and re-ran its integrity checks by hand; I did not
  run the script, so the runtime-image build, the `sh-agent list` count
  comparison and the plain-copy assertion are unverified in this session.
- **Concurrency under a live agent session** — I found no concurrent writer to
  `~/.claude/settings.json` by reading `orchestrator.go`, `httpapi/setup.go`,
  `httpapi/credentials.go` and `httpapi/credharvest.go`; I did not enumerate
  every write into a project home.
- **Whether the operator approved styrman's own R5 exception.** I can see only
  that the sentence claiming it and the ADR number it cites have no matching
  record in `.agents/docs/architecture/DECISIONS.md`. No agent message is
  evidence of that approval.

## Outcome

- Finding 1 (medium) — resolved: ADR-072 is written by the changelog agent in the same commit that carries the R5 edit, on the operator's word (2026-09-16, "Amend crew's requirement: allow that one key (Recommended)"); R5 cites it.
- Finding 2 (low) — fixed: `image/entrypoint.sh` keeps the installer's stdout and logs every `^settings:` line (printable characters, 200 columns, the MCP block's shape); stderr passes through as before; `bash -n` clean.
- Finding 3 (low) — filed upstream: a settings.json the user made read-only is still replaced by the rename; the harness plan `2026-09-16-attribution-write.md` carries the Follow-up (refuse a file its owner cannot write).
- The verified-good list (twelve probe shapes, a symlink refused, no concurrent writer, attestation and dependencies clean) stands as the record for the image path; a real container boot is the release's Tier 2 (`/preflight`).
- Round 1 closed 2026-09-16.
