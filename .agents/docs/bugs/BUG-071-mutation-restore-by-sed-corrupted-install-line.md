---
owner: bugs
date: 2026-09-21
id: BUG-071
status: fixed
area: deploy/gcp/install.sh
---
# BUG-071: mutation-check restore by inverse `sed` corrupted a second, unintended line

## Symptom
While proving `TestGCPInstallCloneHelperShape` (`internal/config/gcp_scripts_test.go`)
fails without its fix, `deploy/gcp/install.sh` was mutated with `sed` and then
"restored" with the inverse pattern (no line address). The named test failed
under the mutation and passed after the restore, so the mutation counted as
verified.

## Repro
Mutate: `cat "$HOME/.styrman-install/gh-token"` → `cat "${STYRMAN_GH_TOKEN_FILE}"`
in the git credential-helper line. Restore with the inverse `s|cat \"\$HOME/.styrman-install/gh-token\"|cat \"\${STYRMAN_GH_TOKEN_FILE}\"|`
(no line address) — it matches that line AND, unintended, line 153's
`sudo env GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")" ./deploy/styrman-ctl install --yes …`,
leaving it reading `GH_TOKEN="$(cat "${STYRMAN_GH_TOKEN_FILE}")"`.

## Root cause
`STYRMAN_GH_TOKEN_FILE` is set only on the env line of the separate git-helper
`gcloud compute ssh --command` exec; each remote command is its own exec, so
on the VM `GH_TOKEN` would always have been empty after the corrupted
restore — a first install would die at `host-setup.sh` step 7
("FATAL: no GitHub token for the styrman-harness download") after Docker,
Kata and the firewall were already provisioned. Nothing caught it: the ten
tests then in the file drive `--dry-run` plans (the remote shell never
runs), the verifier's two rounds ran the same tests, shellcheck was clean,
and the two real bring-up runs failed earlier (BUG-069, BUG-070) before
reaching that line. Found by the `reviewer` agent
(`.agents/docs/reviews/2026-09-21-review-gcp-compute-engine-target.md`, the
one high finding, verdict REQUEST CHANGES) by simulating the unset variable.

## Fix
The Phase 1 commit restored the install line to
`GH_TOKEN="$(cat "$HOME/.styrman-install/gh-token")"`; a new assertion in
`TestGCPInstallPlan` finds the plan's `styrman-ctl install --yes` line and
requires that literal token path and no `STYRMAN_GH_TOKEN_FILE` in it —
written first, it failed on the corrupted line, passed after the one-line
fix. `go test ./internal/config/ -run TestGCP` ok (11 tests), shellcheck
clean. Later mutations in the same session used byte copies (`cp` to a
scratch backup, `cmp` after restore) and found no drift.

## Prevention
A must-fail mutation is restored from a byte-identical copy (`cp` before,
`cmp` after), never by an inverse edit — an inverse `sed` without a line
address rewrites every match, and the named test going green again proves
only that ITS line was restored. A test that asserts a command string names
every value the command needs from its own process (here: the token path
inside the same ssh command) — an env line on one exec proves nothing about
the next.

## Related
BUG-005/006/007 (`.agents/rules/writing-files.md` — ask git what actually
changed: `git diff` after a restore would have shown the unintended hunk had
the file been tracked; it was untracked). BUG-033/041 (a green signal that
measured something else).
