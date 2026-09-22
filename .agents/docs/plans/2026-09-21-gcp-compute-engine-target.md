---
owner: main
date: 2026-09-21
---
# Plan — GCP target: a Compute Engine VM running the unchanged host path   (2026-09-21)

## Why

The operator (2026-09-21): "I know we can install this locally, now I want you
to figure out to install this in gcp. You have access to a gcp account. Do no
remove the local one, just add install in gcp account" and, on the scope
table: "The gcp for now is only temporary, I want to keep 'Dave' local setup.
But for work later I will use gcp. This is just so we have a deploy process
for gcp that I can use later."

The deliverable is therefore a REPEATABLE GCP deploy process committed to
this repo — scripts under `deploy/gcp/` and the guide
`.agents/docs/deploy/gcp.md` (today: "Not applicable") — exercised once now
by bringing up a second, disposable styrman instance in the operator's GCP
project. The production NUC (`dave`, crew.styrman.xyz) is not touched:
nothing under `/var/lib/styrman`, `/etc/styrman` or `/opt/styrman` on it is
written; the only local read is the one-time, non-printing derivation of the
GCP host's `.env` from the NUC's (D4).

`REQUIREMENTS.md` R11 already names "a GCP VM with nested virtualization" as
a supported host; `deploy/host-setup.sh` line 4 claims the same. Nothing in
`.agents/docs/bugs/INDEX.md` names GCP; BUG-056 (Docker Hub anonymous pull
budget) applies to any host that builds the runtime image.

## Scope

In:
- `deploy/gcp/vm.sh` — the VM's lifecycle through `gcloud` (`create`,
  `status`, `ssh`, `stop`, `start`, `delete`), idempotent, `--dry-run`,
  typed confirmation on `delete`.
- `deploy/gcp/install.sh` — from the operator's machine: clone this repo at a
  release tag on the VM, stage the three secret files there, run
  `styrman-ctl install --yes …` (the SAME install path as the NUC, ADR-050/
  ADR-052 tooling unchanged), verify, remove the staged secrets.
- `deploy/gcp/gcp.env` — the committed, non-secret defaults (project, zone,
  instance name, machine type, disk, image family, tags, static-IP name).
- A Go test of both scripts' `--dry-run` plans (`internal/config/gcp_scripts_test.go`),
  in the `styrman_ctl_test.go` pattern (fake tools on PATH, scratch prefix).
- `deploy/verify-commit.sh`'s bash -n / shellcheck lists gain `deploy/gcp/*.sh`.
- One bring-up in project `driven-eft-h4it` at v1.36.0, verified end to end.
- Docs: `.agents/docs/deploy/gcp.md` (the five defaults, procedure, verify,
  rollback, teardown), `deploy/README.md`, `README.md` (the host list),
  `AGENTS.md` Deploy section, `.agents/docs/testing/styrman/README.md` Tier 1
  count; ADR-075.

Out:
- Cloud Run, GKE, Terraform (Rejected rows).
- Migrating any data from the NUC; a shared state between the two hosts; DNS
  cut-over of crew.styrman.xyz.
- A GCP-specific fork of `host-setup.sh`, `styrman-ctl`, the updater, the
  firewall, the server or the image. What the bring-up teaches that ALSO
  holds for the NUC is applied to the shared path in the same change (D14):
  a new box under Phase 2 ending `— found in:`, and a "Learnings for the
  local host" section in the change entry, empty or not.
- OS Login, a VPN. (IAP-only ssh was Out here until the bring-up: D16.)

Follow-ups noted, not built:
- `deploy/verify-commit.sh` runs only `bash -n` on `host-setup.sh` and
  `styrman-ctl`; both pass `shellcheck -S warning` today (checked
  2026-09-21) and could join the shellcheck line. found in: BUG-070.
- BUG-072 and BUG-073 (found by the live bring-up, fixed after the tag): the optional `.env` keys `styrman-ctl install` does not carry, and a `status` probe of a path nothing creates. found in: the Phase 2 bring-up.
- `gcloud compute ssh` forwards the NUC's `LC_ALL=en_US.UTF-8`, which the
  Debian GCE image lacks (`bash: warning: setlocale`); cosmetic — a
  `--ssh-flag='-o SetEnv=LC_ALL=C.UTF-8'` would silence it. found in:
  `vm.sh ssh` (Phase 2 nested-virt gate).
- `vm.sh` resize (change machine type) — `gcloud compute instances set-machine-type`
  by hand until asked.
- Scanning the sh-agent binary in `deploy/vulncheck.sh` (carried from the
  v1.36.0 handoff, unrelated).

## Surfaces touched

Read this session (no explorer brief: 9 files):
- `deploy/styrman-ctl` — `cmd_install` (lines 428–501): `--yes --user U
  --version X.Y.Z --env-file F --gh-token-file F --allowed-signers-file F`
  is fully unattended, including `setup_auto_update` (lines 520–616); refuses
  to run from `/opt/styrman`; `private_file` (507–518) requires owner root,
  `$SUDO_USER` or the caller, mode `600`, parent not group/other-writable;
  `https_origin` (412–426) needs a plain https `origin` on the checkout.
- `deploy/host-setup.sh` — line 37 refuses a host without `/dev/kvm`;
  `--user` must be an existing account (line 30); step 7's image build takes
  `GH_TOKEN` or `/etc/styrman/gh-token` (259–260).
- `deploy/verify-commit.sh` lines 167–174 — the bash -n / shellcheck lists.
- `deploy/caddy/ingress/cmd.go:187` — `syncDNS` detects the public IP
  (`ifconfig.me`, `api.ipify.org`) and writes the Cloudflare A records, so a
  static external IP is what makes the records stable.
- `deploy/.env.example` lines 5–21 — the six must-fill keys;
  `https://${STYRMAN_HOSTNAME}.${BASE_DOMAIN}` and `https://auth.${BASE_DOMAIN}`.
- `internal/config/styrman_ctl_test.go` — the fake-tools-on-PATH test pattern.
- `.agents/docs/deploy/gcp.md`, `deploy/README.md`, `README.md:241`,
  `AGENTS.md` "Deploy".
