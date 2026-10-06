---
owner: security
date: 2026-10-06
---
# Security review — release-gate-v1.42.4-ceb6108   (2026-10-06)

Range reviewed: v1.42.4..ceb6108

Seven commits, 26 paths, +654/-60, no binary paths (`git diff --numstat` shows no `-`). Code: `image/Dockerfile` (the harness RUN step), plus comment-only edits to `web/src/lib/api.ts` and `api.encoding.test.ts`. Everything else is trail and docs. I read every hunk of every path in full; for new files that is the whole file. I also read the committed `image/Dockerfile` end to end.

Blast radius followed:
- every runtime-image build site, checked for a `HARNESS_*` build-arg;
- `deploy/gcp/install.sh` and `deploy/styrman-ctl`, which stage and install a host's `allowed_signers`;
- the `sh-agent` bytes and embedded signer in every local runtime image (`styrman-runtime:1.38.0` through `:test`).

## Status: COMPLETE

## Verdict: FINDINGS

The shipped artifact is sound. The committed harness RUN step fails closed on each check, and every fix from the earlier review (`2026-10-06-security-harness-sha256-pin.md`) holds in the committed state. Dropping the old check removes the exposed signer `SHA256:5HpY…` from the image's trust path.

The worst problem is outside the code: the exposed key is still trusted in three places that this range's records either leave open or do not name.
1. **The GCP VM.** Its `/etc/styrman/allowed_signers` still lists `5HpY…` (BUG-094, open). Removing it is an operator step.
2. **Live project homes.** The live `sh-agent` v2.3.2 binary embeds `5HpY…`. Every live project's `sh-agent update` therefore trusts that key until this release reaches the project's home. BUG-094 does not name this list.
3. **The operator's local signers file.** `deploy/gcp/install.sh` stages `${SECRETS}/allowed_signers` from the operator's machine. That file almost certainly still holds the `5HpY…` line, and the next install or re-create of the VM puts it back. No record names this file.

None of these blocks the release to dave. Shipping this release is the fix for the second point. BUG-094 must stay open until the first and third are done.

## Findings

- **[high / confidence: medium] Carried, still open: the GCP VM's updater still accepts tags signed by the exposed key `SHA256:5HpY…`.**
  - Where: `.agents/docs/deploy/gcp.md:84-87`, BUG-094 ("Not yet done: remove the `5HpY...` line … (operator step)"), ADR-089 amendment 1.
  - The records are now correct: BUG-094 exists, ADR-089 says "exposed", and `gcp.md` and `versioning.md` say no host may list the key. That part of the earlier finding 1 holds.
  - The state on the VM is unchanged and unchecked. BUG-094 says "The GCP VM's list was NOT checked (no gcloud on dave)". Per BUG-093's Fix, the VM also does not yet list `2bCW…`. So it cannot take genuine releases, while a crew tag signed with the exposed key would verify there and be deployed as root by its auto-updater.
  - Remediation: remove the `5HpY…` line on the VM and add `2bCW…`. Confirm with `ssh-keygen -lf /etc/styrman/allowed_signers` on the VM. Until then, pause the VM's auto-updater (or stop the VM, which is disposable).

- **[medium / confidence: medium] Every live project's `sh-agent update` trusts the exposed key until this release reaches its home. BUG-094's list of places that trust the key leaves this one out.**
  - Where:
    - BUG-094's Prevention says "name every list that trusts it (each host's `allowed_signers`, the image's pins, GitHub)", but its Fix lists only host lists and GitHub.
    - The handoff `2026-10-06-0804-harness-v2.4.1-pinned.md` says "Live projects … cannot `sh-agent update` (newest release unsigned)".
    - ADR-090's Consequences say a v2.3.2 home "cannot `sh-agent update`".
  - Evidence: I ran `grep -a -c` for the exposed key's base64 against each image's `/styrman/bin/sh-agent`.
    - `styrman-runtime:1.42.4` (live, harness v2.3.2): 1 match.
    - `styrman-runtime:test` (v2.4.1): 0 matches.
  - Why it matters: v2.3.2 requires a signature on an update (ADR-090), and the only signer it carries is the exposed key. "Cannot update" is therefore true only for genuine releases. Suppose someone holds the exposed key and can also publish a release on `kesokaj/styrman-harness`. Harness ADR-053a lists who can: any machine logged in to GitHub, Actions, and the BUG-160 plaintext token. They could publish a release that v2.3.2 accepts. Any user who then runs `sh-agent update` gets that code in their project VM, next to their brokered git/gh/ssh/claude/agy/gcloud credentials.
  - The entrypoint never downgrades a home whose user moved it forward. A home already moved to such a higher version would therefore not be healed by this release.
  - Remediation:
    - Ship this release soon; its image is the fix for the image.
    - Add "the v2.3.2 `sh-agent` binary in the live image and in homes" to BUG-094's Fix.
    - Until projects have rebooted on the new image, tell users not to run `sh-agent update`.
    - After the release, look at any home whose `sh-agent version` is neither v2.4.1 nor v2.3.2.
  - Confidence is medium because two things are inferred, not run: that v2.3.2's update path uses this embedded key (from ADR-090's "v2.3.2 requires a signature"), and whether homes carry their own copy of `sh-agent`.

