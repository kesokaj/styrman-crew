---
owner: security
date: 2026-09-21
---
# Security review — gcp-compute-engine-target   (2026-09-21)

Range reviewed: live working tree vs HEAD (44a88e2) — new `deploy/gcp/{vm.conf,lib.sh,vm.sh,install.sh}`,
new `internal/config/gcp_scripts_test.go`, modified `deploy/host-setup.sh` (Kata download/verify) and
`deploy/verify-commit.sh` (lint globs). Blast radius read: `deploy/styrman-ctl` (`private_file`, the
credential helper at 560–591, the image build site at 467–477, `install` at 451–452),
`deploy/host-setup.sh` step 7 (268–290), `.agents/docs/deploy/gcp.md`.

**The tree moved during this review.** `deploy/gcp/install.sh` and `internal/config/gcp_scripts_test.go`
were rewritten at 22:22:53 while the review was in flight. Findings below are against the content as
of that write: `install.sh` md5 `31cc14800e23b93a655fbf14ebdd4c7b`, `gcp_scripts_test.go` md5
`4ee9641a71b92d31aac0f091c506fa08`. The edit changed the exact line this review's worst finding is
about (`install.sh:153`), and changed it in the direction that **arms** the weakness — see findings 1
and 2. Anything written after 22:22:53 is unreviewed.

## Status: COMPLETE

## Verdict: FINDINGS

The sharp one is `deploy/gcp/install.sh:153`: the GitHub token's **value** is interpolated into the
remote `sudo env GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")"` argv on the VM. sudo writes the
command line it ran to authpriv (`/var/log/auth.log` / the journal on Debian 13), so the token that
reads two private repositories — and that becomes the auto-updater's `/etc/styrman/gh-token` — is left
in cleartext in a persistent log on the VM, surviving the very trap whose job is to leave no secret
behind; the argv is also world-readable through `/proc/<pid>/cmdline` for the 10–20 minutes the
install runs. Until 22:22:53 today the line named an environment variable that was never exported, so
`GH_TOKEN` came out empty and the exposure was latent (and the bring-up died fail-closed in
`host-setup.sh` step 7). That empty-token defect has now been fixed by inlining the token's path — the
repair makes the leak live, and `internal/config/gcp_scripts_test.go:241` now asserts the leaking
shape, so it will survive future review rounds unless the test moves with it. Everything else is
smaller: the VM accepts project-wide SSH keys, and the documented "no public port 22" is asserted
nowhere. Input handling into gcloud argv and the ssh command strings is genuinely careful — the
`GCP_*` regexes, `sq()`, `--`, and the dry-run secret sentinel in the Go test are right.

## Findings

- **[high / confidence: high]** `deploy/gcp/install.sh:153` — the token VALUE crosses an argv on the
  VM: `… && sudo env GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")" ./deploy/styrman-ctl install …`.
  The command substitution is performed by the VM's login shell, so the plaintext token becomes an
  argument of `sudo`/`env`. Two concrete consequences: (a) sudo's own logging records
  `COMMAND=/usr/bin/env GH_TOKEN=<token> …` to authpriv on every invocation — a persistent, plaintext
  copy of the token on the VM's disk (and in any log shipper), which `cleanup()`'s
  `rm -rf ~/.styrman-install` (line 120) does not touch; (b) `/proc/<pid>/cmdline` is world-readable on
  stock Debian (no `hidepid`), so any other local account on the VM can read it while the install runs.
  This is the one place in the change that breaks the repo's own rule — `deploy/styrman-ctl:560`,
  "Secrets travel through root-only temp files, never argv or the plan" — and `install.sh:21-22` states
  the weaker claim ("never … in a *local* argv"), which is true and misleading; the describing comment
  should move with the fix. Remediation, no new dependency: drop `env GH_TOKEN=…` and rely on
  `--gh-token-file`, already passed on the same line and preferred by `styrman-ctl:471`, teaching
  `styrman-ctl` to export `GH_TOKEN` from that file itself before
  `run bash "${REPO}/deploy/host-setup.sh"` (`styrman-ctl:452`) — `host-setup.sh:278` is the only
  consumer that needs the variable. If it must be set from the client side, keep the value out of argv
  by letting the privileged shell read it:
  `sudo sh -c 'GH_TOKEN=$(cat "$1") exec ./deploy/styrman-ctl install …' sh "$HOME/.styrman-install/gh-token"`
  — only the PATH then appears in argv and in sudo's log. Coreutils `cat` plus the login shell;
  nothing new to install.

