---
owner: reviewer
date: 2026-09-11
---
# Review — styrman-ctl Phase 2 (install + update --setup)   (2026-09-11)

Range reviewed: commit `c334efdf` (HEAD on `main`), restricted to
`deploy/styrman-ctl` (blob `bf80942f`) and
`internal/config/styrman_ctl_test.go` (blob `0ee17534`).

## Verdict: REQUEST CHANGES

The worst problem: `setup_auto_update`'s final `styrman-update --check`
invocation sets `STYRMAN_UPDATE_STATE_DIR` and `STYRMAN_DEPLOY_DIR` but not
`STYRMAN_GH_TOKEN_FILE`/`STYRMAN_ALLOWED_SIGNERS`, so the check runs against
`styrman-update`'s hardcoded `/etc/styrman/{gh-token,allowed_signers}`
defaults instead of the `ETC_DIR` this same function just wrote the token and
signers into. It is silently correct only because production `ETC_DIR`
happens to equal `/etc/styrman`; under the project's own ADR-052 test
convention (`STYRMAN_ETC_DIR` pointed at a scratch prefix — exactly what
`ctlSetup` in this same test file does for every other path) a real,
non-dry-run `update --setup` would have its own verification step read the
wrong, real host files. Everything else — install ordering, the compose
invocation, version resolution, the askpass coverage, clone/.env ownership,
the signers regex — is sound and matches `host-setup.sh` 8c and
`versioning.md`.

## Findings

- **[high / confidence: high]** `deploy/styrman-ctl:497` (the
  `run env "STYRMAN_UPDATE_STATE_DIR=${ETC_DIR}/update" "STYRMAN_DEPLOY_DIR=${DEPLOY_DIR}" "${BIN_DIR}/styrman-update" --check`
  line inside `setup_auto_update`) — `deploy/update/styrman-update` reads the
  token and allowed-signers paths from `STYRMAN_GH_TOKEN_FILE` (default
  `/etc/styrman/gh-token`, `styrman-update:206`) and
  `STYRMAN_ALLOWED_SIGNERS` (default `/etc/styrman/allowed_signers`,
  `styrman-update:289`) — both independent of `STYRMAN_DEPLOY_DIR`/
  `STYRMAN_UPDATE_STATE_DIR` and *not* derived from any `ETC_DIR` the caller
  controls. `setup_auto_update` installs the token and signers under
  `${ETC_DIR}/gh-token` / `${ETC_DIR}/allowed_signers`
  (`deploy/styrman-ctl:481,494`) but the `--check` call three lines later
  never sets `STYRMAN_GH_TOKEN_FILE`/`STYRMAN_ALLOWED_SIGNERS` to match.
  Concrete failure: any invocation with `STYRMAN_ETC_DIR` overridden (which
  is exactly how `internal/config/styrman_ctl_test.go`'s `ctlSetup` runs
  every test, and how ADR-052 requires any privileged test of this tool to
  run for real rather than `--dry-run`) would have `--check` look for the
  token/signers at the real `/etc/styrman/...` instead of the scratch tree
  just populated — either a spurious "git fetch failed" (no credential
  helper installed because the token file isn't found at the wrong path) or,
  worse on a host that already has `/etc/styrman/allowed_signers` from a
  prior real install, silently verifying against a *different*,
  production allowed-signers file than the one just written. Fix: add
  `STYRMAN_GH_TOKEN_FILE=${ETC_DIR}/gh-token`
  `STYRMAN_ALLOWED_SIGNERS=${ETC_DIR}/allowed_signers` to the `env` call
  (and note neither of the three Phase 2 tests would have caught this since
  all three exercise `--dry-run`, which only prints this line — the plan
  assertion for it checks only the `STYRMAN_UPDATE_STATE_DIR=` substring).

- **[low / confidence: medium]** `deploy/styrman-ctl:441-448`
  (`setup_auto_update`'s `tmpd="$(mktemp -d)"` block) — the header comment
  states the script's whole safety contract: "Every side effect goes through
  run(): --dry-run prints the exact plan and executes nothing." `mktemp -d`,
  `chmod 700`, and the two `printf … > "${tmpd}/…"` writes that put the real
  GitHub token and the real allowed-signers line on disk are *not* routed
  through `run()` and execute unconditionally, including under `--dry-run`
  — which `need_root` explicitly permits for a non-root caller. A
  `--dry-run` invocation that is supposed to change nothing on disk in fact
  writes the plaintext secret to a temp file (cleaned up via the `trap`, but
  a real write nonetheless, contradicting the documented invariant that
  every reviewer/operator relies on when reading a `--dry-run` transcript as
  proof of no side effects).

## Not examined

- `deploy/host-setup.sh` beyond the 8c/9 sections quoted here, and
  `deploy/firewall/install.sh`.
- `deploy/update/styrman-update` beyond the token/signers/`--check` paths
  cited above (its deploy/build/rollback logic was out of scope for this
  diff).
- Whether `docker compose --env-file <abs> -f <abs>up -d --build`'s build
  `context:` resolution (relative to the compose file's directory) behaves
  identically across the Compose CLI version pinned for this project — taken
  on the doc's own authority (`versioning.md`'s break-glass note) rather than
  independently verified against a running `docker compose` binary.
- `cmd_menu`/`cmd_help` text changes and the `is_loopback_addr`/`is_octet`/
  `is_port` refactor's edge cases beyond the two new bad-case tests added
  (127.0.0.1:70000, 127.0.0.999:2019) — both look correct by inspection.

## Criteria

1. Dead code — pass. No orphaned functions; `resolve_version`, `https_origin`,
   `wait_healthy`, `ask_env_key`, `setup_auto_update` are all reached from
   `cmd_install`/`cmd_update`/`main`.
2. Flow tracing — fail on the one high finding above (`--check`'s env doesn't
   reach the paths its own caller just wrote); otherwise sound — `cmd_install`
   → `host-setup.sh` → env fill → build/compose → `wait_healthy` is a clean
   chain, and error paths (`die`) are hit on every unvalidated input.
3. Standards conformance — matches `set -euo pipefail`/`inherit_errexit`
   discipline already established in this file; secrets kept out of argv and
   plan lines (verified by the test's "token leaked into the plan" check).
4. Directory layout — pass; single-file CLI extension in `deploy/`, tests
   beside the existing `internal/config/styrman_ctl_test.go`.
5. Simplicity — pass; wraps existing scripts (`host-setup.sh`,
   `docker compose`, `styrman-update`) rather than reimplementing them, per
   the file's own stated design; the one-shot `GIT_ASKPASS` helper is a
   reasonable, minimal alternative to host-setup.sh's inline credential
   helper and is stdlib (bash + git), no new dependency.
6. Decoupling — pass; `setup_auto_update` is callable from both `cmd_install`
   and `cmd_update --setup` without duplication; the low finding above is a
   coupling gap (two scripts each owning `ETC_DIR`-relative paths without a
   shared parameter) rather than a structural layering problem.
