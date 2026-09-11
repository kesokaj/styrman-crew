---
owner: reviewer
date: 2026-09-11
---
# Review — styrman-ctl phase 3 (edit, update, security round 1)   (2026-09-11)

Range reviewed: commit `edc4858`, restricted to `deploy/styrman-ctl` (blob `bf4b1f6`) and
`internal/config/styrman_ctl_test.go` (blob `df3341c`).

## Verdict: APPROVE WITH NITS

Nothing in the diff is a security hole or a validated-input bug — the round-1 fixes (literal
awk key/value, backslash refusal, gated `__` hooks, canonical `guard_path`, `printable`/`json_str`)
all do what their comments claim, and the two phase-2 findings this commit set out to close
(unnamed updater inputs, secrets written to disk under `--dry-run`) are genuinely closed. The
one real bug is in `cmd_update`: under `set -e`, a failing `systemctl start
styrman-update.service` or `styrman-update --force` skips the `journalctl`/`cmd_status` lines
that exist specifically to show the operator what happened — so the one moment an operator most
needs that output (a failed tick) is exactly when the script won't produce it.

## Findings

- **[medium / confidence: high]** `deploy/styrman-ctl:552-560` (`cmd_update`) — the trailing
  `run journalctl -u styrman-update.service -n 20 --no-pager` and `cmd_status || true` are meant
  to run unconditionally after either branch (plain tick or `--redeploy`), but the preceding
  `run systemctl start styrman-update.service` (line 557) and `run env "${UPDATER_ENV[@]}"
  "${BIN_DIR}/styrman-update" --force` (line 554) are bare statements under `set -euo pipefail`
  with no `||`/`if` around them. `systemctl start` on a `Type=oneshot` unit blocks for the job
  and returns non-zero when the job's result is `failed`; a failed updater tick therefore makes
  `run` return non-zero, `errexit` fires, and the script exits **before** printing the log tail
  or `cmd_status`. Failure scenario: an operator runs `styrman-ctl update` after a broken tag was
  pushed; the tick fails, the operator gets only "styrman-ctl: ERROR" noise or nothing (whatever
  the failing command wrote to stderr) and no journal tail, no status — exactly the two things
  the code was written to always show. Fix: `run systemctl start styrman-update.service || true`
  (or capture the exit code and report it) so the two diagnostic lines always run, then `exit`
  with the saved code if it was non-zero. No test exercises this path — the fake `systemctl` in
  `styrman_ctl_test.go` never returns non-zero for `start`.

- **[low / confidence: high]** `deploy/styrman-ctl:98-106` (`env_get`) — the awk program's
  `END` block is `if (v != "") print v; else if (found) print ""`, but `found` is never assigned
  anywhere in the program (dead variable, always empty/false in awk). The `else if` branch is
  unreachable. It happens to be harmless today (an absent key and a key present with an empty
  value both fall through to printing nothing, which matches the old `sed | tail -1` behavior
  through command substitution), but it is dead code masquerading as a present/absent
  distinction that doesn't exist. Delete the dead branch or wire `found=1` in the pattern action
  if the distinction is actually wanted.

- **[low / confidence: medium]** `deploy/styrman-ctl:619-624` (`cmd_edit`, the per-key /
  per-target write loop) — validation happens for every change before any write (good), but the
  write loop itself (`for kv in changes: for t in targets: env_set`) has no atomicity across the
  two `.env` copies. If `env_set` fails on the second target (e.g. `${CLONE_ENV}` became a
  symlink, its directory went read-only, or disk is full) after succeeding on the first, the
  checkout's `.env` and the clone's `.env` are left with different values for the same key —
  precisely the drift `cmd_status`'s `envs_differ` check exists to detect, but it's now possible
  to *create* that drift via `edit` itself, mid-run, with no rollback. `cmd_status` will surface
  the mismatch on the next run, so this is not silent, but it is a real partial-write outcome
  worth a comment or a check-both-writable-before-any-write guard, given the whole point of this
  change was "both .env copies, validated before any write."