- **[medium / confidence: high]** `internal/config/gcp_scripts_test.go:241` — the test added at
  22:22:53 asserts the leaking shape literally:
  `strings.Contains(install, "GH_TOKEN=\"$(cat \"$HOME/.styrman-install/gh-token\")\"")`, with a
  comment citing "review 2026-09-21, high: GH_TOKEN was always empty on the VM". The defect it fixes
  was real — before that write, `install.sh:153` referenced `${STYRMAN_GH_TOKEN_FILE}`, which only
  `GIT_ENV` (line 139) exports and only onto the clone/fetch lines (143, 146); on the VM the variable
  was unset, `$(cat "")` failed (verified: `cat: '': No such file or directory`, rc 1), `GH_TOKEN` was
  empty, and `host-setup.sh:278-285` — which runs first, at `styrman-ctl:452`, inheriting only the
  environment — found neither `GH_TOKEN` nor `/etc/styrman/gh-token` on a fresh VM and exited 1. But
  the chosen repair trades a fail-closed break for a live secret-in-log, and the new assertion pins it:
  any correct fix now has to delete or rewrite this test, and a reviewer who only reads the test will
  read the exposure as a requirement. Remediation: replace the assertion with the property actually
  wanted — the install line names the staged token FILE and no `${…}` the line does not itself export,
  and the token VALUE never appears in an argv (assert the absence of `sudo env GH_TOKEN=` rather than
  its presence).

- **[medium / confidence: medium]** `deploy/gcp/vm.sh:78` — `--metadata=enable-oslogin=FALSE` with no
  `block-project-ssh-keys=TRUE`. The VM therefore honours **project-wide** SSH key metadata: every
  principal that can write project metadata (`compute.instances.setMetadata`, `compute.projectAdmin`,
  Editor/Owner), and every key already in the project's metadata from an unrelated VM, has a login on
  a host whose operator account is in the `docker` group (root-equivalent) and whose disk holds every
  tenant's home, workspace and the credential broker's store. OS Login would put that behind IAM
  (`roles/compute.osLogin`) with per-login audit entries and central revocation. Fix:
  `--metadata=enable-oslogin=TRUE` (IAP already covers the transport; `--tunnel-through-iap` is
  unaffected), or — if the stable POSIX login name `--user` relies on is why OS Login was disabled —
  at minimum extend the same flag to `--metadata=enable-oslogin=FALSE,block-project-ssh-keys=TRUE`;
  `gcloud compute ssh` then writes the operator's key to *instance* metadata and the flow is unchanged.

- **[medium / confidence: medium]** `deploy/gcp/vm.conf:18-20` and `.agents/docs/deploy/gcp.md`
  ("**no public port 22**") — nothing in the change asserts that claim, and omitting the `ssh` target
  tag does not establish it: a firewall rule with **no** target tags applies to every instance in the
  network, and `default-allow-ssh` (auto-created with a default VPC) is exactly such a rule allowing
  `0.0.0.0/0:22`. The plan's own bring-up note — scanners trying root logins on the new VM within a
  minute — is evidence the port was in fact open. A false "no public 22" in the deploy doc is worse
  than none, because it is what the next session will trust instead of looking. Fix (gcloud is already
  a prerequisite, no dependency): after `instances create`, list the network's enabled rules that allow
  22 and carry no target tag —
  `gcloud compute firewall-rules list --filter='disabled=false AND allowed[].ports~22' --format='value(name,network,targetTags.list())'`
  — and warn loudly (or refuse) when one matches the instance's network; and name the
  `default-allow-ssh` deletion in the gcp.md prerequisites.

- **[low / confidence: high]** `deploy/host-setup.sh:78-88` — for any `KATA_VERSION` other than the
  pinned `3.31.0`, and without `KATA_SHA256`, the expected digest is read from
  `api.github.com/repos/.../releases/tags/${KATA_VERSION}`, i.e. from the **same origin** that serves
  the tarball. The comment calls it "the publisher's SHA256"; GitHub's `digest` field is computed by
  GitHub over whatever asset is stored, so it detects corruption in transit (which TLS already covers)
  and nothing else — an actor who can replace the release asset supplies both halves, and the tarball
  is extracted as root into `/`. The unauthenticated API call is also rate-limited per IP (60/hour),
  which yields an empty digest and a fail-closed FATAL — correct, and worth keeping. Fix: make the
  pinned-constant path the only accepted one (die when an unpinned `KATA_VERSION` is requested without
  `KATA_SHA256`, the same shape as the existing M15 block), and shape-check the value before comparing:
  `[[ "${EXPECT}" =~ ^[0-9a-f]{64}$ ]] || die`.

