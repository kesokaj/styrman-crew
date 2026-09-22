---
owner: main
date: 2026-09-22
---
# Preflight — v1.37.0

Range reviewed: v1.36.0..HEAD (HEAD = the release commit this report is committed in; the gate ran against 2b84e94 plus the fixes it produced, which that commit contains and nothing else).

## VERDICT: SHIP

No blocking finding stands. Round 1 of the code review returned REQUEST CHANGES with one high — a real defect, reproduced and fixed here, not waved through — and round 2 returned APPROVE WITH NITS; the security review found no high. Every finding is fixed or rebutted in its report's `## Outcome`, each fix carries a test that failed before it, and the whole suite is green in both terminal states.

## Fixed during the gate, worst first

| # | Severity / confidence | Finding | Outcome |
|---|---|---|---|
| 1 | high / high | `internal/config/gcp_scripts_test.go`: the test harness lent its child this terminal, so `has_tty` was true, two tests never reached the branch they name, and the package hung under a pty (`panic: test timed out`). Every green run cited for those two tests measured the wrong thing. | Fixed: both helpers set `SysProcAttr{Setsid: true}`. Before: `--- FAIL: TestGCPVMCreateNeedsYesWithoutTerminal` then a hang; after: `ok … 6.4s` under a pty and without one. |
| 2 | medium / high | `deploy/gcp/lib.sh`: the new ssh-exposure check failed OPEN and SILENT — a missing `python3` or a filter error read as "nothing exposed" (BUG-034's family, inside the check the security review asked for). | Fixed: the filter's exit status and an `OK` sentinel are both read; anything else refuses and names the override. An unlisted firewall refuses too. Mutation: guard removed → `an unrunnable check did not refuse: code 0`. |
| 3 | medium / high | `TestStyrmanCtlInstallExportsTokenForHostSetup` asserted only a log line: deleting `export GH_TOKEN` kept it green — the very line that repaired the previous round's high. | Fixed: `styrman-ctl` gained `run_host_setup` (the real path `cmd_install` calls) and a `__run-host-setup` hook; the test runs it against a fake `host-setup.sh` and reads the environment it received. Mutation: export deleted → `GH_TOKEN=[<unset>]`. |
| 4 | medium / high | "No public port 22" was asserted by nothing: true in `driven-eft-h4it` today, false in a project with an untagged `default-allow-ssh`, while `vm.conf`, `gcp.md` and ADR-075 all state it. | Fixed: `assert_no_public_ssh` runs before anything is reserved or created. Ten rule shapes tested (bare `tcp:22`, a `20-30` range, portless `tcp`, `all` on our tag, numeric protocol `6`, `::/0`; and empty, another tag, web ports, an internal range, disabled, egress, service-account-scoped), plus the two unrunnable cases and the override. |
| 5 | medium / high | The NUC's live Cloudflare token, OAuth secret and GitHub token were reused on a host the design calls disposable; compromise there reaches the production name. | Guidance reversed: `gcp.md` now requires a separate OAuth client and a separate scoped Cloudflare and GitHub token per host, naming what reuse costs. The VM already holds the reused secrets — rotation is an operator action, in the handoff's Next Steps. |
| 6 | medium / high | `install.sh` re-install ran `checkout` on an existing VM checkout, keeping local edits and untracked files, so `verify-tag` vouched for the tag while another tree ran as root. | Fixed: `fetch --tags && reset -q --hard v<VERSION> && clean -qfdx` before the verification. |
| 7 | low / high | `verify-tag` ran inside a VM-user-writable checkout, where a repo-local `gpg.ssh.program` could subvert it. | Fixed: `-c gpg.format=ssh -c gpg.ssh.program=ssh-keygen` pinned on the command line. |
| 8 | low / high | `--dry-run ssh` opened a real session, against the header's promise. | Fixed and tested; the planned line is printed shell-quoted so it can be re-run as printed. |
| 9 | low / high | The `__run-host-setup` hook did not constrain a relative script path. | Fixed: the hook requires an absolute path, which `cmd_internal` then confines to `TMPDIR`. |
| 10 | low / medium | Staged secrets survive a failed cleanup and the warning named no command; `GH_TOKEN` was read during a dry run; describing text outlived `block-project-ssh-keys=TRUE`. | All three fixed. |

Rebutted, with the reason in the security report's `## Outcome`: the Kata digest sharing a trust root with the download (the shipped version is pinned to a constant matched against a same-day download; other versions still fail closed; `KATA_SHA256` pins it offline) and `--secrets-dir`'s symlink check covering only the last component (the path is the operator's own home).

## Gate rows

- **Verification — PASS.** `deploy/verify-commit.sh HEAD` on 2b84e94 (`testing/2026-09-22-verify-v1.37.0-release.md`): "safe to tag" — build, vet, `-race` tests, the vendored ingress module, gofmt, `web/dist` freshness, the frontend trio, `bash -n` + shellcheck over the host scripts including `deploy/gcp/*.sh`, and both image builds. Re-run on the release commit itself after the gate's fixes: `testing/2026-09-22-verify-v1.37.0-release-round-2.md`. Main-thread bare runs on the final tree: `go vet ./...` 0; `go test ./...` 0, 15 packages ok; `go test ./internal/config/` under a pty 0; the ingress module 0; `shellcheck -S warning` over `deploy/gcp/*.sh deploy/styrman-ctl deploy/host-setup.sh` 0; `gofmt -l` empty.
- **Code review — round 1 REQUEST CHANGES, round 2 APPROVE WITH NITS.** `reviews/2026-09-22-review-v1.36.0-HEAD.md`, `reviews/2026-09-22-review-v1.37.0-round-2.md`. Round 2's three findings are rows 2, 4 (false positives and a false negative in the new filter) and 9 above; all fixed after it reported.
- **Security — FINDINGS, no high.** `reviews/2026-09-22-security-v1.36.0-HEAD.md`, every release-diff path read end to end, `## Not examined` names none of them.
- **Docs lens — APPROVE WITH NITS.** `reviews/2026-09-22-review-v1.36.0-HEAD-docs.md`: one medium, fixed here — `versioning.md` now names the GCP host on the auto-updater path and carries the fresh-host requirement as release-procedure step 0.
- **Trail — pass.** Newest `changes/` entry describes this work; no future `date:`; `changes/` and `DECISIONS.md` append-only in range (the one modified line was v1.36.0's `DEPLOY_RESULT` placeholder being filled by the previous session); `ROADMAP.md` "Now" names the active plan, `TODO.md` empty.
- **Requirements — untouched in range.** No edit proposed, none awaiting approval.

## What was not checked

- **Fresh-host path unexercised.** `deploy/host-setup.sh`'s Kata download and verification (the BUG-070 fix) runs only where `/opt/kata/bin/kata-runtime` is absent. No test reaches it and the NUC never re-runs it, so this release ships it with the **fresh-host path unexercised** — the words `.agents/rules/shared-path-learnings.md` and `versioning.md` step 0 require this report to carry. It is exercised by the GCP bring-up performed immediately after this release; the result is recorded in the release entry's Deploy section.
- The real gcloud, IAP, ssh, `scp` and `git verify-tag` paths of `deploy/gcp/*.sh`: the suite drives `--dry-run` plans against a fake gcloud. The live bring-up after this release is their first real exercise.
- Tier 2 `integration.sh`, Tier 4 `web/e2e/run.sh` and `deploy/vulncheck.sh` were not run in this gate; the frontend trio ran inside `verify-commit.sh` only.
- Pre-existing dependency advisories, none introduced by this range and none fixed here: `GO-2026-4887` and `GO-2026-4883` in `github.com/docker/docker@v28.5.2+incompatible` (upstream "Fixed in: N/A") and `js-yaml` GHSA-2883-xcg3-v3hh in `web/` (a fix exists). Bumping the frontend at tag time would invalidate the verification this gate took; left as a dependency task.
- The live GCP instance still holds credentials copied from the NUC (row 5). The documentation now forbids that for a new host; the existing VM is not re-provisioned by this release.
