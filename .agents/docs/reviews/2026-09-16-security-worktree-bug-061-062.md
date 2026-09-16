---
owner: security
date: 2026-09-16
---
# Security review — worktree-bug-061-062   (2026-09-16)

Range reviewed: working tree vs 8ada624 — .agents/docs/testing/styrman/integration.sh, image/entrypoint.sh

Repository HEAD at review time: `8ada624`, branch `main`. Diff read with
`git diff HEAD -- .agents/docs/testing/styrman/integration.sh image/entrypoint.sh`
(33/4 and 15/8 lines; `git diff --numstat` reports text on both, not `-`/`-`).
Blobs pinned (re-verified unchanged at the end of the review, while other
agents moved BUG-061/062, `bugs/INDEX.md` and the testing README in parallel):
`integration.sh` = `80c25759568ddacc300780ec083a55879198a2d8`,
`image/entrypoint.sh` = `fecae1bde5863dfb738d20bf989802354e27f3ff`.
Every line number cited below was re-resolved against those blobs.
Blast radius followed outward from the changed lines: the installer that
produces every line the new filter can admit (`image/harness/cmd/sh-agent/main.go`
— `attributionSet`, `attributionHint`, `attributionOff`, `readSettings`,
`pathHint`; unchanged by this diff), the entrypoint's `log()` sink, the HTTP
route that serves the boot log to a user (`internal/httpapi/stats.go`
`projectLogs` → `internal/orchestrator/orchestrator.go` `Logs` →
`internal/httpapi/server.go:677` `getProjectFor`), and the one guest-influenced
input to a logged line (the terminal username,
`internal/httpapi/profile.go:28,76`).

## Verdict: FINDINGS

Nothing here is exploitable, and the change is a net improvement to an
operator-facing control: the widened filter surfaces exactly the cases where
the trailer-suppression key is NOT written, and the new Tier 2 checks assert
the file itself rather than only the log. All three findings are low. The one
worth the operator's attention is not a defect in the diff but a limit on what
it can mean: the boot log the new lines land in is written by a PID 1 the
project user is root alongside (`image/entrypoint.sh:55-56` grants that user
passwordless `sudo` inside the microVM), so a `settings: … created with …`
line is a diagnostic, not an attestation. The diff's own Tier 2 checks already
do the right thing by reading the file with `jq`; the risk is a later check, or
a later reader, treating the log line as proof of state.