- **[low / confidence: high]** `deploy/gcp/lib.sh:71` — `vm_ssh` passes `--command="$*"`, flattening
  its arguments into one string that the VM's login shell re-parses; quoting and word boundaries are
  lost (`vm.sh ssh -- rm 'a b'` removes `a` and `b`). Nothing exploitable today — `install.sh` always
  hands it a single, fully pre-quoted string and `vm.sh ssh` is operator-driven — but it is a seam that
  silently converts a future caller's data into remote shell words. Fix: accept exactly one argument
  (`[ $# -eq 1 ] || die`), or quote each argument with the `sq()` already written in `install.sh:137`
  before joining.

- **[low / confidence: medium]** `deploy/gcp/install.sh:141-147` — the tag the VM checks out is never
  verified, although `allowed_signers` (the release-trust root the updater will use) is staged on the
  same run and git is installed two steps earlier. The first install therefore trusts GitHub plus TLS
  alone, while every later release the updater deploys is signature-checked — an asymmetry that makes
  the initial state the weakest link. Fix, no dependency:
  `git -c gpg.ssh.allowedSignersFile="$HOME/.styrman-install/allowed_signers" verify-tag "v${VERSION}"`
  after the fetch/clone, refusing on non-zero.

## Checked

- **Secrets:** no credential value is committed; `vm.conf` holds only non-secret defaults (project ID,
  zone, machine type — identifiers, not credentials), and the three secret files live outside the repo
  at `~/.config/styrman/gcp` with symlink/owner/mode-600/parent-700 checks (`install.sh:57-72`) that
  are stricter than `styrman-ctl:507-517`. One real leak path found and filed above (sudo's log,
  `install.sh:153`). `--dry-run` output was traced: `run()` echoes the command *before* the VM expands
  it, so the plan shows `$(cat …)` literally and only file PATHS reach the output; `gcpRun`
  (`gcp_scripts_test.go:90`) fails any test whose output contains a `SENTINEL-` value — a good guard,
  and it covers the diagnostics too (`install.sh:170` prints only the curl status line). `env_get`
  values (`install.sh:74-81`) never reach a message; `BASE_DOMAIN`/`STYRMAN_HOSTNAME` are regex-checked
  before reaching the verify URL.
- **Injection:** every value that reaches gcloud argv is validated in `load_gcp_env` (`lib.sh:32-46`) —
  GCP resource-name, lowercase-token, tag-list, `^[0-9]+GB$` and `^[01]$` regexes — and reaches gcloud
  as separate array elements; `printf -v` is guarded by `^GCP_[A-Z_]+$`, so a vm.conf line cannot write
  `PATH` or an array the script builds afterwards, and environment values win but face the same regexes
  (`TestGCPVMRejectsBadNames` proves `GCP_INSTANCE=x;rm` reaches no tool). In the remote command
  strings `${VERSION}`, `${USER_NAME}` and `${ORIGIN}` are each regex-pinned before use
  (`install.sh:50-55`, `:89`), `clone` uses `--` before the URL, and `sq()` (`install.sh:137`) is a
  correct single-quote escape (`'` → `'\''`) applied to the one fragment that needs it; the credential
  helper receives the token path through the environment, never interpolated into the snippet its shell
  parses, and `TestGCPInstallCloneHelperShape` asserts that. `host-setup.sh:95` compares the digest with
  a **quoted** right-hand side inside `[[ ]]`, which is a literal comparison rather than a glob —
  correct as written, and worth not "simplifying" later.
- **Authz/authn:** IAP tunnelling for ssh/scp (`lib.sh:46`), the caller's own IAM decides; `delete` is
  gated by a typed instance name or `--confirm <name>` and `create` by a TTY prompt or `--yes`, both
  covered by tests. Two gaps filed: project-wide SSH key metadata (OS Login disabled) and the
  unasserted "no public 22". `gcloud compute ssh --quiet` will also generate a **passphraseless**
  `~/.ssh/google_compute_engine` key on the operator's machine on first use and push it to metadata —
  standard gcloud behaviour, noted rather than filed.
- **Crypto:** nothing hand-rolled. No primitive, protocol, token or session format is implemented or
  composed here; the change consumes `sha256sum` (coreutils), git's ssh-signature machinery and TLS as
  designed.
- **Dependencies:** no new module, package or binary — `go.mod`/`go.sum` untouched; the scripts use
  gcloud, git, curl, python3, tar and zstd, all already required (zstd is installed from Debian's
  signed archive by `host-setup.sh:98`). `govulncheck` is present (`~/.go/bin/govulncheck`) but was not
  run: the project runs it through `deploy/vulncheck.sh` inside a pinned builder image, and no Go
  dependency changed in this diff, so the result would be HEAD's. `npm audit` is not applicable — no
  `web/` change in range. `shellcheck -S warning deploy/gcp/*.sh` run bare here: **exit 0, clean**;
  `deploy/verify-commit.sh:168-171` now wires the same glob into the gate.
