---
owner: bugs
date: 2026-09-22
id: BUG-079
status: fixed
area: deploy/gcp/vm.sh
---
# BUG-079: styrman-ctl allowlist accepted shell metacharacters; the fix added a round-trip and broke a test

## Symptom
`deploy/gcp/vm.sh`'s styrman-ctl allowlist used `case` with a
`/home/*/styrman-crew/deploy/styrman-ctl` arm; `*` matches `/`, `;`, a
space and `$(` alike, and the admitted string is re-parsed by the remote
shell in `vm_ssh "sudo ${ctl} status"`. Demonstrated real, not
theoretical: against the old arm, `/home/$(id)/...` and `/home/x;id/...`
were accepted. The first fix resolved `$HOME` with a second `vm_ssh`
round-trip and broke `TestGCPVMStatusExitCodes`, because the test's fake
ssh does not answer that probe. A second attempt at the same area then
reworded the refusal message and broke two tests that assert its phrase.

## Repro
Call `vm.sh status` with a crafted path such as `/home/$(id)/styrman-crew/deploy/styrman-ctl`
against the pre-fix `case` arm; it is admitted and re-parsed by the remote
shell.

## Root cause
The allowlist used a shell glob pattern (`case … *`) to validate a string
that is later interpolated into a remote command line; a glob does not
exclude shell metacharacters, so the "allowlist" admitted arbitrary
injected commands. The first repair attempt introduced a second `vm_ssh`
round-trip to resolve `$HOME`, which the test's ssh fake had no fixture
for.

## Fix
Two literal comparisons plus a regex
`^/home/[a-z_][a-z0-9_-]{0,31}/styrman-crew/deploy/styrman-ctl$`, no extra
round-trip. Test: `TestGCPVMStatusRefusesAMetacharacterCtlPath`. The
refusal message's phrase was kept (not reworded) after the second attempt
broke two tests asserting it; the explanation was appended instead.

## Prevention
A fix to a remote-facing check must not add a round-trip the test fakes
do not answer; and a message that tests assert on is extended, never
reworded. Also: an allowlist for a string that will be interpolated into
a shell command is validated with a proper regex (anchored,
character-class), never a shell glob — a glob has no notion of
metacharacter exclusion.

## Related
Same family as `.agents/rules/remote-check-names-transport-first.md`
(BUG-069) and the global hard rule against hand-rolled parsing of
untrusted input. The regex-vs-glob distinction and the "test fakes must
answer every round-trip a fix adds" point are candidates for
`deploy/gcp/lib.sh`'s existing conventions if similar allowlist patterns
recur elsewhere in `deploy/gcp/`.
