---
owner: security
date: 2026-10-06
---
# Security review — harness-sha256-pin   (2026-10-06)

Range reviewed: HEAD (927eb72)..working tree (uncommitted `git diff HEAD`).
- **Files:** `image/Dockerfile`, `.agents/docs/architecture/DECISIONS.md` ADR-090, `REQUIREMENTS.md` R5, `specs/styrman/deploy.md`, `specs/styrman/runtime-image.md`, `testing/styrman/README.md`, `README.md`, `image/README.md`. That is 8 files, +60/-39, as of the last read.
- **The tree moved during this review:** `REQUIREMENTS.md`, the Dockerfile header and two docs were edited after my first read. Line numbers below are from the final state.
- **Blast radius followed:**
  - the five runtime-image build sites;
  - `image/bump-pins.sh`;
  - `deploy/gcp.md`;
  - ADR-082 and ADR-089;
  - the harness's own ADR-053/053a (kesokaj/styrman-harness@v2.4.1, read through `gh api`, as data).

## Status: COMPLETE

## Verdict: FINDINGS

The Dockerfile change works as written: every check fails the build closed, no build site can override the pin, and the build secret is handled exactly as before.

What is broken is the trust model around the change. ADR-090 says trust "rests on the crew commit (a signed release tag)". Three facts undercut that:
- **The removed signer is also crew's tag signer.** The signer this diff removes from the Dockerfile, `SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU` (fingerprinted here from `HEAD:image/Dockerfile`), is crew's original release-tag signer.
- **That key was exposed.** The harness's own ADR-053 records it as dave's GitHub login key, "exposed on 2026-10-05".
- **It is still trusted on the GCP VM.** Crew's `deploy/gcp.md:80` still lists that key as trusted there.

The pinned v2.4.1 hash was also copied from GitHub after that exposure, and nothing in the change verifies it independently.

Removing `HARNESS_SIGNER` from the image is an improvement on HEAD, which still accepted anything that exposed key signed. Even so, the change is only as sound as two things it does not establish: where the hash came from, and which keys each host's `allowed_signers` still lists.

## Findings

- **[high / confidence: medium] The trust anchor ADR-090 names still accepts a key its owner rotated because it was exposed.**
  Locations: `.agents/docs/architecture/DECISIONS.md:2993` (ADR-090 "Trust rests on the crew commit (a signed release tag)"), `.agents/docs/deploy/gcp.md:80-85`, `DECISIONS.md:2988` (ADR-089 "The original key can no longer sign").
  - The removed `HARNESS_SIGNER` is `SHA256:5HpYQJjpG+…`. That is the same key as crew's original release-tag signer (`DECISIONS.md:2822`: dave's passphrase-less `~/.ssh/id_ed25519`).
  - Harness ADR-053 (styrman-harness@v2.4.1 `.agents/docs/architecture/DECISIONS.md:588`) says this key "was also dave's GitHub login key; it was exposed on 2026-10-05".
  - Crew records it only as "replaced". The GCP VM's `allowed_signers` still lists it (`gcp.md:80`). The documented operator step adds the new key and does not remove the old one (ADR-089 Consequences).
  - On any host that lists it, a crew tag signed with that key verifies. That host's auto-updater then builds whatever the tag carries: any `HARNESS_SHA256`, any code. The pin is therefore no stronger than the weakest host's `allowed_signers`.
  - Remediation:
    - Remove `SHA256:5HpYQ…` from every host's `/etc/styrman/allowed_signers` (GCP VM first; confirm dave's list holds only `2bCWnLoM…`).
    - Delete the key from GitHub, both as an authentication key and as a signing key.
    - Record the exposure in crew (a bug record, plus an amendment to ADR-089 saying "exposed", not "replaced").
    - Correct `gcp.md:80-85` and ADR-089's "can no longer sign".
  - This condition predates the diff. It is in scope because ADR-090 makes this anchor carry the whole pin.