- **GCP surface:** strong defaults otherwise — `--no-service-account --no-scopes` (no metadata token
  for a LAN-mode project to reach), all three Shielded VM flags, a reserved static IP, IAP instead of a
  public 22, secrets delivered as files over scp rather than as instance metadata or env arguments, and
  the staged directory created `install -d -m 700` *before* the scp, so the pre-`chmod` window is not
  traversable by another local account. No IAM binding, service-account key file or
  `--allow-unauthenticated` appears anywhere in the change.

## Not examined

- Anything written to `deploy/gcp/install.sh` or `internal/config/gcp_scripts_test.go` after
  2026-09-21 22:22:53 (the mid-review rewrite); the md5s above fix what was read.
- The live secret files (`~/.config/styrman/gcp/*`, `/opt/styrman/deploy/.env`, `/etc/styrman/*`) —
  out of scope by the dispatch, reasoned about from the code only. No gcloud command was run, so
  nothing here is verified against the live project: the port-22 and OS Login findings are about what
  the scripts and docs guarantee, not about the current state of `driven-eft-h4it`.
- `deploy/styrman-ctl` beyond the sections named in the Range line (its `install`, `update` and
  firewall paths are pre-existing and outside this diff), and `image/Dockerfile`'s use of the
  `gh_token` build secret (ADR-074, reviewed in the v1.36.0 gate).
- The plan, the two new bug records (BUG-069, BUG-070) and the three plan cold-read reviews were not
  read as sources of truth; every finding is grounded in the code.
- Not run: `govulncheck` / `deploy/vulncheck.sh` (no Go dependency change in range), `npm audit` (no
  frontend change), `go test ./internal/config/ -run TestGCP` (the tests were read, not executed — the
  dispatch asked for no changes and the verifier owns execution), `deploy/firewall/test.sh` (root, out
  of band).

## Outcome

1. High, `install.sh:153` (token value in a `sudo env GH_TOKEN=…` argv on the VM): **fixed**. The GCP line passes the token by path only (`--gh-token-file`); `deploy/styrman-ctl` `cmd_install` now exports `GH_TOKEN` from `--gh-token-file` inside its own process before `host-setup.sh` (environment only, never argv) — the shared path gains the fix, and the documented NUC break-glass line (`sudo GH_TOKEN=$(gh auth token) bash host-setup.sh`) is superseded by `styrman-ctl install --gh-token-file` (docs task, Phase 3). Tests: `TestGCPInstallPlan` requires `--gh-token-file "$HOME/.styrman-install/gh-token"` on the install line and refuses any `GH_TOKEN=` there; `TestStyrmanCtlInstallExportsTokenForHostSetup` requires the export log line and refuses the token's content or an `env GH_TOKEN` on any planned command. All three were red before the fix.
2. Medium, the test locking the leaking shape: **fixed** with 1 (the assertion now forbids it).
3. Medium, `enable-oslogin=FALSE` without `block-project-ssh-keys=TRUE`: **fixed** — `create` sets both; the live instance got `block-project-ssh-keys=TRUE` by `gcloud compute instances add-metadata` and `vm.sh ssh` still logs in over IAP (gcloud moved the key to instance metadata); `TestGCPVMCreatePlan` asserts the flag.
4. Medium, "no public 22" asserted by nothing: **verified** on the live instance with `gcloud compute instances network-interfaces get-effective-firewalls` — the only `0.0.0.0/0` allows are `allow-http` (80), `allow-https` (443) and `allow-icmp`; the command is now in `gcp.md`'s Verify section. No `default-allow-ssh` rule exists in this project (the rules are listed in the plan's Surfaces).
5. Low, Kata digest from the same origin as the tarball for unpinned versions: **accepted/rebutted** — the pinned version (the one every host installs) checks against a constant matched to a same-day download; for an operator-chosen other version, GitHub's asset digest is what the publisher publishes, and the fail-closed rule stands (`KATA_SHA256` pins it offline).
6. Low, `vm_ssh --command="$*"` flattening: **accepted** — every caller passes one string; documented on the function.
7. Low, no signature verification of the installed tag: **fixed** — `install.sh` runs `git -c gpg.ssh.allowedSignersFile=<staged> verify-tag v<VERSION>` before `styrman-ctl install`; `TestGCPInstallPlan` asserts the line and its order.