- GCP (gcloud 584.0.0, account ostling.simon@gmail.com, ADC ok): project
  `driven-eft-h4it` (241339653094), billing on, Compute API on, no
  instances; VPC `driven-eft-h4it`, subnet `europe-north2` 10.0.0.0/22;
  firewall rules by target tag `http` (80), `https` (443), `ssh` (22,
  0.0.0.0/0); reserved external IP `headscale-static` (not ours, left
  alone); project metadata already carries two ssh keys. Zone
  `europe-north2-a` offers `n4-standard-4/8`, `c4-standard-4/8`,
  `e2-standard-4/8`; image `debian-13-trixie-v20260921` (family `debian-13`,
  project `debian-cloud`).
- Google's nested-virtualization overview (docs.cloud.google.com, fetched
  2026-09-21): not supported on E2, memory-optimized, Arm, or AMD except N4D.

## Interfaces

### `deploy/gcp/vm.conf` (committed; every key overridable from the environment)

Named `vm.conf`, not `gcp.env`: the repository's `.gitignore` line 2
ignores `*.env`, and an `.env`-shaped name reads as a secret file (D18).

```
GCP_PROJECT=driven-eft-h4it
GCP_ZONE=europe-north2-a
GCP_SUBNET=europe-north2
GCP_INSTANCE=styrman
GCP_MACHINE_TYPE=n4-standard-8
GCP_BOOT_DISK_SIZE=200GB
GCP_BOOT_DISK_TYPE=hyperdisk-balanced
GCP_IMAGE_FAMILY=debian-13
GCP_IMAGE_PROJECT=debian-cloud
GCP_TAGS=http,https              # no public 22: ssh tunnels through IAP (D16)
GCP_ADDRESS=styrman-static       # the reserved external IP's name; region = the zone's
GCP_SSH_VIA_IAP=1                # 0 = the public path (then add the `ssh` tag)
```

`vm.sh` reads it from its own directory line by line and sets a key only
when it is unset, so a `GCP_*` value already in the environment wins over
the file (a plain `source` would clobber it).

### `deploy/gcp/lib.sh` (implementer)

Shared by both scripts, never run directly: `log`/`warn`/`die`/`run`, the
`gcp.env` loader and its validation, the `gcloud` lookup, `instance_state`
and `vm_ssh`. Covered by the same `bash -n`/shellcheck lines.

### `deploy/gcp/vm.sh`

```
deploy/gcp/vm.sh [--dry-run] [--yes] create
deploy/gcp/vm.sh [--dry-run] status
deploy/gcp/vm.sh [--dry-run] ssh [-- CMD…]
deploy/gcp/vm.sh [--dry-run] stop | start
deploy/gcp/vm.sh [--dry-run] delete --confirm <GCP_INSTANCE>
deploy/gcp/vm.sh help
```

- `gcloud` is found on `PATH`, else at `$HOME/google-cloud-sdk/bin/gcloud`
  (this box); absent → exit 1 naming `gcloud-setup`.
- Every side effect goes through `run()`; `--dry-run` prints `+ gcloud …`
  and runs nothing (reads — `describe`, `list` — still run so the plan is
  the real one; a read failing under `--dry-run` is reported as
  `(dry run) would …`).
- `--yes` gates exactly one prompt: `create`'s cost confirmation. Before the
  `instances create`, `create` prints
  `Create <GCP_INSTANCE> (<GCP_MACHINE_TYPE>, <GCP_BOOT_DISK_SIZE> <GCP_BOOT_DISK_TYPE>) in <GCP_ZONE>: billed while running (about USD 250–300/month for n4-standard-8) — continue? [y/N]`
  and reads from `/dev/tty`; `--yes` answers y; with no terminal and no
  `--yes` it exits 1 with `pass --yes to create unattended` and runs
  nothing. The prompt is skipped when the instance already exists (no-op
  path). `status`, `ssh`, `stop`, `start` have no prompt; `--yes` is
  accepted and ignored there; `delete` is gated by `--confirm` alone, never
  by `--yes`. A dry run obeys the same rule (`--dry-run create` without
  `--yes` and without a terminal refuses), so it shows exactly what the
  unattended run would do — `TestGCPVMCreateNeedsYesWithoutTerminal`.
