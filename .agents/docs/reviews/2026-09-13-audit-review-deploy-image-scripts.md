---
owner: reviewer
date: 2026-09-13
---
# Audit — deploy/ and image/ scripts (2026-09-13)

Scope: `deploy/styrman-ctl`, `deploy/update/styrman-update`,
`deploy/update/*.service`, `deploy/update/*.timer`, `deploy/firewall/*`,
`deploy/host-setup.sh`, `deploy/verify-commit.sh`, `deploy/vulncheck.sh`,
`deploy/docker-compose.yml`, `deploy/Dockerfile.server`, `image/Dockerfile`,
`image/entrypoint.sh`, `image/configs/*`, `image/skel/`. Repository
`/home/kesokaj/workspace/styrman-crew`, HEAD `e56e775` (clean). This is an
audit against recorded intent (AGENTS.md, REQUIREMENTS.md, the newest
`.agents/docs/changes/` entries, specs under `.agents/docs/specs/styrman/`,
and `.agents/docs/bugs/INDEX.md`), not a diff review.

## Verdict: APPROVE WITH NITS

Nothing broken was found that a user or attacker can hit today. This is the
most heavily self-audited part of the repo — `deploy/styrman-ctl`,
`deploy/update/styrman-update` and `deploy/firewall/*` each carry the marks
of five-plus internal security-review rounds, and the specific bug families
in `.agents/docs/bugs/INDEX.md` (BUG-002/003 symlink races, BUG-009/010/013
root-script hazards, BUG-033/041 verification-chain swallowing, BUG-035
mechanism-reaches-every-caller) are each explicitly guarded against in the
code, with the guard's rationale left as a comment. `shellcheck -S warning`
is clean on every script in scope but for one true false-positive (SC2088 on
a case-pattern, not an expansion) and one missing-shebang note on a
`profile.d` snippet that is sourced, never executed, by design. The one real
finding is a narrow inconsistency in `host-setup.sh` against a convention
the project itself established after BUG-009.

## Findings

