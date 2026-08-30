# Marathon-diff code review + full remediation (v1.14.1)

Date: 2026-08-30. Operator asked whether a fresh review would be green; the
honest answer was "probably not" — and it wasn't. Adversarial review of the
v1.13.2→v1.14.0 diff (5 dimensions, every finding re-verified by a refuter):
5 blocking + 9 important (1 refuted, 2 downgraded) + 12 nits. ALL confirmed
findings fixed in this release; live-verified where it matters.

## Blocking (all fixed)

1. **Every private share preview 502'd** — serveShared fed the PRIVATE
   link's zero-valued Expires to context.WithDeadline (private links never
   expire; revocation is the share list), yielding an instantly-expired
   context. Now bounded only when a real expiry exists. Live-proven: a
   private share serves 200 + content again.
2. **File editor unusable / 3. Escape deadlock** — useModalA11y keyed its
   whole effect on [onClose] (a fresh closure per render): every keystroke
   re-ran it, yanking focus to the first control, and the re-push lifted a
   background modal above a nested ConfirmDialog (topmost-Escape wedged).
   Now mount-once with an onClose ref; [autofocus] children win initial
   focus; Tab inside .xterm is left to xterm.
4. **R11: the host backup layer lived only on the host** — the restic
   script + systemd units were installed live but never committed; a
   reinstall would silently lose disaster recovery. Now in deploy/backup/
   (env-overridable bucket/project), installed + conditionally enabled by
   host-setup.sh; host synced to the repo copies and re-verified.
5. **Transcript janitor followed guest symlinks** — the per-project home is
   guest-writable; ~/.styrman symlinked at another tenant's tree (or the
   host) would have the root janitor deleting THROUGH it. Now os.Root-
   confined (fs.WalkDir over Root.FS + Root.Remove) like history.go.

## Important (all fixed)

- selfAttached memo stored on FAILURE paths (60s of unretried preview 502s
  after a transient NetworkConnect error) → memoize only verified
  attachment; invalidate on network (re)creation.
- seedFromBackup invisible to the guard machinery → now holds restoring
  (new id) + backingUp (source) so cadence can't archive a half-seeded
  tree and retention can't prune the source archive mid-read.
- Boot-time cadence pass + keep-N rotated away history across restart
  bursts → freshness gate: skip projects whose newest archive is younger
  than the cadence interval.
- Location rewrite doubled the token prefix on preview-host requests →
  external prefix threaded separately from the internal strip prefix.
- Logs endpoint kept the FIRST 256KiB (dropping the newest lines) → keeps
  the last, line-aligned, with a truncation marker; error branching (404
  no-container / 502 other) replaces the blanket 409.
- gitSummary ran N serial 8s execs, no overall budget → bounded fan-out
  (4-way) under one 10s deadline.
- Caddy env allowlist had dropped 5 knobs the ingress reads (AUTH_DOMAIN,
  SHELL_DOMAIN, APPS_DOMAIN, DEFAULT_RPS, CADDY_ADMIN) → restored as
  optional pass-throughs.
- Integration firewall probe attributed EVERY failure to the firewall →
  only curl exit 28 (timeout) counts; empty VMIP / refused = loud FAIL.
- e2e interpolated a server-supplied id into a shell string → execFileSync
  + strict UUID validation.
- **Auto agent-resume REVERTED to opt-in** — all terminals of a project
  share one cwd, so the automatic `--continue` rewrite made several
  terminals resume the SAME conversation after a VM restart. Users set
  `claude --continue` / `agy --continue` as the autostart themselves
  (verified: both CLIs start fresh, rc=0, when nothing to resume). Guide +
  tests updated; revises the v1.14.0 change.
- reAuth reload capped via sessionStorage (2 attempts/min, then a manual-
  action message) — downgraded finding, fixed anyway.
- Nested-interactive rows (sidebar project, terminal list, explorer
  entries): keyboard semantics moved to the NAME span; containers are
  plain clickable divs again (axe-clean).

## Nits fixed

Meta-write failure now fails the backup (no invisible orphan archives);
wipe-delete retries briefly on a backup-scheduler collision before
aborting; credential push is single-flighted per user + pins HOME/USER;
LogsDialog surfaces poll errors; shared projects store catches up on
visibilitychange; ConfirmDialog's autofocus honored; stale comments;
e2e logs the private-link sub-skip. Refuted (no change): the focus trap
does not steal Tab from xterm (xterm cancels its own keydowns) — a
defensive .xterm bail was added regardless.

## Verification

All 14 Go suites green (autostart tests updated to the opt-in contract);
tsc/oxlint/web build clean; v1.14.1 deployed (server + caddy); private
share proven live (200 + body, was 502); integration + e2e final results
appended below.

### Final results
Integration **57 / 0 / 3-skipped** (one more harness fix en route: the new
probe's bare curl aborted the suite under set -e — now `|| RC=$?`);
e2e **34 / 0 / 2-skipped** (both firewall skips now logged). Go 14/14,
tsc/oxlint clean. v1.14.1 live; private share serving verified (200+body).
