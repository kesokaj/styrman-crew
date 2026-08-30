# Final /audit + full remediation → v1.14.2 (2026-08-30)

## Summary

Comprehensive post-marathon audit (5-dimension multi-agent workflow with
adversarial verification: go-remediation, web-remediation, alignment,
scripts, hygiene). Result: **0 red / 17 yellow / 17 green** — no
correctness or security regressions from the v1.14.x remediation waves;
all 17 yellows fixed in this pass. Deployed as v1.14.2.

## Fixes by area

### Go (internal/)
- **credharvest.go** — cred-push single-flight now COALESCES: a push
  arriving while one runs sets `credPushRerun[user]`; the running push
  hands off its slot and immediately re-runs, so the newest credentials
  always land (before: the concurrent import was silently dropped).
  Login-shell no-op exec now pins `HOME`/`USER` env explicitly.
- **server.go** — pre-wipe safety-backup retry no longer sleeps after the
  final failed attempt (3×10 s → 2 sleeps); gofmt sweep.
- **config.go** — `ProjectFromNetworkName` extracted (sdb- prefix +
  sys-setup guard) so orchestrator's self-attach invalidation can't
  misparse system-VM networks.
- **orchestrator.go** — self-attach memo stored only on VERIFIED attach
  and invalidated on fresh-network create/recreate; `Logs` 256 KiB trim
  cuts at a newline with an explicit truncation marker (no mid-line JSON
  shear).
- **backups.go** — meta-write failure path now checks `os.Remove(final)`
  and logs "ORPHAN ARCHIVE LEFT" if the cleanup itself fails (operator
  can reclaim disk; before: silent orphan).
- **links_api.go** — serveLink's S10 comment corrected (private links
  have no expiry; revocation is the share list).

### Web (web/src/)
- **ui.tsx** — focus grab respects focus already inside the panel (React
  autoFocus is imperative; `[autofocus]` never matches), and the trap's
  first/last lookups are narrowed for `noUncheckedIndexedAccess`.
- **projects.ts** — `pendingRefresh` re-tick when a poll lands during an
  in-flight one; `refreshProjects` un-exported.
- **api.ts** — reAuth retry cap clears on first OK response; static toast
  import; `ApiError` un-exported.
- **index.tsx** — logs dialog keeps stale data visible with a retry
  banner instead of blanking on a poll error.
- **Sidebar/TerminalList/FileExplorer** — `group-focus-within:visible`
  beside every `group-hover:visible`; keyboard target is the name span
  (role=button), fixing the axe nested-interactive violation.

### e2e harness (web/e2e/run.mjs)
- Firewall probe attributes ONLY `TimeoutError` to the host firewall —
  any other failure fails the check instead of skipping.
- Share flow split three ways (mint / serve / revoke) so revocation is
  asserted even on firewall-hardened hosts. Now exactly 36 checks +
  final client-error gate; verified live: 35 passed, 0 failed, 2
  auto-SKIP (= 34 run + 2 skip + gate).
- `execFileSync` + UUID-validated project ID (no shell interpolation).

### Scripts / deploy
- **deploy/backup/styrman-host-backup** — parameterized (`STATE_DIR`,
  self-defaulting credential exports overridable via
  `/etc/styrman/backup.env`); `GOOGLE_PROJECT` → `GOOGLE_PROJECT_ID`
  (restic's actual env var — the old name only worked because the bucket
  pre-existed). Synced to the host and verified with a real backup run.
- **deploy/host-setup.sh** — printed provisioning steps are now complete
  runnable commands (`chmod 600` on the restic password, correct
  `GOOGLE_PROJECT_ID` in the `restic init` example).

### Docs
- ADR-045 inline revision marker: agent auto-resume REVERTED to
  per-terminal opt-in in v1.14.1 (shared cwd collision).
- deploy.md: all `GOOGLE_PROJECT` → `GOOGLE_PROJECT_ID`; timer wording.
- api.md git-commit row: broker DOES deliver the git identity at login.
- guide.md + testing README dev builds carry
  `--build-arg STYRMAN_VERSION=dev-$(date +%s)`.
- e2e counts unified everywhere to "36 checks (+ client-error gate; 34
  run + 2 auto-SKIP on firewall-hardened hosts)"; stale 34/35 figures
  removed from AGENTS.md, guide.md, testing README.
- Tier 1 documented as `go test -race ./...` — verified passing first.
- Changelog backfill for tags v1.9.1–v1.9.3
  (2026-08-30-tag-backfill-v1.9.1-to-v1.9.3.md).
- deploy/README.md: `backup/` directory explained (script/unit naming,
  runbook pointer).

## Verification
- `gofmt -l` clean; `go vet ./...` clean; `go test -race ./...` all ok.
- `npx tsc --noEmit` clean; `oxlint` clean; production web build OK.
- e2e: 35 passed / 0 failed / 2 skipped (matches documented math).
- Host backup script executed for real against gs:styrman-crew-backups.
- Deployed: `{"version":"1.14.2"}` live, state adopted (2 containers),
  terminals reattached.

## Deferred / operator-gated
- Two REQUIREMENTS.md clarifying parentheticals (R9-vs-R11 scope,
  sdb-uuid vs sdb-hash naming) await operator approval — REQUIREMENTS.md
  is never modified without it.