- **[low / confidence: medium]** `deploy/host-setup.sh:372` — `install -d -m
  700 /etc/styrman` runs unconditionally, with no existence check and no
  symlink guard, unlike every other root-owned path this same file and
  `deploy/update/styrman-update` protect (`deploy/update/styrman-update:153-154`
  checks `[ -d "${STATE_DIR}" ] || install -d ...` specifically because
  `install -d` re-modes an *existing* directory — the exact mechanism named
  in BUG-009's prevention rule — and `host-setup.sh` itself refuses a
  symlink at `STATE_DIR`/`homes`/`projects` a few lines above (lines
  182-186) for the same reason). Verified empirically: `install -d -m 700
  <symlink-to-dir>` follows the link and re-chmods the *target* directory,
  not the link. On a host where `/etc/styrman` had been replaced by a
  symlink (requires prior write access to `/etc`, so this is not reachable
  by an unprivileged guest or a project VM — R12/ADR-067's isolation is
  untouched), this step would silently chmod 700 an arbitrary directory as
  root. Given the threat model (single admin, no guest ever writes to the
  host's `/etc`), impact is bounded to a scenario that already presumes host
  compromise, hence low rather than medium/high. Fix: mirror the pattern
  already used for `STATE_DIR` two sections above — `[ -L /etc/styrman ] &&
  { echo FATAL...; exit 1; }; [ -d /etc/styrman ] || install -d -m 700
  /etc/styrman`.

- **[low / confidence: low]** `image/configs/styrman-credentials.sh:59-60` —
  `shellcheck` flags SC2088 ("tilde does not expand in quotes") on the
  `"~/"*` case pattern. This is a false positive (case patterns are glob
  matches, not subject to tilde expansion, so the literal `~/` prefix is
  exactly what is intended and the code is correct); noted only so a future
  reader does not "fix" working code by chasing the warning. Not a defect —
  included for completeness since the dispatch asked for a shellcheck pass.

## Criteria

1. **Dead code** — pass. `deploy/firewall/install.sh` and
   `deploy/update/styrman-update`'s `host_converge` both carry deliberate,
   commented-out-in-spirit but *active* removal code for ADR-065/066
   leftovers (the `.path` unit, the spool mount, the tagged-rule cleanup in
   `styrman-firewall.sh:113-153`) — this is live migration logic for
   upgrading hosts, not dead code, and each is dated and scoped to expire
   naturally as hosts converge. No unused functions, unreferenced exports,
   or stale files found in the scope.
2. **Flow tracing** — pass. The `[Install]` WantedBy sections on both
   `styrman-firewall.service` and `styrman-update.timer` are propagated to
   every installer (`deploy/firewall/install.sh`, `host-setup.sh`) and to
   the self-updating path (`host_converge`'s unconditional `systemctl
   enable styrman-firewall.service` on *every* tick, not gated on
   "changed" — the exact BUG-035 fix, and it is unconditional as the rule
   requires). `styrman-ctl`'s three global flags are parsed before verb
   dispatch (the BUG-047 fix), and the `CTL_TMPD` trap uses a global, not a
   function-local (the BUG-048 fix) — both verified present and correct in
   the current file.
3. **Standards conformance** — pass. Every script has `set -euo pipefail`
   (or, where errexit is deliberately weaker — `image/entrypoint.sh`'s
   `set -uo pipefail`, `styrman-git-credential`'s `set -eu` — the choice is
   explained inline and is appropriate to a script that must degrade
   gracefully rather than abort). `shopt -s inherit_errexit` is present on
   every script that puts a side-effecting function inside `$(...)`
   (`styrman-ctl`, `firewall/install.sh`, `firewall/styrman-firewall.sh`);
   `deploy/update/styrman-update` and `deploy/host-setup.sh` lack the
   `shopt` but were checked line-by-line for a side-effecting call inside
   command substitution and none was found (the one place that could bite —
   `host-setup.sh:247`'s `git describe` inside `$(...)` — is explicitly
   guarded with `|| true` for exactly this reason, per its own comment).
4. **Directory layout** — pass. Everything sits where the project's own
   map (`AGENTS.md`'s Key Files table, `.agents/docs/specs/styrman/`) says:
   deploy tooling under `deploy/`, the runtime image tree under `image/`,
   nothing stray at the repo root.
5. **Simplicity (DIY-except-security)** — pass. No hand-rolled auth/crypto:
   git signature verification goes through `git verify-tag`/SSH signing (a
   vetted mechanism), tag integrity is content-addressed (SHA), and secrets
   flow through root-only files + git credential helpers, never invented
   crypto. `deploy/host-setup.sh` and `image/Dockerfile` verify third-party
   downloads (Kata, OpenTofu, yq, hadolint) by SHA256 against
   publisher-published sums rather than trusting TLS alone.
6. **Decoupling** — pass. `styrman-ctl` wraps `host-setup.sh`,
   `firewall/install.sh` and `docker-compose.yml` rather than
   re-implementing them (stated as a design constraint in its own header
   comment, and honored — no duplicated logic found). The firewall script
   and the updater are independent units connected only through
   `host_converge`'s narrow verified-tree install, matching the module
   boundary the specs describe.

## Not examined

- `image/harness/` (the vendored `styrman-harness` mirror) — out of the
  named scope; `verify-commit.sh` checks its integrity manifest, which was
  read but the mirror's own contents were not audited here.
- `image/skel/*` dotfiles (`bashrc`, `profile`, `bash_aliases`,
  `bash_exports`, `bash_secrets`, `npmrc`) — opened only enough to confirm
  they are plain dotfiles seeded write-once by `entrypoint.sh`; not
  line-by-line reviewed.
- `image/configs/styrman-browser` (348 lines), `image/configs/tmux.conf`,
  `image/configs/profile-motd.sh` — not read in full this round; the
  browser launcher in particular has had six independent review rounds
  logged in `.agents/docs/reviews/2026-09-13-*browser*` within the last day,
  which this audit did not re-litigate given the turn budget. A reviewer
  with more budget should at least re-check `styrman-browser` against
  BUG-042/046/050/051's exact mechanisms to confirm no regression.
- `internal/config/styrman_ctl_test.go` and the other Go test harnesses that
  exercise these scripts were not read; this audit is shell-only per the
  dispatch.
- No live host was available to run `deploy/firewall/test.sh` under
  `unshare -Urnm`, `deploy/verify-commit.sh`, or `deploy/vulncheck.sh`
  end-to-end; both were read for correctness, not executed.