- **[low / confidence: high]** `internal/config/styrman_ctl_test.go:566-575`
  (`TestStyrmanCtlUpdatePlan`'s last case) — the comment says it exercises the "timer not
  enabled → --setup hint" path, but `cmd_update`'s `need_root` (line 545) runs *before* the
  `systemctl is-enabled` check (line 550) and the test calls `ctl(t, e, "update")` without
  `--dry-run` and without root, so `need_root` dies first with "run as root", never reaching the
  `is-enabled` check. The new `die "auto-update is not set up on this host…"` branch this commit
  added has zero coverage.

## Previously reported, still open (out of scope for this commit, unaddressed)

- `2026-09-11-review-styrman-ctl-phase1.md`: `ENV_FILE` derived from `REPO`/`BASH_SOURCE` with no
  `STYRMAN_ENV_FILE`-shaped guard against being the updater's own clone (medium) — not touched
  here.
- Same file: `is_loopback_addr`'s regex doesn't bound octets to 0-255 or the port to ≤65535 (low)
  — not touched here.
Both are pre-existing and unrelated to `cmd_edit`/`cmd_update`; flagging only so they don't fall
off the trail.

## Closed by this commit (verified)

- Phase 2 high: the updater's `--check` invocation now names all four inputs via `UPDATER_ENV`
  (`deploy/styrman-ctl:460,527,550,554`), including `STYRMAN_GH_TOKEN_FILE`/`STYRMAN_ALLOWED_SIGNERS`,
  and the plain-`update` and `--redeploy` paths reuse the same array.
- Phase 2 low: `setup_auto_update`'s `--dry-run` branch (`deploy/styrman-ctl:483-498`) no longer
  writes the token/askpass/signers to a real temp dir; it substitutes the literal `<tmpdir>`
  placeholder and skips `mktemp -d`/the `trap`.

## Focused checks requested

- `env_get`/`env_set`: key travels as an awk variable (`-v k=`), matched via `index($0, k "=") ==
  1` — never interpolated into a program. Value travels through `ENVIRON["V"]` (not `-v`), so awk
  never escape-decodes it; a literal backslash is refused before that point anyway
  (`deploy/styrman-ctl:117`). Verified against the added regression test (backslash-n case,
  `styrman_ctl_test.go:252-259`).
- `guard_path`'s canonical-and-existing rule (`deploy/styrman-ctl:210-224`): correctly rejects a
  non-canonical literal path (symlink component or `..`) by comparing `realpath -m` output to the
  literal argument, and now requires the target to exist as a directory. Not yet called by any
  caller other than the `__guard-path` test hook (uninstall is phase 4), so this is forward
  infrastructure, not dead code.
- Gated `__` hooks (`deploy/styrman-ctl:643-644`): `cmd_internal` refuses unless
  `STYRMAN_CTL_TEST=1`; verified against the new "hooks usable without the harness variable" test.
- `printable`/`json_str`: `printable` strips C0 controls + DEL before every value that plausibly
  carries attacker-influenced bytes into JSON/terminal output (hostname, container fields,
  deployed-version, git sha/tag). The Python-less fallback in `json_str` escapes `\` and `"` after
  `printable`, which is sufficient for valid JSON given control bytes are already gone.
  `deployed_version`'s use in `cmd_edit` (line 631) is not `printable`-wrapped, but it's
  immediately validated against `^[0-9]+\.[0-9]+\.[0-9]+$` before being placed in
  `STYRMAN_VERSION=${ver}`, which is a stricter whitelist than `printable` would give — no gap.
- `cmd_edit`'s re-apply: `deployed_version` is trustworthy for this purpose (root-owned updater
  state file, or the actually-running container's image tag) and is regex-validated before use;
  `up -d` with no `--build` is correct per `versioning.md`'s documented rollback recipe (same
  version, env-only recreate, no rebuild).
- `cmd_update`'s timer precondition (`is-enabled` must literally equal `enabled`, or `--dry-run`)
  is correctly placed after `need_root`/`--setup` dispatch; the `--redeploy` bypass of `systemctl`
  in favor of running `styrman-update --force` directly is deliberate (systemd can't be told to
  add a flag to a fixed `ExecStart` via `systemctl start`) and still goes through the updater's own
  unconditional `verify_tag` — see the `journalctl`/`cmd_status`-skipped-on-failure finding above,
  which is the one real defect in this area.

## Not examined

- `deploy/host-setup.sh`, `deploy/firewall/*`, and the vendored Caddy ingress — out of the stated
  diff scope.
- Manual/live execution against a real host or real `systemctl`/`docker`/`git` — this review is
  static, reading the script and the Go-driven fake-tool test harness only.
- Whether `docker compose up -d` (no `--build`) would silently trigger an implicit rebuild if the
  deployed version's image had been pruned from the local Docker image store between deploy and
  `edit` — plausible per Compose's "build only if image missing" behavior, but outside this
  commit's changes and not exercised by any test either before or after.
