# Review — PREFLIGHT release gate RECHECK, 6b41979..534e0bb   (2026-09-01)

Scope: the single follow-up commit `534e0bb` ("security: pin the tag signer,
verify on rollback, drop the keyscan TOFU seed"), 8 files, +880/-30 — three of
which are the preflight report set itself. Reviewed code: `deploy/host-setup.sh`
step 8c, `deploy/update/styrman-update`, `internal/httpapi/server.go`,
`internal/httpapi/check_test.go`. `web/dist/client/_shell.html` diffs by exactly
one byte-range — the build stamp `u:1788261743823` → `u:1788264176451`; no asset
graph change, nothing to check. Verification gates are taken on report from
`.agents/docs/testing/2026-09-01-verify-preflight.md`; I re-ran only
`shellcheck -S style` on both shell files (clean bar two pre-existing infos, one
of which is a false positive on prose backticks in the heredoc) and checked the
repo's actual tag signature state, which turns out to matter (F2).

## Verdict: APPROVE WITH NITS — nothing in this delta is SHIP-blocking

My findings 1–3 are genuinely closed, not papered over. The auto-clone and the
`ssh-keyscan` seed are both gone from `deploy/host-setup.sh`; nothing writes
`/etc/styrman/known_hosts` any more, so there is no unauthenticated write path
into the updater's trust anchor and no non-idempotent step left in 8c (I
re-walked every statement in the step — `install`, `daemon-reload`, the now
echo-only clone branch, `systemctl enable --now`, the heredoc — all re-runnable).
The updater comment no longer claims a property the code lacks; it now names the
channel the trust comes from and says why `ssh-keyscan` is not it. The instruction
block seeds from `api.github.com/meta` over TLS with a cross-check against the
published fingerprints page and clones with `StrictHostKeyChecking=yes`. That is
the right shape.

The worst thing in the delta is new and was not in the diff I reviewed before:
**`verify_tag` reads its allowlist path from root's `--global` gitconfig, and
`styrman-update.service` sets `ProtectHome=yes`, which makes `/root` — and
therefore `/root/.gitconfig` — inaccessible to the unit.** Under the timer, the
only automated path, `git config --global --get gpg.ssh.allowedSignersFile`
returns empty and every tick dies at "tag signing is not verifiable", even for an
operator who followed the printed instructions exactly. It fails CLOSED, so it is
not a security hole and not ship-blocking, but the feature is inoperative in its
shipped configuration and the trap is well hidden: `sudo styrman-update --check`
run by hand passes (no `ProtectHome`), *then* the timer is enabled, and the
failures are journal-only from there. Second: because `deploy()` now verifies
every tag and **every existing tag in this repo is unsigned** (`v1.14.2` is even
lightweight), the first rollback on any host will hard-`fail` instead of rolling
back. That is the behaviour the security review explicitly asked for, so I am
recording it as an operational consequence to accept knowingly rather than a
defect to fix blind.

Finding #4 (changelog overstates the `EditorPane` key fix) and the `TerminalPane`
resize-spam low are tracked as being handled in parallel; not re-reported.

## Findings

- **[high / confidence: high]** `deploy/update/styrman-update:161` +
  `deploy/update/styrman-update.service:19` — `signers="$(command git config
  --global --get gpg.ssh.allowedSignersFile …)"` resolves against `$HOME`, and the
  unit runs with `ProtectHome=yes`, under which systemd makes `/root` inaccessible
  and empty. The lookup returns nothing (confirmed locally: `HOME=/nonexistent git
  config --global --get …` → empty, exit 1; likewise with `HOME=`), so the very
  next line `fail`s. Compounding it, a systemd system service without `User=` is
  not guaranteed `HOME=/root` at all, while `sudo git config --global …` from the
  instructions writes `/root/.gitconfig` — two independent ways for the path to
  disagree. The comment above the function says the design "ASSERT[s] it is
  configured rather than relying on root's ambient gitconfig", but the code relies
  on exactly that; the security review's own remediation was to *pin* the path:
  `git -c gpg.ssh.allowedSignersFile=/etc/styrman/allowed_signers verify-tag`.
  Fix: drop the `git config --global` lookup, use the fixed root-owned
  `/etc/styrman/allowed_signers` (the path host-setup already prints), assert it is
  root-owned, non-empty and readable, and delete the `sudo git config --global`
  line from the instructions so the two sides cannot drift. Not ship-blocking —
  the timer ships installed-but-disabled and this fails closed — but the updater
  must not be enabled until it is fixed, and hand-testing must be re-done via
  `systemctl start styrman-update`, not as a bare root shell, or it will pass
  again for the wrong reason.

