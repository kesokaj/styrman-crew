---
owner: documentation
date: 2026-09-22
target: gcp
---
# Deploy — GCP (a Compute Engine VM on the unchanged host path)

styrman on GCP is the same host as the NUC (`remote.md`): one Debian 13
Compute Engine VM with nested virtualization, converged by
`deploy/host-setup.sh` and brought up by `deploy/styrman-ctl install` —
no Cloud Run, no GKE (neither offers `/dev/kvm`, which Kata needs), no
Terraform (one VM). Two scripts under `deploy/gcp/` wrap the GCP half so
the whole procedure is two commands. ADR-075; plan
`.agents/docs/plans/2026-09-21-gcp-compute-engine-target.md`.

The instance is disposable (operator, 2026-09-21: the GCP install is
temporary, the NUC stays primary): no backup layer, durability is git plus
these scripts, as on the NUC.

## Defaults (recorded so the next session does not ask)

| Item | Value | Where |
|---|---|---|
| Project | `daring-lioness-uwgw` | `deploy/gcp/vm.conf` `GCP_PROJECT` |
| Zone / region | `europe-north2-a` / `europe-north2` (subnet `europe-north2`) | `GCP_ZONE`, `GCP_SUBNET` |
| Machine | `n4-standard-8` (8 vCPU / 32 GB), 300 GB Hyperdisk Balanced, Debian 13 (`debian-13` family), Shielded VM | `GCP_MACHINE_TYPE`, `GCP_BOOT_DISK_*`, `GCP_IMAGE_*` |
| Service account | **none** (`--no-service-account --no-scopes`): the host needs no GCP API, and a LAN-mode project can reach the metadata server | `vm.sh create` |
| Public | yes for 80/443 via the project's `http`/`https` firewall tags; **no public port 22** — ssh goes through IAP (Identity-Aware Proxy TCP forwarding, the project's `allow-iap` rule) | `GCP_TAGS`, `GCP_SSH_VIA_IAP` |
| Static IP | reserved as `styrman-static` in the region; DNS points to it (via Cloudflare ingress sync or manual Cloud DNS records) | `GCP_ADDRESS` |
| Hostname | `BASE_DOMAIN=spgo.demo.altostrat.com`, `STYRMAN_HOSTNAME=crew` → app `https://crew.spgo.demo.altostrat.com`, sign-in `https://auth.spgo.demo.altostrat.com` | the `env` secrets file |
| Instance name | `styrman` | `GCP_INSTANCE` |

Every `GCP_*` key can be overridden from the environment for one run
(`GCP_MACHINE_TYPE=c4-standard-8 deploy/gcp/vm.sh create`). Nested
virtualization is refused by Google on E2, Arm, memory-optimized and every
AMD family except N4D — keep an Intel type.

Cost: roughly USD 250–300/month while running; `vm.sh stop` leaves only the
disk (about USD 20/month). Stop it when idle.

## Prerequisites (once)

1. `gcloud` installed and signed in (`gcloud auth login`; the `gcloud-setup`
   skill), with access to the project and the IAP tunnel role
   (`roles/iap.tunnelResourceAccessor`; a project owner has it). Set
   `GCP_SSH_VIA_IAP=0` and add `ssh` to `GCP_TAGS` only if you want the
   public path instead — during the first bring-up the operator's own
   network dropped port 22 to the new VM within a minute, while scanners
   were already trying root logins on it.