- **[high / confidence: medium] The pin's provenance is GitHub alone, and the pinned value was taken in a window where, by the harness's own records, others besides the operator could publish releases.**
  Locations: `image/Dockerfile:195` (`ARG HARNESS_SHA256=3031e27e…`), `image/Dockerfile:190-193`, `image/README.md` and `specs/styrman/deploy.md` "Image build" ("the hash copied from that release's `SHA256SUMS` line"), `DECISIONS.md:2993`.
  - The bump procedure copies the hash from the same channel the binary comes from. Whoever can publish a release on kesokaj/styrman-harness when the hash is copied therefore chooses the bytes that get pinned. The signed crew commit then carries their hash faithfully.
  - Harness ADR-053/053a (`DECISIONS.md:591-600` there) lists who could publish a release: "any machine logged in to GitHub", GitHub Actions on the repository ("anything that can push a workflow can publish a release"), and "the gh token found in plain text in scratch (BUG-160)". The exposed key was also the GitHub login key.
  - v2.4.1 postdates that exposure, and nothing in this change checks 3031e27e… against anything except GitHub's own `SHA256SUMS`.
  - Before ADR-090, getting into a build needed the signer key as well as GitHub. After a bump, the pin is stronger than the signature was: the bytes are fixed, and a stolen key cannot swap them under an existing tag. At bump time it is weaker: plain trust-on-first-use.
  - Impact if the hash is bad: code in every project VM at its next boot, beside every user's brokered credentials.
  - Remediation: before committing, check 3031e27e… out of band. Either use the artifact on the machine that cut v2.4.1 (its local `sha256sum` of `sh-agent-linux-amd64` from the release run), or rebuild sh-agent from the v2.4.1 tag on a trusted machine and compare. Record where the hash came from in the commit, and in ADR-090 for every future bump. Never take the pin only from the GitHub copy. ADR-090's Consequences should state the trust-on-first-use step.