- **[medium / confidence: high]** `deploy/update/styrman-update:221` — `deploy()`
  now calls `verify_tag "${tag}"` on the rollback target too, which is correct
  security-wise, but `verify_tag` reports failure with `fail` (i.e. `exit 1`), not
  a return code. Since `deploy` is invoked as `if deploy "${PREVIOUS}" &&
  reports_version …`, an unverifiable rollback target exits the script from inside
  the condition: the box is left running the broken new release, `${STATE}` is
  never corrected, and the deliberately-worded `fail "rollback to ${PREVIOUS} ALSO
  failed — manual intervention required"` is unreachable on this path. This is not
  hypothetical: `git verify-tag` fails for every tag in the repo today
  (`v1.13.2`…`v1.15.0` unsigned annotated, `v1.14.2` lightweight), so the first
  rollback after a signed `v1.15.1` is guaranteed to hit it. It also adds a second
  instance of the `deploy()`-calls-`fail`-instead-of-`return 1` defect already
  tracked at `TODO.md:64-66`. Two fixes, both cheap: (a) give `verify_tag` a
  non-fatal caller contract (`verify_tag "$tag" || return 1` inside `deploy`) so
  the rollback failure surfaces through the intended path; and (b) for the real
  problem — rolling back to a pre-signing tag — record the *commit SHA* alongside
  the tag in `${STATE}` at deploy time and roll back by SHA, which is immune to
  the re-pointed-tag attack the verification was added to stop and does not require
  history to have been signed. Until then, document in `deploy.md` that rollback is
  unavailable on hosts whose previous tag predates signing.

- **[medium / confidence: high]** `deploy/update/styrman-update:84` — the new
  `deploy/.env` assertion checks `stat -c %u` only. The security review it cites
  asked for `stat -c '%u %a' … = "0 600"`, and the mode is the half that matters
  for the stated threat: a root-owned but group- or world-writable
  `deploy/.env` passes this check and still lets any local user rewrite
  `STYRMAN_ALLOWED_EMAILS` (full auth bypass) and `STYRMAN_INGRESS_TOKEN`. The
  script already knows how to do this properly — 45 lines later `find … -perm /022`
  guards `update.env`. Apply the same test here (and the same argument applies to
  the pre-existing `.git` assertion above it).

- **[low / confidence: high]** `internal/httpapi/check_test.go:254-272` —
  `TestCheckAndFormatGetTheLargeBodyLimit` does not test what it is named for. The
  first loop builds three literal strings and asserts they end with the suffixes
  just written into them; it never touches `csrfGuard`, so deleting the `/check`
  case from `server.go` leaves this test green. The two real assertions bound
  `maxCheckBytes` against a hardcoded `8<<20` that is not linked to the limit in
  `server.go`, so the two can drift silently, and they ignore JSON escaping (5 MiB
  of content can exceed an 8 MiB body once escaped, so "reachable" is approximate).
  It is theatre for its headline claim, and the honest test is ~15 lines:
  `csrfGuard` is a plain `func(http.Handler) http.Handler`, so wrap a handler that
  `io.Copy`s the body, POST 2 MiB to `/api/projects/x/check` and assert the read
  succeeds, then POST the same to a path with no special case and assert it fails.
  Keep the `maxCheckBytes` bound as a second, separate assertion. Not blocking —
  the production change it accompanies is correct.