2. The secrets directory `~/.config/styrman/gcp/` (mode 700), three files,
   each mode 600 and owned by you — never inside the repository:
   - `env`: the five must-fill keys of `deploy/.env.example` for THIS host —
     `BASE_DOMAIN`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`,
     `ADMIN_EMAIL`, `STYRMAN_ALLOWED_EMAILS` — plus the optional
     `STYRMAN_HOSTNAME` and the optional `CLOUDFLARE_API_TOKEN`
     (`styrman-ctl install --env-file` writes the token to the host's `.env`
     when the file carries it — validated, an invalid value stops the
     install, never prompted for; from v1.39.0 until the release after it a
     token here was dropped silently).
     When `CLOUDFLARE_API_TOKEN` is unset, the ingress uses HTTP-01 ACME
     challenges (works for non-wildcard hostnames) and DNS records must be
     created manually (e.g. in Cloud DNS — see "Create DNS records" below).
     **Use a separate OAuth client and a
     separate Cloudflare token for this host**, the token scoped to the zone
     it actually needs. Copying the production host's credentials works —
     one zone token covers every sub-name, one OAuth client takes several
     redirect URIs — but it hands a disposable machine the keys to the
     production name: whoever takes this VM, or the secrets directory on
     your own machine, can repoint the production host's DNS and obtain a
     certificate for it (security review 2026-09-22). The same applies to
     `gh-token`: give this host its own, scoped to the repositories it
     reads.
   - `gh-token`: a fine-grained GitHub token that reads BOTH this repository
     (the VM's clone and its auto-updater) and `kesokaj/styrman-harness`
     (the runtime image's `sh-agent` download, ADR-074). The NUC's
     `/etc/styrman/gh-token` is such a token.
   - `allowed_signers`: one line, `<git email> <key type> <public key>` of
     the release signer (the NUC's `/etc/styrman/allowed_signers`).
3. On the Google OAuth client (console → APIs & Services → Credentials),
   add the redirect URI
   `https://auth.<BASE_DOMAIN>/oauth2/google/authorization-code-callback`.
   Until it is there, the stack runs but sign-in fails with
   `redirect_uri_mismatch`. This is the one step no script can do.

## Procedure

```bash
deploy/gcp/vm.sh create          # reserves the static IP, creates the VM (asks once: it bills while running; --yes skips), waits for ssh
deploy/gcp/install.sh            # clones this repo at its newest release tag on the VM, stages the three files,
                                 # runs `sudo styrman-ctl install --yes …` there (10–20 min), verifies, removes the staged files
```

### Create DNS records (Cloud DNS, no Cloudflare)

When `CLOUDFLARE_API_TOKEN` is unset in the secrets `env` file, the ingress
does not manage DNS. Create the A records manually after `vm.sh create`
reserves the static IP:

```bash
IP=$(gcloud compute addresses describe styrman-static --project=daring-lioness-uwgw --region=europe-north2 --format='value(address)')
gcloud dns record-sets create crew.spgo.demo.altostrat.com. --project=daring-lioness-uwgw --zone=spgo-demo-altostrat-com --type=A --ttl=300 --rrdatas="$IP"
gcloud dns record-sets create auth.spgo.demo.altostrat.com. --project=daring-lioness-uwgw --zone=spgo-demo-altostrat-com --type=A --ttl=300 --rrdatas="$IP"
```

Skip this when `CLOUDFLARE_API_TOKEN` is set — the ingress creates the
records itself.

`install.sh` also delivers the OPTIONAL keys in `env` — `STYRMAN_HOSTNAME`,
`STYRMAN_PUBLIC_DOMAIN`, `STYRMAN_PRIVATE_DOMAIN` — with `styrman-ctl edit`
after the install, because `styrman-ctl install --env-file` itself carries
only the five must-fill keys plus (since this change) `CLOUDFLARE_API_TOKEN`, and the host would otherwise keep its defaults
(BUG-072). It then reads the hostname back from the host and verifies that
URL, so a mismatch is reported rather than probed blindly.

`install.sh --version X.Y.Z` pins a release; `--user U` sets the VM login
(default: your local user name, which `gcloud compute ssh` uses);
`--secrets-dir D` points at another directory. Both scripts take
`--dry-run`, which prints every command that would change something as
`+ cmd` and runs none of them (reads such as `describe` still run, so the
plan is the real one). `deploy/gcp/lib.sh` stat calls are portable across
macOS and Linux (`stat -c` on Linux, `stat -f` on BSD/macOS), so `install.sh`
runs on either operator OS without issues.

`install.sh` refuses, before staging anything: a missing or non-running VM
(`vm.sh create` / `vm.sh start`), an unreachable VM (it prints gcloud's
`--troubleshoot` command; `vm.sh status` reports the same as "unreachable"
rather than "not installed"), a VM without `/dev/kvm` (nested
virtualization off or unsupported for the machine type — `host-setup.sh`
only warns on this), a loose or incomplete secrets file, a `--user` that is
not a login name.

Before anything runs from the checkout, `install.sh` verifies the release
tag's signature on the VM against the staged `allowed_signers`
(`git verify-tag`), the updater's rule applied to the first install too.
The token never appears on a command line: it travels as a file and
`styrman-ctl` exports it for `host-setup.sh` in its own process.

What runs on the VM is exactly the NUC's first bring-up (`remote.md`,
`specs/styrman/deploy.md` "Bring-up"): `host-setup.sh --user U`, the five
must-fill `.env` keys, the runtime image built with the token as a build secret, the
stack at the release version, then the auto-updater set up with the same
token and signer, so the VM follows every signed release tag on its own.