- `create`: (1) `gcloud compute addresses describe $GCP_ADDRESS --region
  "${GCP_ZONE%-*}"` (the zone's region: `europe-north2-a` → `europe-north2`) → absent: `addresses create` (RESERVED, external);
  (2) `instances describe $GCP_INSTANCE` → present: print its state and IP,
  exit 0 (idempotent); absent:
  ```
  gcloud compute instances create $GCP_INSTANCE --project=… --zone=…
    --machine-type=$GCP_MACHINE_TYPE --enable-nested-virtualization
    --image-family=$GCP_IMAGE_FAMILY --image-project=$GCP_IMAGE_PROJECT
    --boot-disk-size=$GCP_BOOT_DISK_SIZE --boot-disk-type=$GCP_BOOT_DISK_TYPE
    --subnet=$GCP_SUBNET --address=$GCP_ADDRESS --tags=$GCP_TAGS
    --no-service-account --no-scopes
    --shielded-secure-boot --shielded-vtpm --shielded-integrity-monitoring
    --metadata=enable-oslogin=FALSE
    --labels=app=styrman
  ```
  then waits until `ssh -- true` succeeds (≤ 5 min) and prints the external
  IP. Under `--dry-run` the wait and the IP print are skipped entirely and
  one line `+ (dry run) would wait for ssh and print the external IP` is
  printed instead — nothing under `--dry-run` may block on a VM that does
  not exist.
- `status`: instance `status` + external IP; if RUNNING, also the exit of
  `ssh -- sudo styrman-ctl status` when `/usr/local/bin/styrman-ctl` exists
  there (else "styrman not installed"). Exit 0 running+healthy, 1
  running+degraded/not installed, 2 no instance, 3 instance exists but is
  not RUNNING (stopped, staging, …). One output shape (D9).
- `ssh`: `gcloud compute ssh $GCP_INSTANCE --project --zone
  --tunnel-through-iap -- [CMD…]` (`gcloud` adds this machine's key to
  project metadata on first use; IAP TCP forwarding through the project's
  `allow-iap` rule, D16). `scp` in `install.sh` tunnels the same way.
- Every diagnosis over ssh starts with `ssh_reachable` (one `true`): `status`
  prints `ssh to <instance> unreachable — gcloud … --troubleshoot` and exits 1
  before any styrman check; `install.sh` dies `ssh to <instance> failed — …`
  before its KVM gate (D17).
- `delete`: refuses without `--confirm <GCP_INSTANCE>` (exit 2, nothing run);
  with it: `instances delete --quiet` (boot disk auto-deleted with the
  instance) then `addresses delete --quiet`. Prints the plan first.
- Instance name, zone, project and tags are validated against
  `^[a-z]([-a-z0-9]*[a-z0-9])?$` / `^[a-z0-9-]+$` / `^[a-z0-9,-]+$` before
  reaching any command line. `install.sh`'s `--user` is validated the same
  way against `^[a-z_][a-z0-9_-]{0,31}$` (a POSIX login name) before it
  reaches the remote command string; `--version` against ADR-053's regex.
- Shielded VM flags (`--shielded-secure-boot --shielded-vtpm
  --shielded-integrity-monitoring`): D10.

### `deploy/gcp/install.sh`

```
deploy/gcp/install.sh [--dry-run] [--secrets-dir D] [--version X.Y.Z] [--user U]
```

The bare command is the whole procedure (D13): `--secrets-dir` defaults to
`$HOME/.config/styrman/gcp` and holds three fixed-name files — `env`,
`gh-token`, `allowed_signers` — each checked as below; the directory itself
must be mode 700 and owned by the caller. The older three-flag form is not
offered.

- Runs on the operator's machine; every remote step goes through
  `vm.sh ssh --` / `gcloud compute scp`. Same `run()`/`--dry-run` seam.
- `env` (the former `--env-file`): a private file (checked exactly as `styrman-ctl`'s
  `private_file`: not a symlink, mode 600, owner the caller, parent not
  group/other-writable) with the six must-fill keys for the GCP host
  (`BASE_DOMAIN GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET CLOUDFLARE_API_TOKEN
  ADMIN_EMAIL STYRMAN_ALLOWED_EMAILS`); each must be non-empty (checked
  locally, values never printed); the optional `STYRMAN_HOSTNAME` (default
  `styrman`, as `deploy/.env.example`) is read for the verify URL only.
  `gh-token`: the fine-grained token that reads BOTH `kesokaj/styrman-crew`
  (the VM's clone and the updater's) and `kesokaj/styrman-harness` (the
  image build, ADR-074). `allowed_signers`: one line, as `styrman-ctl`
  takes it.
- `--version` default: this checkout's `git describe --tags --abbrev=0`
  without `v` (must match `^[0-9]+\.[0-9]+\.[0-9]+$`, ADR-053; never a sha).
  `--user` default: the local `$USER` (gcloud's remote login name).
- Steps, each logged `==> install: …`:
  1. `vm.sh status` must report RUNNING (exit 0 or 1); on exit 3 (exists,
     not running) stop with "run deploy/gcp/vm.sh start first"; on exit 2
     (no instance) stop with "run deploy/gcp/vm.sh create first".
  1b. Remote `test -e /dev/kvm` must exit 0; else stop (exit 1) with
     "no /dev/kvm on the VM: nested virtualization is off or unsupported for
     GCP_MACHINE_TYPE — recreate with a supported Intel type" — a HARD gate,
     because `host-setup.sh` lines 37–39 only warn when `/dev/kvm` is
     missing but cpuinfo shows `vmx` (D8).
  2. Remote: `sudo apt-get update -q && sudo apt-get install -y -q git curl
     python3` (the Debian GCE image lacks git and ships with an empty apt
     cache).
  3. Remote: stage dir `~/.styrman-install` (`install -d -m 700`); scp the
     three files there; `chmod 600`.
  4. Remote clone, the `setup_auto_update` shape exactly (`styrman-ctl`
     lines 562–571, security review 2026-09-16): the token-file PATH reaches
     the helper through the environment of the one git call, never
     interpolated into the snippet the helper's shell parses, and the helper
     is scoped to the origin's host with every inherited helper cleared:
     ```
     env GIT_TERMINAL_PROMPT=0 STYRMAN_GH_TOKEN_FILE="$HOME/.styrman-install/gh-token" \
       git -c credential.helper= \
           -c 'credential.<ORIGIN_HOST>.helper=!f() { echo username=x-access-token; printf '"'"'password=%s\n'"'"' "$(cat "${STYRMAN_GH_TOKEN_FILE}")"; }; f' \
           clone --branch v<VERSION> -- <ORIGIN> "$HOME/styrman-crew"
     ```
     where `<ORIGIN>` is this checkout's `origin` rewritten to https by the
     `styrman-ctl` `to_https` rule and validated by the same
     `^https://[a-z0-9.-]+/[A-Za-z0-9._/-]+$` regex, and `<ORIGIN_HOST>` is
     its `https://host` prefix; so a fork installs itself. If
     `$HOME/styrman-crew` exists: `fetch --tags` + `checkout v<VERSION>` with
     the same env line and helper (D7). The whole remote command is one
     string handed to `vm.sh ssh --`; the helper snippet is a constant in
     `install.sh` with only `<ORIGIN_HOST>` substituted, after validation.
     No `--depth`: the repository is 17 MB packed and a full clone lets the
     later `fetch --tags` work without shallow-clone caveats (implementer).
  5. Remote: `sudo env GH_TOKEN="$(cat ~/.styrman-install/gh-token)"
     ~/styrman-crew/deploy/styrman-ctl install --yes --user <U> --version
     <VERSION> --env-file ~/.styrman-install/env --gh-token-file
     ~/.styrman-install/gh-token --allowed-signers-file
     ~/.styrman-install/allowed_signers` — `GH_TOKEN` because
     `host-setup.sh` step 7 builds the image before `styrman-ctl` installs
     the token file; `styrman-ctl install` then builds the same image again
     with `--gh-token-file`, a cache hit.
  6. Remote: `rm -rf ~/.styrman-install` (always, also on failure — a trap).
  7. Verify from here: `vm.sh status` exit 0; `curl -sS -o /dev/null -w '%{http_code} %{redirect_url}'
     https://<STYRMAN_HOSTNAME:-styrman>.<BASE_DOMAIN>/` is `302 https://auth.<BASE_DOMAIN>/…`
     (retried ≤ 3 min while the ingress obtains its certificate). Prints the URL.
- Secret values never reach a log line or an argv visible in `ps` on this
  machine: files travel by scp; the token reaches git through a helper that
  reads a file; the one `GH_TOKEN=` is inside the remote `sudo env` command
  string, which is what `styrman-ctl` itself documents for the first
  bring-up (`deploy.md` "Bring-up" line 199).

### Tests — `internal/config/gcp_scripts_test.go`

Fake `gcloud`, `curl`, `git` on PATH recording argv; `HOME` and the env file
under `t.TempDir()`; `GCP_*` overridden to test values.
- `TestGCPVMCreateNeedsYesWithoutTerminal`: `vm.sh --dry-run create` (go
  test has no tty) → exit 1 naming `--yes`, no `create` line in the plan.
- `TestGCPVMCreatePlan`: `vm.sh --dry-run --yes create` with a fake gcloud whose
  `describe` calls exit 1 → the plan contains ONE `instances create` line
  with every flag in Interfaces (asserted individually:
  `--enable-nested-virtualization`, `--no-service-account`, `--no-scopes`,
  `--tags=http,https,ssh`, `--boot-disk-type=hyperdisk-balanced`,
  `--machine-type=n4-standard-8`, `--address=styrman-static`) and one
  `addresses create`; with `describe` exiting 0 → no create lines.
- `TestGCPVMDeleteRefusesWithoutConfirm`: exit 2, no `delete` in the plan;
  with `--confirm styrman`: instance delete before address delete.
- `TestGCPVMRejectsBadNames`: `GCP_INSTANCE='x;rm'` → exit 1 before any
  gcloud call (calls log empty); `install.sh --user 'a b'` → exit 1 naming
  `--user`, calls log empty.
- `TestGCPInstallPlan`: a 700 secrets dir whose `env` has
  `GOOGLE_CLIENT_SECRET=SENTINEL-abc`;
  `--dry-run` plan contains the scp of the three files, the clone with
  `--branch v1.36.0`, the remote `styrman-ctl install --yes … --version 1.36.0`,
  and the cleanup; and `SENTINEL-abc` appears NOWHERE in stdout/stderr.
- `TestGCPInstallRefusesLooseEnvFile`: `env` mode 644 → exit 1 naming
  `chmod 600`; a missing key → exit 1 naming the key; a missing
  `gh-token` → exit 1 naming the path.
- `TestGCPInstallNeedsRunningVM`: fake gcloud `describe` says `TERMINATED` →
  exit 1 naming `vm.sh start`; `describe` failing (no instance) → exit 1
  naming `vm.sh create`.
- `TestGCPInstallNeedsKVM`: fake gcloud `ssh` exits 1 for a command
  containing `test -e /dev/kvm` → exit 1 naming `/dev/kvm` and
  `GCP_MACHINE_TYPE`, and the calls log holds no `scp` and no `clone`.
- `TestGCPSSHUnreachableIsNamed` (D17): fake gcloud `FAKE_SSH_RC=255` →
  `status` exit 1 naming "unreachable", never "not installed"; `install.sh`
  exit 1 naming "ssh to styrman failed" and `--troubleshoot`, never
  `/dev/kvm`. `TestGCPInstallPlan` also asserts every `compute scp` line
  carries `--tunnel-through-iap`.
- `TestGCPInstallCloneHelperShape`: the dry-run plan's clone line carries
  `STYRMAN_GH_TOKEN_FILE=` on the env line, `credential.helper=` (cleared),
  a helper scoped to `credential.https://github.com.helper`, and the literal
  `"${STYRMAN_GH_TOKEN_FILE}"` inside the snippet — never the staged token
  path itself inside the `-c` string.

## Failure modes

- `gcloud` missing / not logged in / wrong project: `vm.sh` exits 1 with the
  gcloud error and the `gcloud-setup` hint; nothing created.
- Zone has no `n4-standard-8` capacity or nested virt refused: `instances
  create` fails; the reserved address stays (idempotent re-run reuses it);
  the message names `GCP_MACHINE_TYPE` and `GCP_ZONE` as the knobs.
- Instance exists: `create` is a no-op that prints state and IP.
- ssh never comes up within 5 min: `create` exits 1 with the serial-console
  command (`gcloud compute instances get-serial-port-output`).
- `install.sh`: a loose or incomplete env file is refused before any remote
  step; a VM without `/dev/kvm` is refused before any file is staged; clone failure (token lacks the repo) exits with git's message and the
  stage dir removed; `styrman-ctl install` failure surfaces its own `die`
  line (its `wait_healthy` names `docker logs styrman-server`), stage dir
  removed; the verify `curl` failing after 3 min exits 1 with the last
  status but leaves the stack up (the ingress may still be obtaining a
  certificate — re-run `vm.sh status`).
- Google sign-in fails with `redirect_uri_mismatch` until the operator adds
  `https://auth.<BASE_DOMAIN>/oauth2/google/authorization-code-callback` to
  the OAuth client: documented as the one manual prerequisite, not detectable
  by the scripts.
- `delete` with the wrong word: exit 2, nothing run.

## Verification

Tiers: unit (Go, the dry-run plans — Tier 1 of `testing/styrman/README.md`),
shellcheck/bash -n in `verify-commit.sh`, cloud smoke (the real bring-up),
user e2e (browser sign-in).

E2E criteria:
1. Golden path: from this machine, `deploy/gcp/vm.sh create` then
   `deploy/gcp/install.sh …` end with `vm.sh status` exit 0 and a `302` from
   the app URL to `auth.<BASE_DOMAIN>`; on the VM `sudo styrman-ctl status`
   shows the server and caddy on 1.36.0, the updater timer active.
2. Edge: `deploy/gcp/vm.sh delete` without `--confirm` exits 2 and `vm.sh
   status` afterwards still shows RUNNING.
3. User: sign in at the app URL with an allowed Google account, create a
   project, open a terminal, `sh-agent version` prints `sh-agent v1.6.0`.
4. Untouched NUC: after Phase 2, on this machine `sudo styrman-ctl status`
   is unchanged from before (same version, same project count) and
   `git -C /opt/styrman status --short` is empty.

## Phases

- [x] Cold-reader check — three rounds (`reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target{,-round-2,-round-3}.md`): round 3 `implementation ready`, `design needs revision` over three low items (D15); cap reached; the operator's override 2026-09-21: "Proceed (Recommended)" to "May I proceed to code under that override?"

### Phase 1 — the reusable scripts and their tests
- [x] `internal/config/gcp_scripts_test.go` — ten tests, red first — verify: unit, `go test ./internal/config/ -run 'TestGCP' -count=1`, who: AI — expect: FAIL naming missing `deploy/gcp/vm.sh` — evidence: `fork/exec …/deploy/gcp/vm.sh: no such file or directory`, rc=1, 2026-09-21
- [x] `deploy/gcp/gcp.env` + `deploy/gcp/lib.sh` + `deploy/gcp/vm.sh` — verify: `go test ./internal/config/ -run 'TestGCPVM' -count=1`; `shellcheck -S warning deploy/gcp/vm.sh`, who: AI — expect: `ok`, shellcheck silent, exit 0 — docs: `.agents/docs/deploy/gcp.md` — evidence: `ok … 0.663s` (all ten), `shellcheck rc=0`; must-fail mutation: `--no-service-account` removed → `--- FAIL: TestGCPVMCreatePlan`, restored, 2026-09-21
- [x] `deploy/gcp/install.sh` — verify: `go test ./internal/config/ -run 'TestGCPInstall' -count=1`; `shellcheck -S warning deploy/gcp/install.sh`, who: AI — expect: `ok`, exit 0 — docs: `.agents/docs/deploy/gcp.md` — evidence: `ok`, `shellcheck rc=0`; must-fail mutation: token path interpolated into the helper and `STYRMAN_GH_TOKEN_FILE` dropped from the env line → `--- FAIL: TestGCPInstallCloneHelperShape` (two assertions), restored, 2026-09-21
- [x] `deploy/verify-commit.sh` lists `deploy/gcp/*.sh` under bash -n and shellcheck — verify: `bash -n deploy/verify-commit.sh` and `grep -n 'deploy/gcp' deploy/verify-commit.sh`, who: AI — expect: two hits (the bash -n line and the shellcheck line) — docs: `deploy/verify-commit.sh`'s own header comment (lines 165–171; `versioning.md` names no script list — Phase 3's box) — evidence: lines 168, 170 (+171 the echo), `bash -n` exit 0, 2026-09-21
- [x] `.agents/docs/deploy/gcp.md` rewritten from "Not applicable" to the target: the five defaults (project, zone, machine, service account none, public yes), the hostname, the three secret files, `vm.sh`/`install.sh` procedure, verify, rollback (`styrman-ctl` on the VM, `versioning.md`), teardown (`vm.sh delete --confirm`), cost and `vm.sh stop` — verify: `grep -c 'vm.sh\|install.sh' .agents/docs/deploy/gcp.md`, who: AI — expect: ≥ 6, and no "Not applicable" — evidence: grep count 16, "Not applicable" count 0, 2026-09-21
- [x] Checkpoint — expect: `go test ./...` exit 0, `go vet ./...` exit 0, shellcheck clean on `deploy/gcp/*.sh`, `gcp.md` no longer says "Not applicable" — evidence: verifier log `testing/2026-09-21-verify-gcp-compute-engine-target-phase-1.md` Status PASS (vet exit 0; `go test ./... -count=1` exit 0, 15 packages ok; 10/10 `TestGCP*` PASS; bash -n exit 0; shellcheck exit 0 no output; ingress module ok; `git diff --numstat deploy/verify-commit.sh` = `3 3`; `file` reports text for every new file); the verifier also wrote the absent `testing/guide.md`, 2026-09-21

### Phase 2 — the bring-up (cloud smoke)
- [x] Assumption check: `gcloud compute instances create --dry-run`-equivalent does not exist; instead `gcloud compute machine-types describe n4-standard-8 --zone europe-north2-a` exit 0 and the nested-virt support statement cited in Surfaces — who: AI — expect: exit 0 — evidence: `n4-standard-8 8 32768`, rc=0, 2026-09-21
- [x] The secrets dir `~/.config/styrman/gcp/` (700, outside the repo): `env` derived from the NUC's `/opt/styrman/deploy/.env` in one root pipeline that prints nothing (`GOOGLE_CLIENT_ID/SECRET`, `CLOUDFLARE_API_TOKEN`, `ADMIN_EMAIL`, `STYRMAN_ALLOWED_EMAILS` copied; `BASE_DOMAIN=gcp.styrman.xyz`; `STYRMAN_HOSTNAME=crew`), `gh-token` copied from `/etc/styrman/gh-token`, `allowed_signers` from `/etc/styrman/allowed_signers` — verify: `stat -c '%a %U' ~/.config/styrman/gcp ~/.config/styrman/gcp/*` and `grep -c '^[A-Z_]*=.' ~/.config/styrman/gcp/env`, who: AI (standing root permission, `.claude/settings.json`) — expect: `700 kesokaj`, `600 kesokaj` ×3, count ≥ 7; the change entry records the exact command — evidence: `stat` printed `700 kesokaj` for the dir and `600 kesokaj` for `allowed_signers`, `env`, `gh-token`; `keys=7`; token 94 bytes, 1 signer line; no value printed, 2026-09-21
- [x] `deploy/gcp/vm.sh create` — verify: `deploy/gcp/vm.sh status`, who: AI — expect: `RUNNING`, an external IP, "styrman not installed", exit 1 — evidence: create rc=0 (address `styrman-static` reserved, instance created, ssh ready); status printed `instance styrman  RUNNING  34.51.142.50` and `styrman not installed`, rc=1; before that, `--dry-run --yes create` against the real project printed the exact planned command line and `status` exited 2, 2026-09-21
- [x] Nested-virt gate: `deploy/gcp/vm.sh ssh -- test -e /dev/kvm; echo rc=$?` — who: AI — expect: `rc=0`; on `rc=1` the phase stops here and the machine type is the knob (Risks), `install.sh` is not run — evidence: `kvm_rc=0`, 16 cpuinfo lines with `vmx`, kernel `6.12.107+deb13-cloud-amd64`, Debian 13.6, root disk auto-grown to 197 GB, 8 vCPU / 31 GB, 2026-09-21
- [x] Found in `install.sh` (first real run, 2026-09-21 ~20:03 UTC): the run died at the KVM gate with "no /dev/kvm" while the VM was healthy — the NUC's path to the VM's public port 22 had gone dead (Google's firewall log: the NUC's SYNs ALLOWED until 20:01:29, none after; scanners still reaching 22; VM sshd active, hairpin open; IAP ssh working). Two script defects (status and install attributed an ssh outage to styrman/KVM) and one design defect (a public 22 that scanners hit within four minutes) — fixed: `ssh_reachable`/`ssh_hint`, IAP by default (`GCP_SSH_VIA_IAP=1`, `GCP_TAGS=http,https`), the live instance's `ssh` tag removed; `TestGCPSSHUnreachableIsNamed` red without either fix (two mutations) and green with them; bug record by the `bugs` agent (its path in the change entry) — found in: `deploy/gcp/install.sh --version 1.36.0`
- [x] Found in the same run: `deploy/gcp/gcp.env` was invisible to git (`.gitignore` `*.env`) — renamed `vm.conf` (D18); `git check-ignore` now exit 1 — found in: the Phase 1 commit's `git status`
- [x] Found in the retry of `install.sh --version 1.36.0` (over IAP: apt, the clone at v1.36.0, `styrman-ctl install --yes`, Docker CE 29.7.2 all succeeded): `host-setup.sh` step 2 died `installing Kata 3.31.0` → `curl: (22) … 404` — Kata publishes `kata-static-3.31.0-amd64.tar.zst` and no `.sha256sum` any more; the NUC never hits the step (`[[ ! -x /opt/kata/bin/kata-runtime ]]`). Fixed in the SHARED path (D14): `.tar.zst`, `tar --zstd`, `zstd` installed when absent, the digest from GitHub's release-asset `digest` (pinned for 3.31.0 to `68c2786a…2b0b`, matched against a same-day download; other versions read the API; fail-closed unchanged); parser positive/negative controls printed the digest / empty; `bash -n` + shellcheck exit 0; BUG-070 by the `bugs` agent — found in: `deploy/gcp/install.sh --version 1.36.0` (second run)
- [-] `deploy/gcp/install.sh --version 1.36.0` — verify: its own step 7 plus `deploy/gcp/vm.sh status`, who: AI — expect: `302 https://auth.gcp.styrman.xyz/…`, status exit 0 — superseded by the next box (2026-09-21): v1.36.0's `host-setup.sh` cannot install Kata on a fresh host (BUG-070); the fix reaches the VM only in the next release
- [x] Operator: release the fixes (`/preflight`, next MINOR — new `deploy/gcp` surface), then `deploy/gcp/install.sh` (defaults to the checkout's newest tag) — verify: its own step 7 plus `deploy/gcp/vm.sh status`, who: both (release: user; install + checks: AI) — expect: `302 https://auth.gcp.styrman.xyz/…`, status exit 0 — evidence: operator's "Approved go go go" (2026-09-22) → v1.37.0 signed, verified (`testing/2026-09-22-verify-v1.37.0-release{,-round-2}.md`) and pushed at 2c15ea4; the NUC's updater deployed it in 60 s; `install.sh` exit 0 on the GCP host after BUG-072's fix; `https://crew.gcp.styrman.xyz/` → `302` to `auth.gcp.styrman.xyz`; `vm.sh status` exit 0 after BUG-073's fix — the fresh-host Kata path exercised for real (release entry's Deploy section), 2026-09-22
- [x] Found by the code review (`reviews/2026-09-21-review-gcp-compute-engine-target.md`, high): `install.sh`'s remote install line read the token through `${STYRMAN_GH_TOKEN_FILE}`, a variable only the separate git call's env line sets — `GH_TOKEN` would always have been empty on the VM. Cause: the earlier must-fail mutation's inverse `sed` restore rewrote this line too (BUG-071). Fixed: the literal staged path; `TestGCPInstallPlan` asserts the install line names it and never the variable (red on the reviewed line, green after) — found in: the code review
- [x] Found by the security review (`reviews/2026-09-21-security-gcp-compute-engine-target.md`; one high, three medium, three low — its Outcome section): the token value on the VM's `sudo env` line (fixed in the SHARED path: `styrman-ctl` exports it from `--gh-token-file` before `host-setup.sh`, D20; the GCP line passes the path only), project-wide ssh keys admitted (`block-project-ssh-keys=TRUE`, D21, applied live), the "no public 22" claim verified by the effective-firewalls listing, the tag signature verified on the VM before install (D22); three tests red before, green after — found in: the security review
- [x] Learnings for the local host: every defect, workaround or simplification the bring-up exposed in the shared path, applied to it (or listed with why not) — verify: the change entry's "Learnings for the local host" section exists, who: AI — expect: the section, with at least "none" written deliberately — evidence: four learnings in the Phase 1 change entry (the fourth: D20, the token export in `styrman-ctl`); (1) `host-setup.sh`'s Kata step was dead for any fresh host — fixed in the shared script (BUG-070); (2) an ssh-based check names the transport's failure first (BUG-069) — `styrman-ctl` has no remote checks, nothing to apply; (3) `host-setup.sh` now passes `shellcheck -S warning` — added to `verify-commit.sh`'s shellcheck list? NO: left as is, because that list is the gate's and widening it is a separate decision (Follow-up). 2026-09-21
- [x] E2E criterion 2 (delete refused) and 4 (NUC untouched) — verify: the commands in Verification, who: AI — expect: exit 2 then RUNNING; NUC status unchanged, clone clean — evidence: `vm.sh delete` without `--confirm` → exit 2, "nothing changed", and `vm.sh status` exit 0 straight after; the NUC healthy on v1.37.0 at sha 2c15ea4 with its 2 project dirs intact and `crew.styrman.xyz` answering 302 (it was never touched by this work; it upgraded through its own updater), 2026-09-22
- [x] Operator: add the redirect URI `https://auth.gcp.styrman.xyz/oauth2/google/authorization-code-callback` to the Google OAuth client, then E2E criterion 3 — who: user — expect: `sh-agent v1.6.0` in a project terminal on the GCP host — evidence: the operator added the URI and signed in (incognito, see BUG-074), created a project; server-side on the VM the microVM `styrman-3761476d-…` runs `styrman-runtime:1.37.0` with its home seeded and `sh-agent version` inside it prints `sh-agent v1.6.0`, 2026-09-22
- [x] Checkpoint — expect: criteria 1, 2, 4 met by AI, 3 handed to the operator — evidence: the release entry `changes/2026-09-22-v1.37.0-release.md` Deploy section carries every command and its result for both hosts (it replaced the two planned logs, which would have duplicated it); `testing/2026-09-22-verify-v1.37.0-release-round-2.md` is the Tier 1 + shellcheck run on the tagged commit, 2026-09-22

### Phase 3 — docs and records (the deliverable the operator asked for)
- [x] ADR-075 appended to `.agents/docs/architecture/DECISIONS.md` (D1: a Compute Engine VM on the unchanged host path; the Rejected rows) — verify: `grep -c '^## ADR-075' .agents/docs/architecture/DECISIONS.md`, who: AI (`changelog` agent, then read against the plan) — expect: 1 — evidence: 1 (line 2741), read against D1–D22, 2026-09-21
- [x] `deploy/README.md` gains a "GCP" section pointing at `deploy/gcp/` and `gcp.md`; `README.md` line 241's host list names the GCP path; `AGENTS.md` "Deploy" names three targets and the `deploy/gcp/` scripts — verify: `grep -n 'deploy/gcp' deploy/README.md README.md AGENTS.md`, who: AI (`documentation` agent, then read against the diff) — expect: at least one hit in each of the three files — docs: these three files — evidence: `grep -c` = deploy/README.md:1; README.md:1; AGENTS.md:2, 2026-09-21
- [x] `.agents/docs/testing/styrman/README.md` Tier 1 names `internal/config/gcp_scripts_test.go` and its test count — verify: `grep -n 'gcp_scripts_test' .agents/docs/testing/styrman/README.md`, who: AI (`documentation`) — expect: 1 hit with the count `go test ./internal/config/ -run TestGCP -v | grep -c '^=== RUN'` prints — evidence: 1 hit, count 11, 2026-09-21
- [-] `.agents/docs/specs/styrman/versioning.md`'s gate script list names `deploy/gcp/*.sh` — dropped: `versioning.md` names no script list and no shellcheck at all (the `documentation` agent checked, 2026-09-21); the gate's own header comment at `deploy/verify-commit.sh` lines 165–171 is the description, and it changed in the same edit
- [x] `/review --docs` over the whole range; findings to `documentation` — who: AI — expect: the review report path under `reviews/` — evidence: `reviews/2026-09-21-review-e5bd830-docs.md`, APPROVE WITH NITS, two low findings (an `AGENTS.md` Setup Commands block; the third restatement of the bring-up token line), both fixed by the main agent in the follow-up commit, 2026-09-21
- [x] Checkpoint — expect: every `grep` above at its expect; `changes/2026-09-21-gcp-compute-engine-target.md` written; plan closed (every box `[x]` or `[-]`), ROADMAP "Now" cleared — evidence: docs and records in e5bd830, 9a0e962 and the v1.37.0 release commit 2c15ea4; the plan is closed by this checkpoint and ROADMAP "Now" is cleared in the same commit, 2026-09-22

## Risks

Assumptions:
- N4 supports nested virtualization (Google's overview excludes E2, Arm,
  memory-optimized, AMD-except-N4D; N4 is Intel) — checked by the `create`
  itself: `--enable-nested-virtualization` on an unsupported family is
  refused by the API, and `host-setup.sh` refuses a host without `/dev/kvm`.
  Fallback knob: `GCP_MACHINE_TYPE=c4-standard-8`.
- The Debian 13 GCE image boots with KVM available to the guest once nested
  virt is on — checked by the Phase 2 "Nested-virt gate" task right after
  `create`, and again by `install.sh` step 1b before anything is staged.
- The NUC's GitHub token reads `kesokaj/styrman-crew` as well as the harness
  repo — checked by the remote clone (step 4) failing loudly if not; the
  `gh auth token` of this machine is the fallback, chosen by the operator.
- The Cloudflare token's zone scope covers `*.gcp.styrman.xyz` (a sub-name of
  the same zone) — checked by the ingress's `syncDNS` on first start; its
  failure is in `docker logs styrman-caddy`.