- **[low / confidence: high]** `deploy/host-setup.sh:348-360` — the clone branch
  now logs `creating deploy clone at /opt/styrman (from <origin>)` and then
  immediately prints `==> deploy clone not created automatically`. The `log` line
  and the comment above the `if` ("Try, but treat failure as an expected, explained
  outcome rather than a stray WARN") are both stale leftovers of the deleted
  auto-clone and now contradict the code — delete them, and collapse the
  `if [ -n "${ORIGIN}" ]` wrapper, which now guards nothing but a static echo
  (`ORIGIN` is still needed for the heredoc).

- **[low / confidence: medium]** `deploy/update/styrman-update:161,165` —
  `verify_tag` bypasses the file's own `git()` wrapper with `command git`, so the
  `-c safe.directory=` the wrapper exists to supply is absent from the one command
  that reads the repository under the strictest conditions. It is consistent today
  (script asserts `id -u` is 0 and `.git` is root-owned, so git's ownership check
  passes), but if the wrapper is ever needed, `verify-tag` fails on "dubious
  ownership" and the updater dies closed for a non-security reason. The wrapper
  composes fine — `git -c gpg.format=ssh -c gpg.ssh.allowedSignersFile=… verify-tag`
  expands correctly — so either use it, or state the reason for the bypass in the
  comment.

- **[low / confidence: medium]** `deploy/update/styrman-update:141` — the opt-out
  grep `'^[[:space:]]*STYRMAN_UPDATE_REQUIRE_SIGNED[[:space:]]*=[[:space:]]*0'` is
  unanchored at the end, so `…=0anything`, `…=01` and `…=0.5` all disable signature
  checking, while `export STYRMAN_UPDATE_REQUIRE_SIGNED=0` — a shape an operator
  might reasonably write into a file called `update.env` — does not. Both
  directions are wrong-ish; the second is the safe one. The security review's
  suggested anchored form (`'^STYRMAN_UPDATE_REQUIRE_SIGNED=\([01]\)$'`) is the
  fix. Ownership + mode assertions around it are correct and well placed.

- **[low / confidence: high]** `internal/httpapi/server.go:508-510` — the suffix
  match cannot over-match today: `HasSuffix(path, "/check")` and `"/format"` are
  each satisfied by exactly one registered route (`POST
  /api/projects/{project}/{check,format}`; there is no `/home/` variant), `/file`
  still excludes `/file/rename`, and an unrouted path ending in `/check` gets a
  raised cap on a body the 404 never reads. So: correct as written. The residual
  risk is future-tense — `strings.HasSuffix` on a raw path silently opts any
  later-added route with the same last segment into an 8 MiB cap. One line of
  comment naming the two intended routes, or matching the full path, closes it.

- **[low / confidence: high]** `deploy/host-setup.sh:387` — the duplicated
  parenthetical "(gitignored, so a fresh clone has none) (gitignored, so a fresh
  clone has none)" from my previous report survives, on a line adjacent to lines
  this commit edited. Same for "Three prerequisites, all root-only" followed by a
  prerequisite 3 that is neither root-only nor performed on this host, now made
  slightly worse: the "Then enable it:" block sits between items 2 and 3.

## Criteria

1. **Dead code — fail (low).** One stale comment block and one now-vacuous `log`
   line + `if` wrapper in `host-setup.sh` step 8c (finding 5); the first loop of
   the new Go test is inert (finding 4). No unused Go or shell symbols otherwise:
   `verify_tag` has two callers, `REQUIRE_SIGNED` three readers.
2. **Flow tracing — fail (see F1, F2).** Traced `REQUIRE_SIGNED` → `verify_tag` →
   both call sites → `deploy()` → the rollback block. Two hops break: the
   allowlist lookup against a `ProtectHome=yes` unit, and `fail`-not-`return`
   escaping the `if deploy … &&` condition. `set -euo pipefail` interaction is
   otherwise sound: `[ "${REQUIRE_SIGNED}" = "1" ] || return 0` is errexit-safe in
   both directions, `local tag="$1" signers` does not mask a command substitution
   exit status (the assignment is on its own line with `|| true`), and `signers` is
   always assigned before it is read under `set -u`. `LATEST` is verified twice on
   the deploy path (top-level, then inside `deploy`) — harmless, one extra
   `verify-tag`.
3. **Standards conformance — pass.** `shellcheck -S style` clean on the updater;
   the two host-setup hits are pre-existing (SC2015 at line 260) or a false
   positive on prose backticks. Go side is stdlib-only, gofmt/vet/test taken from
   the verifier log.
4. **Directory layout — pass.** No new files; the Go test sits beside the code it
   covers.
5. **Simplicity / DIY-except-security — pass.** No new dependencies. Nothing
   hand-rolled in the security exemption list: tag trust is `git verify-tag` with
   OpenSSH's allowed-signers mechanism, host-key trust is OpenSSH's, and the delta
   moves *away* from home-grown trust bootstrapping (the keyscan seed) toward the
   forge's published fingerprints. The `grep`-instead-of-`source` parse of
   `update.env` is the right stdlib-shell call for a single boolean.
6. **Decoupling — pass.** `verify_tag` is a clean seam with one input (`$1`) and
   one ambient dependency — which is precisely the one that breaks it (F1);
   removing that dependency makes the function self-contained. The httpapi change
   touches one `switch` and no other package.