- **[medium / confidence: medium] `deploy/gcp/install.sh` puts the exposed key back from the operator's local secrets file, which no record names.**
  - Where: `deploy/gcp/install.sh:63`, `:117-119`, `:151`, `:156`; `deploy/styrman-ctl:580-588`, `:657`.
  - How the file flows:
    - `SIGNERS_FILE="${SECRETS}/allowed_signers"` on the operator's machine is copied to the VM with scp.
    - `install.sh` verifies the tag against that copy (`verify-tag`).
    - It then runs `styrman-ctl install --allowed-signers-file`. That accepts exactly one line and installs it as the VM's whole `/etc/styrman/allowed_signers`.
  - Why the file is probably stale: the VM was set up when the NUC's signer was `5HpY…`, and `styrman-ctl` takes only one line. The Mac key on the VM must therefore have been added by hand. So the local file very likely holds exactly the `5HpY…` line.
  - What the records say: BUG-094, ADR-089 amendment 1 and `gcp.md:84-87` name only the VM's installed list ("a VM set up before then still carries its line, which the operator removes").
  - Effects of a re-create or re-install (the documented lifecycle for this disposable VM):
    - It installs `5HpY…` as the VM's only trusted signer. That undoes a manual removal and drops `2bCW…` and the Mac key.
    - `install.sh` itself acts as a confused deputy. Its default `--version` is the newest tag in the operator's local checkout. If a tag signed with `5HpY…` was ever fetched there, it passes `install.sh`'s own `verify-tag` and runs as root through `sudo styrman-ctl install`.
    - A current tag signed with `2bCW…` fails `verify-tag` against the stale file. That path fails closed.
  - Remediation:
    - Replace `${SECRETS}/allowed_signers` on the operator's machine with the NUC's `2bCW…` line.
    - Name that file in BUG-094's Fix and in `gcp.md`'s operator step.
    - Turn the rule into a mechanism: `styrman-ctl install` and the updater refuse the fingerprint `SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU` by name. This is a one-line denylist check on the fingerprint they already compute (`styrman-ctl:590-592`).

- **[low / confidence: medium] The rebuild ties the binary to its source, but the source itself rests on GitHub alone.**
  - Where: ADR-090 Decision ("The pin's provenance is a rebuild from the harness tag, not GitHub's `SHA256SUMS` alone"), `image/Dockerfile:192-195`.
  - The reproducible rebuild proves that `3031e27e…` is the build of commit `7ea9068` as hosted on GitHub. That commit was published after the 2026-10-05 exposure, and the exposed key was dave's GitHub login key until 2026-10-06. Nothing in crew shows that `7ea9068` is the operator's own commit, or that anyone reviewed the source between v2.3.2 and v2.4.1.
  - I could not check it myself: no harness checkout was found at `~/workspace/styrman-harness`, `~/workspace/devshell_prompt` or `~/workspace/harness`.
  - Remediation, either of:
    - Confirm that `7ea9068` appears in the reflog of the clone the release was cut from as `commit`/`merge`, not as a fetch.
    - Review the v2.3.2..v2.4.1 diff.
    Record which in ADR-090, and make the same check part of every future bump.

- **[low / confidence: high] The trust records state the target state as if it were current.**
  - ADR-089 amendment 1 opens with "No host lists it:", then says in the same sentence that the GCP VM's list "must drop its `5HpY…` line".
  - `gcp.md:80-84` says "The VM's list holds the NUC's release signer (`SHA256:2bCWnLoM…` since v1.42.4…)". BUG-093's Fix and the 2026-10-05 handoff, Next Steps 1, say the VM does not yet hold it, and nothing in the range confirms that it does.
  - A reader skims a trust record as a closed guarantee.
  - Remediation: write each as a rule ("no host may list…", "the VM must list…"), followed by an "as of <date>" state line.

- **[low / confidence: medium] It is not confirmed that the exposed key is gone from GitHub's SSH signing keys.**
  - Where: BUG-094 ("GitHub SSH signing keys could not be listed (404 for the token)").
  - If the key is still registered for signing, GitHub shows commits and tags signed with the exposed key under the operator's account as "Verified". Crew's updater does not rely on that badge, so the impact is limited to people reviewing on GitHub.
  - Remediation: check Settings → SSH and GPG keys → Signing keys in the browser, or run `gh auth refresh -s read:ssh_signing_key` and then `gh api user/ssh_signing_keys`. Delete the key and record the result in BUG-094.

## Checked

