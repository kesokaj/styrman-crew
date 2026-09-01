# Preflight gate — v1.16.0 (2026-09-01)

Base: `v1.15.0` → HEAD `9d41e55`. Four commits, 85+ files.
Gate owners dispatched per `/preflight` Step 2; trail and docs freshness per Steps 3–4.

## VERDICT: SHIP

Ship the **tag**. Do **not** enable the auto-update timer yet — see the blocking
condition below. The timer ships disabled and uninstalled, which is what keeps
its open items off the tagging path.

---

## FAIL-first: what is still open

Nothing blocks tagging. Everything below blocks *enabling the updater*, a
feature that ships off.

### [medium / confidence: medium] The decoy-marker fix is not independently verified

`deploy/update/styrman-update` now compares the **last** armor header in the tag
object rather than grepping for a marker, because the tag message is
author-controlled and git's `parse_signature` keeps the last header — a decoy
`-----BEGIN SSH SIGNATURE-----` in the message passed the old check while a real
PGP block appended at the end still won, and `verify-tag` then exited 0 with the
allowlist never consulted.

Security reproduced that bypass. **I could not** — gpg signing failed in my
scratch keyring, so I never built the crafted tag. The fix is security's own
recipe, and I confirmed it against synthetic tag objects (decoy+PGP → refuse,
genuine PGP → refuse, genuine SSH → accept) plus a live control proving a real
SSH-signed tag in the allowlist still verifies. That is weaker evidence than a
reproduction, and it is the last change in the release, so **it has had no
independent review at all**.

### [medium / confidence: high] Enabling the timer requires setup that does not exist here

The gate fails closed and would refuse every existing tag. Before enabling:
`/etc/styrman/allowed_signers` (root-owned, not group/world-writable), releases
signed with SSH (`git tag -s` with `gpg.format ssh`), a deploy key, a
`known_hosts` seeded from GitHub's **published** fingerprints, the `/opt/styrman`
clone, and that clone's own `deploy/.env`. None of `/opt/styrman`,
`/etc/styrman` or `/var/lib/styrman/update` exists on this host, and
`styrman-update.timer` is `not-found`.

### Carried, recorded in `TODO.md`, not fixed

Unsaved editor edits are still lost without a prompt when switching terminals;
`TerminalPane` sends up to four identical resize frames per toggle; no
handler-level Go test for `checkFile`/`formatFile`; `deploy/Dockerfile.server`
embeds a committed `web/dist` with nothing verifying it matches `web/src`;
unpinned `yq`/helm/OpenTofu fetches; two unfixable `docker/docker` govulncheck
findings (no upstream fix available).

---

## Gate owners

| Owner | Verdict | Report |
|---|---|---|
| `reviewer` | APPROVE with nits — none SHIP-blocking | `2026-09-01-preflight-release-gate.md`, `…-recheck.md` |
| `security` | FINDINGS — none block tagging | `2026-09-01-security-preflight-v1.15.1.md`, `…-recheck.md`, `…-final.md` |
| `verifier` | **PASS** | `2026-09-01-verify-preflight.md` |

Four remediation rounds ran inside this gate. Each independent pass found
something real in the auto-updater, and three of those were cases of **code
asserting a security property it did not have**:

1. `git verify-tag` alone does not pin the signer — with PGP it exits 0 for a
   good-but-untrusted signature and never compares signer to tagger. The trust
   set was "any key in root's keyring", not "the operator", as the comment and
   the changelog both claimed.
2. The allowlist path was read from root's `--global` gitconfig, which is
   unreadable under the shipped unit (`ProtectHome=yes`, no `$HOME`). The timer
   would have refused everything while a hand-run `sudo styrman-update` passed —
   invisible until the timer was enabled.
3. `deployed-sha` was written and never read, so the rollback still re-verified
   the previous *tag*; every tag here predates signing, so the first rollback was
   guaranteed to be refused from inside an `if` condition, leaving the box on the
   broken release. Found by the **documentation** agent, which documented what
   the code did rather than what it was told.
4. The `ssh-keyscan` seeding was the exact mechanism the first security review
   had rejected, paired with a comment claiming the TOFU window was closed.

## Steps 3–4

- **Trail freshness:** `trail-check.sh --check` → **FRESH**. Newest `changes/`
  entry covers HEAD; no wrong-dated filenames. The `ROADMAP.md`/`TODO.md` notes
  are report-only and reflect both being committed *with* HEAD.
- **Docs freshness:** every user-visible surface in the diff is documented —
  auto-updater and its signing/verification prerequisites (`deploy.md`),
  `POST /api/projects/{project}/check` (`api.md`), the editor pane (`guide.md`),
  `shellcheck`/`hadolint` (`runtime-image.md`), plus `README.md` and `AGENTS.md`.
  `deploy.md` was corrected mid-gate: it was still teaching `ssh-keyscan`
  seeding and a bare `verify-tag` after both were removed from the code.

## Suites

`go build` · `go vet` · `go test -race -count=1 ./...` · `gofmt -l` · the separate
`deploy/caddy/ingress` module · `tsc -b` · `oxlint` · `shellcheck -S error` ×3 —
all clean. Integration **57/0/3**. Browser e2e **48 passed / 0 failed / 2 skipped**
(34 at the start of the day; the 2 skips are the known host-firewall
port-preview cases).

## What was NOT checked

- **The final commit (`9d41e55`) has had no independent review.** It closes the
  decoy-marker bypass and asserts the state dir. Verified by me only.
- **The decoy bypass was never reproduced by me**, only by security. My evidence
  is synthetic objects plus a non-regression control.
- **The updater has never run end to end on this host.** Every test used scratch
  clones with `STYRMAN_DEPLOY_DIR`/`STYRMAN_UPDATE_STATE_DIR` overrides. No real
  tag has been fetched, verified, built and deployed by it, and the rollback path
  has never executed against a genuinely failing release.
- **ARM64 is out of scope, not passing.** The runtime image is deliberately
  AMD64-only (`AGENTS.md`, `runtime-image.md`) and pins `hadolint-Linux-x86_64`,
  `yq_linux_amd64` and an amd64 Go toolchain. The verifier found that with QEMU
  the ARM64 build *completes*, then proved by execution (`go version` →
  `linux/amd64` inside the "arm64" image) that it is an emulation artifact, not
  compatibility. `deploy/Dockerfile.server` is genuinely portable and built and
  ran clean on both arches.
- **`styrman-update` has no automated tests**, matching this repo's
  shellcheck-only convention for privileged scripts. Its gate paths were
  exercised by hand.
- **Nothing was deployed.** The live server remains on v1.14.2 and therefore
  still carries the `/format` data-loss bug this release fixes.

## The reason to deploy promptly

`/format` returned `{"content":""}` for every request, and the editor pushes that
straight into the buffer — Format then Save wiped the file. It is live on
v1.14.2 right now. That is the strongest argument for shipping this release
rather than polishing it further.
