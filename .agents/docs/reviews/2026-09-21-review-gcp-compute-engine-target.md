---
owner: reviewer
date: 2026-09-21
---
# Review — GCP Compute Engine target (deploy/gcp/*, host-setup.sh, verify-commit.sh)   (2026-09-21)

Range reviewed: working tree — new `deploy/gcp/vm.conf`, `deploy/gcp/lib.sh`,
`deploy/gcp/vm.sh`, `deploy/gcp/install.sh`, `internal/config/gcp_scripts_test.go`;
modified `deploy/host-setup.sh` and `deploy/verify-commit.sh` (`git diff` on
those two). Neighbouring code imitated: `deploy/styrman-ctl` (`run()`,
`private_file`, `setup_auto_update`, `cmd_install`),
`internal/config/styrman_ctl_test.go`. `gcloud` was never invoked; `go test`,
`bash -n` and `shellcheck` were run directly (read-only) to confirm claims.

## Status: COMPLETE

## Verdict: REQUEST CHANGES

The scripts are well built and the injection/secrets discipline is careful
and consistent with `styrman-ctl`, but the one line that is the whole point
of `install.sh` — handing `host-setup.sh` a working `GH_TOKEN` so it can
build the runtime image on a bare VM — is broken: the variable it reads was
never set in that shell. Every unattended `install.sh` run will die at
`host-setup.sh`'s own `FATAL: no GitHub token for the styrman-harness
download`, after ~10-20 minutes of Docker/Kata/firewall provisioning
already ran. No test in `gcp_scripts_test.go` reaches this, because every
test drives `--dry-run` and the remote command is never actually executed.

## Findings

- **[high / confidence: high]** `deploy/gcp/install.sh:153` —
  ```
  sudo env GH_TOKEN="$(cat "${STYRMAN_GH_TOKEN_FILE}")" ./deploy/styrman-ctl install --yes …
  ```
  `STYRMAN_GH_TOKEN_FILE` is only ever set as a same-command `env` prefix on
  the earlier, *separate* `vm_ssh` calls that do the git clone/fetch
  (`GIT_ENV` at line 139, used at 143/146). Each `vm_ssh` invocation is its
  own `gcloud compute ssh --command=…` exec — nothing persists an
  environment variable from one call into the next. In the remote shell
  that runs line 153, `${STYRMAN_GH_TOKEN_FILE}` is unset, so
  `cat "${STYRMAN_GH_TOKEN_FILE}"` becomes `cat ""` (confirmed directly:
  `bash -c 'unset STYRMAN_GH_TOKEN_FILE; GH_TOKEN="$(cat "${STYRMAN_GH_TOKEN_FILE}")"; echo "[${GH_TOKEN}]"'`
  prints `cat: '': No such file or directory` and `GH_TOKEN=[]`), and
  `GH_TOKEN` is always the empty string. Per the comment at 150-151 and
  `deploy/host-setup.sh:255-289`, `GH_TOKEN` is exactly what host-setup.sh's
  step 7 needs to build the runtime image on a **first** install (no
  `/etc/styrman/gh-token` exists yet, and `cmd_install`'s own
  `--gh-token-file`-driven `docker build` at `styrman-ctl:478` runs *after*
  host-setup.sh, so it can't cover for it). Concrete failure: any real run
  of `deploy/gcp/install.sh --version X.Y.Z` against a fresh host reaches
  `sudo styrman-ctl install --yes` and dies at host-setup.sh step 7 with
  `FATAL: no GitHub token for the styrman-harness download`, after Docker,
  Kata, KSM and the firewall have already been provisioned. Fix: reference
  the staged path directly, e.g.
  `GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")"`, exactly the literal
  path already used two flags later for `--gh-token-file`.
  **Test gap:** no test in `gcp_scripts_test.go` would catch this — every
  test there drives `--dry-run`, and `run()` (lib.sh:10) only echoes the
  `--command=…` argv string under `DRY=1`; the remote shell that would
  expose the unset variable is never executed. `TestGCPInstallPlan` asserts
  the substring `"styrman-ctl install --yes"` is present in the plan, which
  is true whether or not `${STYRMAN_GH_TOKEN_FILE}` resolves to anything —
  a test that would pass whether or not the code under it works.

## Criteria

1. **Dead code** — pass. The old `.tar.xz`/`.sha256sum` fetch in
   `host-setup.sh` is fully replaced, not left commented out; no stray docs
   elsewhere still reference the old asset extension (`grep -rl
   kata-static *.md` hits only the new bug record, plan and verify log).
2. **Flow tracing** — fail on the GH_TOKEN finding above; otherwise sound.
   The clone/fetch credential-helper plumbing (`GIT_ENV`/`GIT_C`/`sq()`)
   correctly threads `STYRMAN_GH_TOKEN_FILE` through its *own* `env`
   prefix and was verified against `styrman-ctl:571/588-591`'s identical,
   already-reviewed pattern — confirmed the staged-path never leaks into
   the helper snippet itself (`TestGCPInstallCloneHelperShape` covers
   this correctly). `vm.sh`'s exit-code contract (0/1/2/3) is honored by
   both `cmd_status` and `install.sh`'s `case "${rc}"` dispatch, and by
   `TestGCPVMStatusExitCodes`. The install trap/cleanup only removes staged
   secrets when `STAGED=1` was actually reached, so a failure before
   staging leaves nothing to clean up and a failure after staging always
   cleans up (`set -euo pipefail` + `trap … EXIT INT TERM HUP`).
3. **Standards conformance** — `bash -n` and `shellcheck -S warning` on all
   three `deploy/gcp/*.sh` files pass clean (verified directly, not just
   trusted from the `verify-commit.sh` diff); the new coverage in
   `deploy/verify-commit.sh` (`bash -n … deploy/gcp/*.sh`, `shellcheck -S
   warning … deploy/gcp/*.sh`) correctly picks up `lib.sh` too (it globs
   `*.sh`, and `lib.sh` carries the `# shellcheck shell=bash` directive
   needed since it's sourced, not executed).
4. **Directory layout** — pass. `deploy/gcp/` sits beside `deploy/styrman-ctl`,
   `deploy/firewall/`, `deploy/update/` as the project's existing layout
   would predict; the Go test lives in `internal/config/` next to
   `styrman_ctl_test.go` and reuses its `repoRoot(t)` helper rather than
   redefining it.
5. **Simplicity** — pass. No new dependency: `python3` for the GitHub API
   JSON parse in `host-setup.sh` was already a hard requirement (daemon.json
   merging, line 37); using it for JSON rather than regex-scraping the
   GitHub API response is the more correct choice, not scope creep. No
   hand-rolled crypto/auth — the GitHub token flows as a file/`--secret`,
   never re-derived; SHA256 verification uses stdlib `sha256sum`.
6. **Decoupling** — pass. `lib.sh`/`vm.sh`/`install.sh` split mirrors
   `styrman-ctl`'s own internal shape; `vm.sh` and `install.sh` compose
   through `vm.sh`'s CLI (`"${VM}" status`, `"${VM}" ssh`) rather than
   reaching into `lib.sh` internals from outside the pair.

## Not examined

- Live network verification of `KATA_SHA256_3_31_0` (`68c2786a…`) against a
  real download of `kata-static-3.31.0-amd64.tar.zst` — the bug record
  (BUG-070) documents how it was derived and cross-checked, but I did not
  independently re-download the ~1.5 GB asset to confirm the digest.
  Whether the GitHub Releases API actually returns a `digest` field for
  this asset (the python3 fallback parser's assumption) was not checked
  live either — no network calls were made per the dispatch's `gcloud`
  restriction, and I extended that caution to other outbound fetches.
- `.agents/docs/deploy/gcp.md`, `.agents/docs/bugs/INDEX.md`, `ROADMAP.md`
  — all shown as modified in `git status` but not named by the dispatch;
  not reviewed here.
- `.agents/docs/plans/2026-09-21-gcp-compute-engine-target.md` and the
  three `.agents/docs/reviews/2026-09-21-plan-cold-read-*` rounds — not
  reviewed; this report is a code review of the scripts, not a plan
  cold-read.
- Whether `apt-get -y -qq install zstd` in `host-setup.sh:98` can run
  without a prior `apt-get update` on every fresh Debian 13 Compute Engine
  image (Docker's own convenience-script install a few lines earlier
  likely populates apt's lists as a side effect, but this was not traced
  or tested against a real fresh image).
- Whole-repo `go vet ./...` / full `go test ./...` were not re-run; only
  `go test ./internal/config/... -run TestGCP` was executed (all 11 pass).
- No live GCP call of any kind was made (per the dispatch's instruction).

## Outcome

- High, `install.sh:153` (`GH_TOKEN` read through `${STYRMAN_GH_TOKEN_FILE}`, unset in that ssh exec): **fixed** the same day. The line reads `GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")"` again; `TestGCPInstallPlan` now finds the plan's `styrman-ctl install --yes` line and requires that literal path with no `STYRMAN_GH_TOKEN_FILE` in it — the assertion failed on the reviewed line (`the install line must read the staged token by its literal path`) before the fix and passes after. Root cause: a must-fail mutation restored by an inverse global `sed` that also rewrote this line — BUG-071. Verified by the `verifier` round-3 log named in the change entry.

**Addendum, 2026-09-22.** The Outcome above quotes the install line as
`GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")"`. That form shipped for
part of a day and was superseded the same session by the v1.37.0 security
review: the token value must not sit in an argv that `sudo` logs. The line now
passes `--gh-token-file` only, and `styrman-ctl` exports the token in its own
process (`run_host_setup`). See `2026-09-22-security-v1.36.0-HEAD.md`.
