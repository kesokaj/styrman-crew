---
owner: main
date: 2026-09-21
---
# GCP target: a Compute Engine VM on the unchanged host path — the scripts, the first bring-up's findings, and the learnings taken back to the NUC   (2026-09-21)

Plan `plans/2026-09-21-gcp-compute-engine-target.md` (Phase 1 complete; Phase 2 up to the point where the VM needs a release; Phase 3 docs in this commit). ADR-075. Operator's scope: the GCP instance is temporary and disposable, the NUC stays primary, the deliverable is a reusable deploy process; "make sure you make it simple"; "take your learnings back to local if any".

## What changed

- New `deploy/gcp/`: `vm.sh` (`create | status | ssh | stop | start | delete --confirm <name>`, `--dry-run`, `--yes` for create's cost prompt), `install.sh` (`[--secrets-dir D] [--version X.Y.Z] [--user U]`: stage three secret files, clone this repo at the release tag on the VM, `git verify-tag` against the staged signers, `sudo styrman-ctl install --yes …`, verify the sign-in redirect, remove the staged files by trap), `lib.sh` (shared: `run()` dry-run seam, `vm.conf` loader + validation, gcloud lookup, IAP ssh, `ssh_reachable`/`ssh_hint`), `vm.conf` (non-secret defaults; renamed from `gcp.env`, which `.gitignore`'s `*.env` hid — D18).
- `deploy/styrman-ctl` `cmd_install`: exports `GH_TOKEN` from `--gh-token-file` in its own process before `host-setup.sh` (environment only, never argv — D20).
- `deploy/host-setup.sh` step 2: Kata `.tar.zst` asset, `tar --zstd` (installs `zstd` when absent), SHA256 from `KATA_SHA256`, else the pinned `KATA_SHA256_3_31_0` (GitHub's release-asset digest, matched against a same-day download), else the release API digest via python3; fail-closed unchanged (D19, BUG-070).
- `deploy/verify-commit.sh` lines 168–171: `deploy/gcp/*.sh` under `bash -n` and shellcheck.
- Tests: `internal/config/gcp_scripts_test.go` (11 `TestGCP*`), `TestStyrmanCtlInstallExportsTokenForHostSetup` in `styrman_ctl_test.go`.
- Docs: `.agents/docs/deploy/gcp.md` rewritten from "Not applicable"; `deploy/README.md`, `README.md`, `AGENTS.md`, `testing/styrman/README.md`, `specs/styrman/deploy.md`, `specs/styrman/versioning.md`, `deploy/remote.md` by the `documentation` agent; `testing/guide.md` (absent until now) by the `verifier`; ADR-075 by the `changelog` agent — each read against the diff before this commit.
- Trail: plan; cold reads `reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target{,-round-2,-round-3}.md`; verifier logs `testing/2026-09-21-verify-gcp-compute-engine-target-phase-1{,-round-2,-round-3}.md`; `reviews/2026-09-21-review-gcp-compute-engine-target.md`, `reviews/2026-09-21-security-gcp-compute-engine-target.md`; BUG-069, BUG-070, BUG-071; `ROADMAP.md` "Now" points at the plan.

## Verification

- Cold read: three rounds (`design needs revision` ×3; round 3 `implementation ready`, zero blocking, three low items → D15); cap reached, operator override "Proceed (Recommended)".
- Tests red first: the Go file failed with `fork/exec …/deploy/gcp/vm.sh: no such file or directory` before the scripts existed; every later assertion was red before its fix (D17, the review fix, the three security fixes). Must-fail mutations: `--no-service-account` removed → `TestGCPVMCreatePlan` FAIL; the token path interpolated into the helper → `TestGCPInstallCloneHelperShape` FAIL; each `ssh_reachable` line removed → `TestGCPSSHUnreachableIsNamed` FAIL; all restored (the last two from byte copies, `cmp` identical — see BUG-071 for why).
- `verifier`: round 1 PASS (pre-IAP tree), round 2 PASS (IAP, `vm.conf`, Kata; Kata digest cross-checked live against GitHub), round 3 over the final tree: `testing/2026-09-21-verify-gcp-compute-engine-target-phase-1-round-3.md` Status PASS (vet 0; `go test ./... -count=1` 0; 14/14 of the `TestGCP*`/`TestStyrmanCtlInstall*` selection; bash -n and shellcheck 0, `styrman-ctl` shellcheck-clean too; the ingress module ok; no `GH_TOKEN=` in `install.sh`, every changed file text, no secret-shaped path in `git status`). Main-thread bare runs on the final tree: `go vet ./...` exit 0, `go test ./...` exit 0 (15 packages ok), `shellcheck -S warning deploy/gcp/*.sh deploy/host-setup.sh` exit 0.
- Real GCP (project `driven-eft-h4it`): `vm.sh --dry-run --yes create` printed the exact command line; `vm.sh --yes create` reserved `styrman-static` and created `styrman` (`n4-standard-8`, `europe-north2-a`, 34.51.142.50), ssh ready; `/dev/kvm` present (`kvm_rc=0`), 16 `vmx` lines, Debian 13.6, root disk auto-grown to 197 GB; `vm.sh status` → RUNNING, "styrman not installed", exit 1 (over IAP after the `ssh` tag was removed and `block-project-ssh-keys=TRUE` added); effective firewalls from 0.0.0.0/0: `allow-http`, `allow-https`, `allow-icmp` only.
- Secrets dir `~/.config/styrman/gcp/` (700; `env`, `gh-token`, `allowed_signers` 600, 7 keys, 1 signer line, no value printed) built by one root pipeline on the NUC: `umask 077; sudo -n grep -E '^(GOOGLE_CLIENT_ID|GOOGLE_CLIENT_SECRET|CLOUDFLARE_API_TOKEN|ADMIN_EMAIL|STYRMAN_ALLOWED_EMAILS)=' /opt/styrman/deploy/.env > env; printf 'BASE_DOMAIN=gcp.styrman.xyz\nSTYRMAN_HOSTNAME=crew\n' >> env; sudo -n cat /etc/styrman/gh-token > gh-token; sudo -n cat /etc/styrman/allowed_signers > allowed_signers; chmod 600 *`.

## Findings and outcomes

- Code review (one round): one high — the remote install line read the token through `${STYRMAN_GH_TOKEN_FILE}`, unset in that ssh exec (a mutation restore's inverse `sed` had rewritten it, BUG-071) — fixed, tested. `## Outcome` in the report.
- Security review (one round): high (token value in a sudo-logged argv on the VM) fixed in the shared `styrman-ctl` + the GCP line; medium (test locking that shape) fixed with it; medium (`block-project-ssh-keys`) fixed and applied live; medium ("no public 22" unasserted) verified live and added to `gcp.md`; low (tag signature) taken — `verify-tag` before install; two lows accepted/rebutted. `## Outcome` in the report.
- Bring-up findings: BUG-069 (an ssh outage on the NUC's own path to the VM's public 22 was reported as "no /dev/kvm" / "not installed"; IAP + reachability probe), BUG-070 (the Kata step in `host-setup.sh` was dead for any fresh host), BUG-071 (the `sed` restore).

## Learnings for the local host (D14)

1. `host-setup.sh` could not provision a fresh host: the Kata download URL and its sidecar checksum no longer exist upstream; fixed in the shared script (BUG-070). The NUC never ran the step since its first bring-up.
2. The token for the image build never belongs on a command line: `styrman-ctl install --gh-token-file` now exports it for `host-setup.sh`; the documented break-glass `sudo GH_TOKEN=$(gh auth token) bash host-setup.sh` is superseded in the docs (D20).
3. A check made over ssh names the transport's failure first (BUG-069) — nothing to apply on the NUC (`styrman-ctl` has no remote checks); the rule lives in the bug record.
4. `host-setup.sh` and `styrman-ctl` both pass `shellcheck -S warning` today but are not on the gate's shellcheck line — left as a follow-up in the plan, not changed here.

## State and next step

- The VM is up and idle (billing; `deploy/gcp/vm.sh stop` keeps only the disk). Docker CE 29.7.2 is installed on it, `~/styrman-crew` is cloned at v1.36.0, no styrman stack yet: v1.36.0's `host-setup.sh` cannot install Kata (BUG-070), and `install.sh` installs from the released tag only.
- Next: the operator releases (`/preflight`, next MINOR: a new `deploy/gcp` surface), then `deploy/gcp/install.sh` completes the bring-up and the plan's remaining Phase 2 boxes are ticked from its output; then the operator adds the OAuth redirect URI `https://auth.gcp.styrman.xyz/oauth2/google/authorization-code-callback` and signs in (plan, E2E criterion 3).
- The NUC is untouched: nothing under `/var/lib/styrman`, `/etc/styrman`, `/opt/styrman` was written; the only reads were the secrets derivation above.