## Verify

```bash
deploy/gcp/vm.sh status                       # RUNNING + IP, then `styrman-ctl status` on the VM; exit 0 healthy
curl -sSI https://crew.spgo.demo.altostrat.com/ | head -3   # 302 to https://auth.spgo.demo.altostrat.com/…
deploy/gcp/vm.sh ssh -- 'sudo styrman-ctl status'   # server + caddy at the version, updater timer active
gcloud compute instances network-interfaces get-effective-firewalls styrman --zone europe-north2-a   # 0.0.0.0/0 allows: 80, 443 and ICMP only
```

`create` sets `block-project-ssh-keys=TRUE` (the project's metadata keys
get no shell on this host; `gcloud compute ssh` adds your key to the
instance instead) and `enable-oslogin=FALSE`.

Then in a browser: sign in with an allowed Google account, create a
project, open a terminal, `sh-agent version`. Firewall and harness checks
are the same as `remote.md` "Verify", run over `vm.sh ssh`.

## Claude via Vertex AI (optional)

On a GCP host, Claude Code can use Vertex AI instead of the direct
Anthropic API. In Settings → Credentials, add three custom variables:

| Variable | Value |
|----------|-------|
| `CLAUDE_CODE_USE_VERTEX` | `1` |
| `ANTHROPIC_VERTEX_PROJECT_ID` | `daring-lioness-uwgw` (or your project) |
| `CLOUD_ML_REGION` | `global` (or a specific region) |

Then sign in to claude via the Credentials page (click **sign in** next
to the grey **claude** row → boot setup VM → run `claude` → pick
"3rd-party platform" → "Google Vertex AI"). The broker harvests the
credentials and delivers them to every project.

## Update / rollback

The VM runs the auto-updater (`deploy/update/`, ADR-050/051): it deploys
signed release tags itself. Hand operations go over ssh exactly as on the
NUC: `deploy/gcp/vm.sh ssh -- 'sudo styrman-ctl update --redeploy'`, and
rollback per `specs/styrman/versioning.md` "Rollback". `install.sh` re-run
against an installed VM re-checks out the requested tag and re-runs
`styrman-ctl install`, which is idempotent.

## Stop, start, tear down

```bash
deploy/gcp/vm.sh stop                          # keeps the disk; every project home and workspace survives
deploy/gcp/vm.sh start
deploy/gcp/vm.sh delete --confirm styrman      # instance, boot disk and static IP — irreversible; without --confirm it refuses (exit 2)
```

Deleting leaves DNS records behind (the ingress creates, never
deletes Cloudflare records; manual Cloud DNS records also remain) and the OAuth redirect URI on the client. **Remove both by
hand as part of the teardown, not "if you feel like it".** The address is
released back to Google's pool, so the records now point at an address
someone else can be given: whoever gets it can serve content on your names,
obtain a certificate for them through HTTP-01 validation, and — because
`auth.<BASE_DOMAIN>` stays registered as a redirect URI on the OAuth client
— be handed authorization codes issued for that client. When the client is
shared with another host (it should not be: see the prerequisites), that
reaches the other host too.

```bash
# DNS records to remove (Cloudflare dashboard/API, or Cloud DNS if created manually):
#   <STYRMAN_HOSTNAME>.<BASE_DOMAIN>   A   <the released address>
#   auth.<BASE_DOMAIN>                 A   <the released address>
# For Cloud DNS:
#   gcloud dns record-sets delete crew.spgo.demo.altostrat.com. --project=daring-lioness-uwgw --zone=spgo-demo-altostrat-com --type=A
#   gcloud dns record-sets delete auth.spgo.demo.altostrat.com. --project=daring-lioness-uwgw --zone=spgo-demo-altostrat-com --type=A
dig +short <STYRMAN_HOSTNAME>.<BASE_DOMAIN> auth.<BASE_DOMAIN>   # both must return nothing when done
```

Then remove `https://auth.<BASE_DOMAIN>/oauth2/google/authorization-code-callback`
from the OAuth client (console → APIs & Services → Credentials).

## Tests

`internal/config/gcp_scripts_test.go` (11 tests) drives both scripts' `--dry-run`
plans against a fake `gcloud`/`git`/`curl` on PATH (Tier 1,
`go test ./internal/config/ -run TestGCP`); `deploy/verify-commit.sh` runs
`bash -n` and `shellcheck -S warning` over `deploy/gcp/*.sh`.
