---
owner: reviewer
date: 2026-09-11
---
# Review — styrman-ctl Phase 1   (2026-09-11)

Range reviewed: commit 092c7ef69c1d6d4da6ce1710f66e892cdd0b49e8 ("styrman-ctl, phase 1: …"),
restricted to `deploy/styrman-ctl` (blob `f2693c4675faf60503d5b7edbef7936dfa24f82d`) and
`internal/config/styrman_ctl_test.go` (blob `8d70fb312b5e42db9934bd559b80b1bd539e9a35`).
`go test ./internal/config/... -run TestStyrmanCtl -v` re-run and confirmed green (all 6 tests
PASS) as part of this review.

## Verdict: APPROVE WITH NITS

Nothing broken in what phase 1 actually ships (status, .env helpers, path guard, dry-run
harness): the six tests pass, the guard/validate/env_set logic held up under adversarial
tracing (symlinks, newline smuggling, non-key names, duplicate-key collapse), and the script
faithfully follows `host-setup.sh`/`firewall/install.sh` conventions (`run()`, `need_root`,
`set -euo pipefail`, env-overridable paths for tests). The one real gap is that `ENV_FILE`
is the single path in the script NOT env-overridable, which quietly breaks the ADR-052
scratch-prefix promise the script's own header makes.

## Findings

- **[medium / confidence: high]** `deploy/styrman-ctl:35` — `ENV_FILE="${REPO}/deploy/.env"`
  is derived from `REPO` (the script's own on-disk location via `BASH_SOURCE`), which is the
  only path among the "Paths" block (lines 23–36) that has no `STYRMAN_*_DIR` env-var
  override. Every sibling (`STATE_DIR`, `DEPLOY_DIR`/`CLONE_ENV`, `ETC_DIR`, `UNIT_DIR`, …) is
  overridable so tests can point it at a scratch prefix per ADR-052 ("every path below pointed
  at a scratch prefix" per the script's own header comment, line 17-18). `cmd_status` reads
  `ENV_FILE` first and only falls back to `CLONE_ENV` (the scratch-relocatable one) when
  `ENV_FILE` is absent (line 226). Concrete failure: `internal/config/styrman_ctl_test.go`
  invokes the real script at its real repo path (`repoRoot(t)`), so `REPO` always resolves to
  the actual dev checkout, never a scratch copy. If a contributor has a real, gitignored
  `deploy/.env` present locally (standard local-dev practice per R10/the deploy docs, and
  exactly the file the not-yet-implemented `install`/`edit` verbs this phase is scaffolding
  for will create), `go test ./internal/config/...` reads that real file's `BASE_DOMAIN`/
  `STYRMAN_HOSTNAME` instead of the scratch fixture, silently deviating from the isolation
  guarantee the test file's own comment asserts ("nothing here may touch … STYRMAN_STATE_DIR
  et al"; the guard on line 103-107 only greps output for `/var/lib/styrman`, `/etc/styrman`,
  `/opt/styrman` — a literal domain value like a real `BASE_DOMAIN` leaking through
  `Server.URL` would not be caught by that check at all). `TestStyrmanCtlStatusJSON`'s
  hard-coded assertion `st.Server.URL != "https://crew.example.test"` (line 158) would then
  fail with a confusing message rather than the intended scratch-only behavior. Fix: add a
  `STYRMAN_ENV_FILE` override (mirroring `STYRMAN_DEPLOY_DIR`) and have the test set it beside
  `STYRMAN_DEPLOY_DIR`, or drop the `ENV_FILE`/`CLONE_ENV` distinction in favor of only
  `CLONE_ENV`-style resolution rooted at an overridable directory.

- **[low / confidence: low]** `deploy/styrman-ctl:129` — `is_loopback_addr` validates
  `CADDY_ADMIN` with `^127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}:[0-9]{1,5}$`, which does not
  bound each octet to 0-255 or the port to ≤65535 (e.g. `127.999.999.999:99999` passes). No
  concrete exploit path — the value stays within the `127.x` prefix so it cannot bind a
  non-loopback address, and an invalid value would just make the Caddy container refuse to
  start (documented, non-silent failure) rather than misconfigure anything. Worth tightening
  if `edit`/`install` (phase 2/3) start trusting this validation more than "did the operator
  typo the prefix".

## Criteria

1. Dead code — pass. `validate()`/`env_set()`/`guard_path()` have no verb wiring them to an
   operator-facing command yet, but this is explicit, documented phase-1 scaffolding for the
   phase 2-4 verbs named in the script's own usage banner and `cmd_help`, exercised by the
   `__` test hooks; not orphaned.
2. Flow tracing — pass except the `ENV_FILE` finding above. `run()`/`need_root()`/`DRY`/`YES`
   thread consistently from `main()` through every helper; `cmd_status`'s `set -e`/`pipefail`
   interactions are handled deliberately (explicit `if` instead of bare `a || b` lists, `||
   true` inside command substitutions wrapping `grep -c`/`find`/`du`), and this was verified
   by re-running the tests, not just reading the script.
3. Standards conformance — pass. Matches `deploy/firewall/install.sh`'s `run()`/`die()`/
   `need_root` pattern and `set -euo pipefail; shopt -s inherit_errexit` idiom exactly;
   `python3` for JSON encoding is not a new dependency (already a hard `FATAL` requirement in
   `host-setup.sh`).
4. Directory layout — pass. Script in `deploy/`, its Go test beside the existing
   `internal/config/firewall_script_test.go` pattern (shares that file's `repoRoot` helper),
   consistent with how the firewall installer is already tested.
5. Simplicity — pass. No new dependency beyond the already-mandatory `python3`; `env_set`'s
   awk-based single-pass rewrite and `guard_path`'s `realpath -m` + allow-list are the
   simplest correct approach for the stated threat model (BUG-009-style symlink swap).
6. Decoupling — pass. Script only wraps `host-setup.sh`/`firewall/install.sh`/
   `docker-compose.yml`/`styrman-update` by shelling out; no reach-in to their internals.

## Not examined

- The three files outside the requested scope that the same commit touched (`deploy/README.md`,
  `.agents/docs/plans/2026-09-11-installer-ctl.md`, the two other `.agents/docs` additions) —
  excluded per the dispatch's restricted scope.
- `shellcheck` was not available in this environment; only `bash -n` syntax-checked the script.
- Live/root execution of `status` against a real host (only the scratch-prefixed fake-tool
  harness was exercised, via `go test`).