- **Earlier review's fixes in the committed state (`image/Dockerfile:183-209`). All hold:**
  - Pin shape: `case … *[!0-9a-f]*) exit 1` plus `[ "${#HARNESS_SHA256}" -eq 64 ]`, under `set -eu` in dash. This rejects an empty pin, a newline and a backslash. `echo` without `--strict` is safe after that check.
  - `--repo github.com/kesokaj/styrman-harness` pins the host, so `GH_HOST` cannot redirect the download.
  - ADR-090 says "fails the build at the harness RUN step".
  - Pin provenance (the rebuild) is recorded in ADR-090, the Dockerfile comment and `deploy.md`.
  - ADR-090's Consequences state the trust model of the in-project update.
  - BUG-094 and ADR-089 amendment 1 now record the exposure. The operational part is the first finding above.
- **Bytes in the live image.**
  - Every image built on 2026-10-05, from `:dev` at 13:25 to the live `:1.42.4` at 20:16, carries `sh-agent` v2.3.2 `fdc82107…`. So do `:1.41.0` and `:1.41.1`, built 2026-10-03, before the exposure.
  - So the image's harness binary was not swapped during the exposure window. This closes the earlier review's not-examined item for the image only, not for homes.
  - `:harness-v2.4.1` and `:test` carry `3031e27e…`, which equals the pin.
- **Secrets:** none in the range.
  - The build-secret path is unchanged and still correct: `required=true`, no `set -x`, the token sits in `gh`'s environment for that one RUN only, and the temp dir is removed.
  - The trail contains public-key fingerprints, the operator's email (already in the repo), a session ID, and the per-deployment cookie-name prefix (public by nature). None of these is a credential.
- **Instruction-shaped text:** I found none aimed at a reviewer or agent. The handoffs' Next Steps are operator steps.
- **Injection:** the range adds no new external input. `HARNESS_*` comes only from the Dockerfile; `git grep` finds no `--build-arg HARNESS_*` at any build site. The untrusted `SHA256SUMS` is read only with fixed-string `grep -x`.
- **Authz/authn:** no code touched. Tag-signer trust is covered by the findings.
- **Crypto:** nothing hand-rolled. `sha256sum` is used as designed, and the removed `ssh-keygen -Y verify` was a vetted tool.
- **Dependencies:** none added. The harness bump v2.3.2 → v2.4.1 is pinned by hash.
  - `govulncheck ./...` with the host's go1.25.0: rc=3, 31 called vulnerabilities, mostly the host's stdlib@go1.25, which is a toolchain artifact. Output: `.agents/scratch/sec-gate-govulncheck.txt`.
  - `bash deploy/vulncheck.sh`, the project gate, with pinned `golang:1.25.13`: rc=3. Both modules have two called vulnerabilities, GO-2026-4887 and GO-2026-4883, in `github.com/docker/docker@v28.5.2+incompatible`, with no fixed version. They predate this range. Output: `.agents/scratch/sec-gate-vulncheck.txt`.
  - `npm audit` (web): rc=1, one high in `source-map-js` (build-time). Predates this range.
- **GCP surface:** no change to IAM, service accounts, Cloud Run or Secret Manager. The VM's signer state is covered by the findings.
- **`sh-agent search --refs <path> --kind bug`** for all 26 paths: past bugs on `image/Dockerfile` (BUG-066/080/086), `web/src/lib/api.ts` (BUG-022/055/085) and docs. None bears on the changed lines. BUG-066's gh_token secret is still passed at `ensure-test-image.sh:59`.

## Not examined

No release-diff path is left unread: every hunk of all 26 paths was read in full. What I did not reach:
- **Live trust lists:**
  - The GCP VM's real `/etc/styrman/allowed_signers`, its updater state, and the operator's local `${SECRETS}/allowed_signers`: no access.
  - dave's `/etc/styrman/allowed_signers`: permission denied without sudo, and I did not use sudo. The "only `2bCW…`" claim is the main agent's.
- **Harness side:**
  - The harness source at `7ea9068` and its provenance in an operator clone.
  - The update code of harness v2.3.2 and v2.4.1. The use of the embedded key is inferred, not executed.
  - `sh-agent` copies inside live project homes.
- **Build evidence:**
  - I ran no docker build of the committed Dockerfile myself.
  - I did not read the author's `.agents/scratch/harness-build*.out` logs.
  - The verifier log predates the final RUN-step edit.
- **Partly read this time (grep only, not re-read):** `deploy/host-setup.sh:316` continuation and `:405-415` (how it seeds `allowed_signers`), and the arguments at `verify-commit.sh`'s `build_image` call.
- **Unchanged text around modified files:** not re-read in `DECISIONS.md`, `deploy.md`, the testing README, `versioning.md`, `remote.md`, and `gcp.md` outside lines 55-95.
- **GitHub account state:** the signing keys, and whether the harness's BUG-160 token was revoked.