- **[medium / confidence: high] In-project `sh-agent update` now trusts GitHub alone, outside the pin's reach, and this diff installs that updater into every home.**
  Locations: `DECISIONS.md:2994` (ADR-090 "`sh-agent update` works from then on"), `README.md:13-16`, `REQUIREMENTS.md` R5 ("each project moves itself forward when its user runs `sh-agent update`").
  - Harness ADR-053 (`DECISIONS.md:591` there): `sh-agent update` "check[s] the download against the release's `SHA256SUMS` … and never against a signature".
  - v2.3.2 refused unsigned releases (ADR-090's own text). The v2.4.1 that this image installs into every home at its next boot accepts any release that the publishers listed in the previous finding put up.
  - Each home materializes its user's git/gh/ssh/claude/agy/gcloud credentials, so one rogue release reaches every user who runs `update`.
  - ADR-090 presents this as a fix ("works from then on"). R5, edited during this review, describes the image's pin but not the unverified update path.
  - Remediation: record it in ADR-090 and R5 as an accepted consequence the operator signs off on, or limit it. Options are a harness-side check against a pinned or out-of-band list, or crew documenting `sh-agent update` as unverified until the harness verifies again.

- **[low / confidence: high] The pin's shape is not validated, so a value with a newline in it passes both checks without the binary ever being hashed.**
  Location: `image/Dockerfile:201-202`.
  - `grep -F` splits a pattern on newlines into several patterns (probe H, rc=0).
  - `sha256sum -c` without `--strict` exits 0 when the binary's line is malformed and another listed file verifies (probe G, rc=0; with `--strict`, rc=1).
  - dash's `echo` also interprets backslash escapes in the value.
  - No current build site forwards any `HARNESS_*` build-arg (checked below). A Dockerfile `ARG` default cannot hold a raw newline. Whoever sets the ARG controls the pin anyway. So this is hardening, for a future site that forwards an outside value and for review clarity.
  - Remediation: validate the value before use (`case "${HARNESS_SHA256}" in *[!0-9a-f]*|'') exit 1;; esac; [ "${#HARNESS_SHA256}" -eq 64 ]`). Then use `printf '%s  %s\n'` with `sha256sum --strict -c -`, or `test "$(sha256sum < sh-agent-linux-amd64 | cut -d' ' -f1)" = "${HARNESS_SHA256}"`.

- **[low / confidence: high] ADR-090 names the wrong failing line.**
  Location: `DECISIONS.md:2994`.
  - ADR-090 says a binary changed under an existing tag "fails the build at `sha256sum -c`".
  - When `SHA256SUMS` is re-uploaded to match, the build fails one line earlier, at `grep -qxF "${HARNESS_SHA256}  …"` (`Dockerfile:201`). Still fail-closed.
  - Remediation: "fails the build at the harness RUN step".

Resolved during the review (the tree moved; re-read in the final state):
- R5 (`REQUIREMENTS.md:55-59`) no longer claims signed checksums.
- The Dockerfile header (`:4-7`), the testing README (`:415-417`) and `image/README.md` no longer say "verified against the harness's signer".

The R5 edit cites "operator approval 2026-10-06". ADR-090 records approval of the decision. Whether the R5 wording itself was approved, as the REQUIREMENTS rule demands, is for the main agent to confirm, not something I can verify.

## Checked

- **Every check can fail the build:** yes.
  - The RUN uses `/bin/sh` (dash; no `SHELL` instruction) with `set -eu`. In the pipeline the checker comes last, so `sha256sum`'s exit status is the one that counts.
  - Shell-level probe of the RUN's check lines in `.agents/scratch/` (removed afterwards), with this host's coreutils 9.7 and dash:
    - correct pin: reached install, rc=0
    - all-zero pin: rc=1
    - wrong version header: rc=1
    - empty pin: rc=1
    - uppercase pin: rc=1
    - binary swapped with `SHA256SUMS` left unchanged: rc=1
  - Docker-level evidence (a log I read; I did not rerun it): `testing/2026-10-06-verify-harness-v2.4.1.md` shows the real build passing (`sh-agent v2.4.1`, rc=0). Its must-fail controls `HARNESS_SHA256=0…0` and `HARNESS_VERSION=v2.4.0` both stop at the harness RUN with rc=1.
- **Build-arg overrides, all five sites:** none passes `HARNESS_VERSION`/`HARNESS_SHA256`; each passes only `STYRMAN_VERSION` and the secret.
  - Sites: `deploy/update/styrman-update:902`, `deploy/host-setup.sh:316-318`, `deploy/styrman-ctl:515`, `deploy/verify-commit.sh:239`, `.agents/docs/testing/styrman/ensure-test-image.sh:59`.
  - The compose file builds only the server and Caddy.
  - No `--cache-from` anywhere, so no external cache can supply the RUN layer.
  - Each ARG is declared exactly once. No later step writes `/styrman/bin/sh-agent` (`Dockerfile:206-230` read).
  - `image/bump-pins.sh` rewrites only GO/NODE/CLAUDE_CODE and base digests (`:111,115`), never the harness pair.
  - Aside: `host-setup.sh:457` prints a manual `docker build` with no `--secret`. Run as printed it fails at `required=true`, so it fails closed; it is a docs defect from before this diff.
- **Secrets:** none in the diff. The removed `HARNESS_SIGNER` was a public key and an email; the pin is public data.
  - The secret handling lines are unchanged: `required=true`, the token is in gh's environment for that one RUN only, there is no `set -x`, and nothing reaches a layer.
  - Every site picks GH_TOKEN, then a root-owned 600 non-empty token file (`host-setup.sh:306-314`), then stops.
  - `verify-commit.sh` and `ensure-test-image.sh` fall back to `gh auth token`, a token the harness's ADR-053a says can publish releases. It is confined to that RUN; this predates the diff.
- **Injection:** `HARNESS_VERSION` and `HARNESS_SHA256` are quoted throughout. The untrusted `SHA256SUMS` is only read by fixed-string `grep -x`. Nothing from the release runs before the hash check. The newline case is a finding above.
- **Authz/authn:** no auth code touched. The tag-signer trust anchor is the first finding.
- **Crypto:** nothing hand-rolled. coreutils `sha256sum` is used as designed, and the vetted `ssh-keygen -Y verify` is removed, not replaced.
- **Dependencies:** none added or removed.
  - `govulncheck ./...` (host go1.25.0) exit status 3. It reports 31 called vulnerabilities:
    - stdlib `@go1.25`, fixed in later 1.25.x patches. This is a host-toolchain artifact; the project's gate, `deploy/vulncheck.sh` with the pinned `golang:1.25.13` builder, was not run.
    - `github.com/docker/docker@v28.5.2+incompatible`: 2, no fixed version available.
    - Both predate this diff and are outside it.
  - `npm audit` (web/) exit status 1: 1 high, `source-map-js@1.2.1` (GHSA-68fv-2mgg-jv7q) via vite, postcss and @tailwindcss/vite. Build-time only, predates the diff.
- **GCP surface:** no IAM, service-account, Cloud Run or secret-env change. The GCP VM builds through `styrman-ctl install` (no HARNESS_* arg). Its `allowed_signers` is the first finding.

## Not examined

- The `sh-agent` bytes inside the currently deployed runtime images. HEAD (v2.3.2) verified them only against the now-exposed key, and they are not compared against any pre-exposure record. Suggested: `docker run --rm --entrypoint sha256sum styrman-runtime:<live ver> /styrman/bin/sh-agent` against a v2.3.2 hash recorded before 2026-10-05.
- The GCP VM's real `/etc/styrman/allowed_signers` and whether its auto-updater is on. I had no access; the docs (`gcp.md:80`) are the only evidence.
- Whether the exposed key is still registered on GitHub, and whether the harness's BUG-160 plaintext token was revoked.
- sh-agent v2.4.1 source. I read only its ADR-053/053a text; no update source file matched by name in the tree listing.
- A docker build of my own: not run. The docker-level results above are the verifier's log, cited, not reproduced.
- `deploy/vulncheck.sh` (pinned builder) and the separate `deploy/caddy/ingress` module: not run, outside this diff.
- `image/entrypoint.sh`'s boot-time use of `/styrman/bin/sh-agent` (which user runs `install`): not read.
- The reviewer's 2026-10-06 report: deliberately not read (independence).
- Any edit to the tree after my last read (`git diff HEAD --stat`: 8 files, +60/-39).

## Outcome
- Exposed key still trusted (high) — confirmed: harness ADR-053 records `SHA256:5HpY…` as exposed on 2026-10-05, and `deploy/gcp.md:80` lists it on the GCP VM. Outside this diff (ADR-082/089's signer lists, operator-held hosts and GitHub account): raised with the operator as a separate item; this change does not claim to resolve it. ADR-090 now says which keys a host accepts is ADR-082/089's question.
- Pin provenance (high) — fixed: the pin was confirmed independently of GitHub's `SHA256SUMS` by rebuilding the tag. `v2.4.1` (annotated tag) points at `7ea90689ccb5`; a clean clone (`git status --porcelain --ignored` empty) built in `golang:1.27.1` with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTOOLCHAIN=local go build -trimpath -ldflags "-X main.version=v2.4.1" ./cmd/sh-agent` gives `3031e27ea2e6aa98adf102ff5405ff960578b5223cc3a9596019dfb2075e80b1`, bit for bit (the toolchain read from the release binary's `go version -m`). Recorded in ADR-090, the Dockerfile comment and deploy.md's bump procedure, which now requires the rebuild before the hash is committed.
- In-project `sh-agent update` trusts GitHub alone (medium) — fixed as wording: ADR-090's Consequences now say the pin covers the image only and the in-project update is the harness's own trust model (ADR-053/053a). The behaviour is the harness's, unchanged here.
- Pin shape (low) — fixed: the RUN step first refuses a `HARNESS_SHA256` with any character outside `0-9a-f` (newline included) or a length other than 64. Must-fail builds: uppercase pin rc=1 before the download, 8-character pin rc=1, all-zero pin rc=1; must-pass build rc=0, image `sh-agent v2.4.1`, `/styrman/bin/sh-agent` sha256 `3031e27e…` (`.agents/scratch/harness-build2.out`). The same step now names the host in `--repo github.com/kesokaj/styrman-harness` (the harness's ADR-053a `GH_HOST` point).
- ADR-090 names the wrong failing line (low) — fixed: "fails the build at the harness RUN step".