- Docker Hub's anonymous pull budget from the VM's static IP suffices for one
  runtime-image build (three HEADs + pulls) — BUG-056; a 429 is retried by
  re-running `install.sh` an hour later, nothing else.
- Cost: `n4-standard-8` + 200 GB Hyperdisk Balanced in europe-north2 is
  roughly USD 250–300/month while running; `vm.sh stop` leaves only the disk
  (~USD 20/month). Documented in gcp.md; the operator decides when to stop.

## Decisions

D1 — 2026-09-21 — Target is a Compute Engine VM running the unchanged host path (`host-setup.sh` → `styrman-ctl install`), not a new deploy mechanism — by: the request ("install this in gcp") + R11 naming a GCP VM with nested virtualization; the ADR-075 to write.
D2 — 2026-09-21 — Project `driven-eft-h4it`, zone `europe-north2-a`, `n4-standard-8`, 200 GB hyperdisk-balanced, no service account, public 80/443 + key-only 22 via existing tags, `BASE_DOMAIN=gcp.styrman.xyz` (app `crew.gcp.styrman.xyz`), auto-updater enabled — by: the scope table of 2026-09-21, accepted by the operator's reply ("This is just so we have a deploy process for gcp that I can use later") with no row struck.
D3 — 2026-09-21 — Provisioning is a gcloud-driven bash script with a committed non-secret `gcp.env`, in `styrman-ctl`'s `run()`/`--dry-run` style — by: implementer (one VM; Terraform not installed; the harness reserves it for more than one service).
D4 — 2026-09-21 — The GCP host's secrets are three private files OUTSIDE the repo (`~/.config/styrman/gcp*`), handed to `install.sh` by path; the scripts never derive them from the NUC — the derivation is a one-time session command recorded in the change entry — by: implementer (keeps the reusable process free of NUC assumptions; a secret-shaped path in the repo stops a commit).
D5 — 2026-09-21 — The instance is disposable: `delete` removes instance, boot disk and static IP; no snapshot, no backup — by: the operator's words ("only temporary"); durability is git + the scripts, as on the NUC (`deploy.md` "Durability").
D6 — 2026-09-21 — `install.sh` clones with a token-reading git credential helper (the `styrman-ctl` `setup_auto_update` pattern) into `~/styrman-crew` on the VM, since `styrman-ctl install` refuses to run from the updater's `/opt/styrman` clone — by: implementer, `styrman-ctl` line 442.
D7 — 2026-09-21 — `install.sh`'s clone hands the token-file path to the credential helper through `STYRMAN_GH_TOKEN_FILE` on the env line of the one git call, never interpolated into the `-c` snippet; helper scoped to the origin host, inherited helpers cleared — supersedes the first Interfaces draft — by: cold read round 1, `reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target.md` finding 1 (the shape `styrman-ctl` lines 568–571 adopted after the 2026-09-16 security review).
D8 — 2026-09-21 — `install.sh` hard-stops on a VM without `/dev/kvm` (step 1b) and Phase 2 runs the same check as its own gating task, because `host-setup.sh` only warns on that condition — by: cold read round 1, finding 2.
D9 — 2026-09-21 — `status` has one output shape (no `--json`); `stop`/`start` stay because D5 and the Cost risk name stopping the VM as the way to pay for the disk only — by: cold read round 1, Scope; implementer.
D10 — 2026-09-21 — Shielded VM (secure boot, vTPM, integrity monitoring) on, because the Debian 13 GCE image supports it and it costs nothing; no styrman code depends on it — by: implementer; cold read round 2, Scope.
D11 — 2026-09-21 — `--yes` on `vm.sh` gates only `create`'s cost confirmation; no terminal and no `--yes` refuses; `delete` stays on `--confirm <name>` — by: cold read round 2, finding 2.
D12 — 2026-09-21 — The five docs deliverables and ADR-075 are Phase 3 tasks with their own verify clauses, not left to the Definition of Done alone — by: cold read round 2, finding 1.
D13 — 2026-09-21 — `install.sh` takes one secrets directory (`~/.config/styrman/gcp/{env,gh-token,allowed_signers}`, default) instead of three file flags, so the whole GCP procedure is two bare commands: `vm.sh create`, `install.sh` — supersedes the three-flag Interfaces draft — by: the operator's words "make sure you make it simple do deploy in gcp" (2026-09-21).
D14 — 2026-09-21 — What the GCP bring-up teaches that also holds for the NUC is applied to the shared path in the same change, never kept GCP-only — supersedes the Scope Out bullet that froze the shared scripts — by: the operator's words "take your learnings back to local if any" (2026-09-21).
D15 — 2026-09-21 — `vm.sh status` exit 3 for an existing, non-running instance, and `install.sh` names `vm.sh start` for it; `STYRMAN_HOSTNAME` documented as the optional seventh `env` key; `apt-get update` before the remote install — by: cold read round 3, `reviews/2026-09-21-plan-cold-read-gcp-compute-engine-target-round-3.md`, three low findings.
D16 — 2026-09-21 — ssh and scp to the VM go through IAP TCP forwarding by default (`GCP_SSH_VIA_IAP=1`, the project's existing `allow-iap` rule) and the VM carries no public port 22 (`GCP_TAGS=http,https`); the public path stays available as `GCP_SSH_VIA_IAP=0` plus the `ssh` tag — supersedes the Scope Out bullet that deferred IAP — by: implementer, on the bring-up's evidence (the NUC's network dropped port 22 to the new VM within a minute; scanners were on it within four).
D17 — 2026-09-21 — Every remote diagnosis starts with a reachability probe and names the transport's failure first (`status`: "unreachable", exit 1; `install.sh`: "ssh … failed" before the KVM gate) — by: implementer, the bring-up's misdiagnosis (bug record).
D18 — 2026-09-21 — The defaults file is `deploy/gcp/vm.conf`, not `gcp.env`: `.gitignore` ignores `*.env`, and a `.env`-shaped name is read as a secret — by: implementer (`git status` after Phase 1 did not list it).
D19 — 2026-09-21 — `host-setup.sh` takes Kata's checksum from GitHub's release-asset digest (pinned constant for the pinned version, the release API for others), because Kata stopped publishing a `.sha256sum` sidecar; the fail-closed rule stays — by: implementer, BUG-070; the operator's "take your learnings back to local" (D14).
D20 — 2026-09-21 — `styrman-ctl install` exports `GH_TOKEN` from `--gh-token-file` in its own process before `host-setup.sh`; no token value is ever on a command line (sudo logs argv) — the shared path, so the NUC's documented break-glass `sudo GH_TOKEN=$(gh auth token) bash host-setup.sh` is superseded by `styrman-ctl install --gh-token-file` (docs, Phase 3) — by: security review finding 1; the operator's D14.
D21 — 2026-09-21 — `create` sets `block-project-ssh-keys=TRUE` with `enable-oslogin=FALSE` — by: security review finding 3.
D22 — 2026-09-21 — `install.sh` verifies the release tag's SSH signature on the VM against the staged `allowed_signers` before `styrman-ctl install` — by: security review finding 7 (low, taken: it is the updater's rule, ADR-050/051).
Open: none.
Rejected: Cloud Run / GKE Autopilot — no KVM in either; styrman needs `/dev/kvm` for Kata (host-setup.sh line 37).
Rejected: Terraform — one VM, one address; a state location and a binary this box lacks, for no second consumer. Revisit if the GCP footprint grows past one service.
Rejected: E2 machine types (cheaper) — Google excludes E2 from nested virtualization.
Rejected: an attached service account with logging/monitoring scopes — a LAN-mode project reaches the metadata server; with no SA there is no token to read.
Rejected: reusing the reserved `headscale-static` address — not this project's; a name collision with someone else's plan.