Positively verified, because the widened filter is the whole change: no line
the new pattern admits can carry file CONTENT or a secret. `readSettings` and
`attributionOff` return a closed set of fixed problem strings ("could not be
read", "is not a regular file", "is not valid JSON", "is not a JSON object")
and are documented as returning them "without a byte of its contents";
`attributionSet` prints only the fixed path plus a Go `*PathError`/`*LinkError`.
`~/.claude/settings.json` is the file that can hold `env`, `apiKeyHelper` and
`permissions` — none of it is echoed. This is not inferred from a narrow grep:
every `fmt.Fprint*(out, …)` in `main.go` was enumerated (44 call sites, lines
71–661), and the complete set the filter can admit is the fourteen `settings:`
lines (495–619), the two `attributionHint` lines (659, 661), and — only in the
edge case of finding 3 — the `pathHint` line (473). `status`'s own note style
is `"note       "` (spaces, line 318), which `^note: ` cannot match.

The only guest-influenced byte in any logged line is the terminal username
inside the path, bounded by `guestNameRe = ^[a-z][a-z0-9_-]{0,31}$`
(`internal/httpapi/profile.go:28`, enforced in `setGuestUser` at :76-82), so no
quote, space or newline can be injected into a log line through it, and the
`tr -cd '[:print:]\n' | cut -c1-200` behind the filter is a second bound.

Filter validated in BOTH states against the captured installer output
(`.agents/scratch/bug-061-062/install-dir-case.txt`, 4 lines, read-only):
the OLD `grep '^settings:'` yields 0 lines (exit 1) — the must-fail state that
proves BUG-062 was real; the NEW `grep -E '^settings:|^note: .*attribution'`
yields exactly the one advisory line, and the installer's PATH-hint `note:` is
correctly excluded. The substring `integration.sh:416` asserts survives
`tr`/`cut` (checked with `grep -F` on the piped output). The advisory line is
226 bytes and IS truncated by `cut -c1-200` mid-`"attribution": {"co` — the
security-relevant half ("… is not a regular file, so the attribution setting
could not be checked") is in the first ~110 bytes and survives, so the
truncation costs legibility only, not meaning. `tr -cd '[:print:]\n'` also
strips the UTF-8 `§` and em dash (seen as `M-BM-'` / `M-bM-^@M-^T` in the
sample) — deliberate sanitisation, cosmetic effect.

## Findings

- **[low / confidence: high]** `image/entrypoint.sh:176` — **the boot log the
  new lines land in is guest-forgeable, so a `settings: … created with …` line
  is evidence only for a cooperating guest.** `image/entrypoint.sh:55-56`
  writes `<user> ALL=(ALL) NOPASSWD: ALL` into `/etc/sudoers.d/`, so the
  project user is root inside its own microVM and can write arbitrary bytes to
  PID 1's stdout (`/proc/1/fd/1`), or replace `/styrman/bin/sh-agent` in the
  container layer before a re-seed. Forged lines are plain ASCII and so pass
  `tr -cd '[:print:]\n' | cut -c1-200` untouched, arriving prefixed exactly
  like a real one. A project that deleted `~/.claude/settings.json` to get its
  Co-Authored-By trailers back can therefore present a boot log that says the
  key was written. This is not an escalation — the guest is already root in its
  own VM, and `projectLogs` (`internal/httpapi/stats.go:126`) resolves through
  `getProjectFor` → `s.who(r)` → `sp.projects.Get`, i.e. the caller's OWN
  per-user project store, so no other tenant can read the line — but the
  control this change adds is a diagnostic, not an attestation of the ADR-072
  state. Remediation: no code change; say so where the mechanism is described
  (BUG-062 and the entrypoint comment), so no later check substitutes the log
  line for the file. The diff already gets this right in Tier 2 —
  `integration.sh:147` and `:461` assert the FILE with `jq`, not only the log —
  and that pairing should be stated as the rule rather than left as a
  coincidence of these two checks.

- **[low / confidence: medium]** `.agents/docs/testing/styrman/integration.sh:147`,
  `:418`, `:461` — **two of the new in-guest reads run as container root on
  paths under a guest-writable home, while the two mutating ones correctly drop
  to the project user.** `docker exec ${CN} runuser -u ${ME} -- sh -c 'rm -f …
  && mkdir …'` (`:405`) and the undo (`:420`) use `runuser`; the `jq -e …
  ${GH}/.claude/settings.json` checks (`:147`, `:461`) and
  `sh -c 'test "$(cat ${GH}/.styrman/.harness-version)" = …'` (`:418`) do not,
  and `docker exec` with no `-u` is root. `.agents/rules/guest-writable-paths.md`
  is explicit that every component below a mounted home may be a symlink and
  that a root read follows it. Concretely this is NOT exploitable in this
  suite: the guest is the suite's own freshly created project, it is root in
  its VM anyway, and `jq -e`'s output goes to `/dev/null` so at most an exit
  status is observable. It is recorded because the shape is the BUG-002/003
  family and it diverges from the convention two lines away. Remediation:
  prefix the two reads with `runuser -u ${ME} --`, matching `:405`/`:420` and
  the existing `:174`.

- **[low / confidence: high]** `image/entrypoint.sh:176` — **the filter's
  `^note: .*attribution` arm matches on the PATH, not only the message, so it
  admits the installer's unrelated PATH-hint note for one legal username.**
  `pathHint` prints `note: <home>/.local/bin is not on PATH; …`
  (`image/harness/cmd/sh-agent/main.go:473`), and `guestNameRe`
  (`internal/httpapi/profile.go:28`) permits a terminal username of
  `attribution`, making `<home>` = `/home/attribution` and the hint match the
  pattern the comment says it excludes. Effect: one extra advisory line in that
  project's own boot log per re-seed. No disclosure, no escalation, no
  cross-tenant reach — this is filter precision, not a security weakness, and
  is listed so the comment's claim ("Its other note, the PATH hint printed on
  every seed, stays out") is not read as unconditional. Remediation (optional):
  anchor on the message instead of anywhere-in-line, e.g.
  `^note: .*attribution setting could not be checked|^note: .*does not set "attribution"`,
  which covers both `attributionHint` branches and cannot be reached by a path.

## Checked

- **Secrets** — no credential, token or key added; nothing echoed into a log.
  The widened filter cannot carry file content by construction (see Verdict —
  all 44 installer stdout sites enumerated). `git diff` carries no `.env`,
  `.pem`, `.key`, `.tfvars` or credential JSON; no secret-shaped literal in
  either changed file.
- **Injection** — the entrypoint's filtered lines reach `log()` as
  `echo "[styrman] $*"` with a leading literal, so no `-e`/`-n` option
  confusion and no escape interpretation (bash `echo` without `-e`); the
  `while IFS= read -r line` loop does no word splitting or globbing. The only
  guest-influenced substring is the username, bounded to `^[a-z][a-z0-9_-]{0,31}$`
  — no newline, so log-line forgery *through the path* is not reachable (the
  forgery in finding 1 is a different, in-VM-root route). In `integration.sh`,
  `${CN}`, `${ME}`, `${GH}` are interpolated into strings run through
  `check(){ eval "$2"; }` and then an inner `sh -c '…'`; `ME="$(id -un)"` and
  `GH="/home/${ME}"` come from the operator's own OS account and
  `CN="styrman-${ID}"` from a server-issued UUID matched by `[0-9a-f-]{36}`, so
  no untrusted byte reaches the `eval`. `eval` in `check` is pre-existing and
  unchanged.
- **Authz/authn** — no auth path touched. The one authorization question the
  diff raises is who can read the newly-logged lines: `projectLogs`
  (`internal/httpapi/stats.go:126`) calls `s.getProjectFor(r)`
  (`internal/httpapi/server.go:677`), which authenticates with `s.who(r)` and
  then looks the project up in that caller's own `userSpace` store — a project
  ID belonging to another user is simply absent, answered 404. `Orchestrator.Logs`
  caps output at 256 KiB through a tail buffer (guest controls line length).
  So the new lines are visible to the project's own owner (already root in that
  VM) and to the host operator, and to nobody else; the path in the line is the
  reader's own home, so there is no cross-tenant identifier disclosure either.
- **Crypto** — nothing hand-rolled, nothing consumed. No primitive, MAC,
  token, session, key derivation or auth flow is implemented or composed in
  either changed file. The attribution key the change reports on is a CLI
  setting, not a credential.
- **Dependencies** — none added or changed: `git diff HEAD --stat` over
  `go.mod`, `go.sum`, `web/package.json`, `web/package-lock.json`,
  `image/harness/go.mod` and `deploy/caddy/ingress/go.mod` is empty. `jq` is
  newly used in-guest at `integration.sh:147`/`:461` but is already installed
  in the runtime image (`image/Dockerfile:26`) and already used in-guest at
  `integration.sh:159`, so it is not a new dependency. `govulncheck` and
  `npm audit` were NOT run: no Go or JS dependency is in scope, and the repo's
  own entry point (`bash deploy/vulncheck.sh`) builds a pinned Docker image
  over both modules — running it would attest nothing about this diff. A
  deliberate skip, not a tool failure.
- **GCP surface** — none. Self-hosted platform (`.agents/docs/deploy/gcp.md` is
  "not applicable"); no IAM binding, service account, `--allow-unauthenticated`,
  Secret Manager path or service-account key file is touched.
- **ADR-052 / scratch discipline** — the new Tier 2 checks mutate a guest home
  (`rm -f`, `mkdir`, `rmdir`), which is the shape ADR-052 exists for. They are
  safe: every one goes through `docker exec ${CN}` on the container the suite
  itself created, whose home is the bind under
  `STYRMAN_STATE_DIR="$(mktemp -d /tmp/styrman-itest-XXXX)"` — it cannot alias
  `/var/lib/styrman` or `/etc/styrman`. (The suite's use of `/tmp` rather than
  `.agents/scratch/` is pre-existing at line 38 and outside this diff;
  `mktemp -d` gives mode 0700, and `set -e` aborts before the
  `sudo rm -rf "${STATE}"` trap is installed if it fails.)
- **Verification integrity of the new checks** (BUG-034/BUG-040 family) — the
  re-seed assertion at `:461` greps `docker logs ${CN}` for a line that can only
  come from the boot under test: logs start fresh after the `docker rm -f` at
  `:406`, and the boot that follows produces the `note:` line, never
  `created with` (the plant is a directory), so the `created with` match at
  `:461` must come from the post-`boot=1` re-seed. `grep -F … >/dev/null`
  rather than `grep -q` matches the project's BUG-057 rule; `set -o pipefail`
  is active and `check` evaluates inside an `if`, so a failing `docker logs`
  fails the check closed rather than passing silently. The `rmdir` at `:420`
  also doubles as an assertion that the installer wrote nothing inside the
  planted directory.
- **Pre-existing, unchanged, noted for context** — project containers are
  created by the orchestrator with no `LogConfig`/`max-size` (`grep` over
  `internal/orchestrator/` and `deploy/docker-compose.yml` finds none), so
  host-side capture of guest-driven PID 1 output is bounded only by the daemon
  default. (The `log-opts: max-size 10m` in `image/entrypoint.sh:305` is the
  NESTED daemon's config and does not cover this.) The diff adds at most a few
  lines per re-seed and does not move this; not counted as a finding of this
  range. Likewise the installer's stderr still bypasses the filter unsanitised
  into `docker logs` — unchanged from 8ada624 and already the shape the
  4aa19be review's finding 2 accepted.

## Not examined

- **Nothing in the diff itself.** Both changed files were read end to end:
  `.agents/docs/testing/styrman/integration.sh` (all 485 lines) and
  `image/entrypoint.sh` (lines 1–400 of 401; line 401 is the trailing
  `wait $!` of the shutdown park).
- `image/harness/cmd/sh-agent/main.go` — read lines 440–700 in full
  (`pathHint`, `attributionSet`, `abandonTemp`, `readSettings`,
  `attributionHint`, `attributionOff`) plus every `fmt.Fprint*(out, …)` call
  site in the file by enumeration (lines 71–661) and the `install` dispatch at
  :167. Lines 1–439 and 700–end were NOT read line by line, so claims about the
  installer's *stdout surface* are complete by enumeration, while claims about
  its other behaviour are not.
- `image/harness/cmd/sh-agent/main_test.go` — only the attribution-related grep
  hits were read. The mirror's own test suite was NOT run: `image/harness` is a
  separate Go module, excluded from the root `go test ./...`.
- **The suite was NOT executed.** No Tier 2 run, no live boot, no `docker logs`
  observation on a real project: every claim about the new checks comes from
  reading them plus replaying the filter against the captured installer output
  in `.agents/scratch/bug-061-062/install-dir-case.txt`. The forged-boot-log
  mechanism in finding 1 was reasoned from `image/entrypoint.sh:55-56`, not
  demonstrated — deliberately, since demonstrating it would mean writing an
  exploit.
- `govulncheck` and `npm audit` not run (no dependency in scope — see Checked).
  `deploy/vulncheck.sh` not run.
- `.agents/docs/testing/styrman/ensure-test-image.sh` (unchanged, invoked at
  `integration.sh:20`) not read.
- `.agents/docs/testing/2026-09-16-verify-bug-061-062.md` (untracked) and the
  dirty testing README are outside the given scope and were not read.
- `internal/httpapi/stats.go`, `internal/orchestrator/orchestrator.go`,
  `internal/httpapi/server.go` and `internal/httpapi/profile.go` were read only
  around the functions named above (`projectLogs`, `Logs`, `getProjectFor`,
  `guestNameRe`/`setGuestUser`). `s.who(r)` (server.go:670) was read and reads
  identity from the request context, so the authn half of the owner-scoping
  claim depends on the ingress auth middleware that populates that context —
  which is outside this diff and was NOT reviewed here.

## Outcome

- Finding 1 (low) — fixed: the entrypoint comment above the filter now says the logged lines are a diagnostic, not an attestation (the project user is root in its VM and can write to PID 1's stdout), and that a check needing the fact reads `~/.claude/settings.json` itself, as Tier 2 does (`image/entrypoint.sh`, the comment ending "(Tier 2 does)").
- Finding 2 (low) — fixed: the three in-guest reads run as the project user — `runuser -u ${ME} --` on the jq assertion in the harness section, the stamp comparison and the jq assertion in the re-seed check (`.agents/docs/testing/styrman/integration.sh`, the checks "settings.json holds exactly the attribution key, trailers off (ADR-072)", "the seed still completed around the refused key (stamp rewritten)", "re-seed after the BUG-062 undo recreates settings.json with the key"). The pre-existing root read of `~/.claude.json` in the browser section is unchanged (out of scope).
- Finding 3 (low) — fixed: the pattern is `^settings:|^note: .*(attribution setting could not be checked|does not set "attribution")`, anchored on the hint's wording; validated again in both states (the real directory-case output and a synthetic "does not set" line pass; the PATH hint with `/home/attribution/` in its path does not).
- The fixes were re-checked bare by the main agent (`bash -n`, `shellcheck -S warning` on both files) and by one more full Tier 2 run on the final tree (`.agents/scratch/bug-061-062/integration-run2.txt`); the release gate's verifier and reviewer rows over the fix commit are the records for the final tree. Round 1 closed 2026-09-16.
