# Verification — project-create provisioning, re-run after review remediation   (2026-09-02)

Scope: 503fc8a..working-tree (re-run after review remediation)
Command used to view the diff: `git diff HEAD -- internal web/src web/e2e`
(diff sha1, tracked paths only, computed via `git diff HEAD -- internal
web/src web/e2e | sha1sum | cut -c1-12`): `2d6539357c6b`. Two files in scope
are new and untracked (`internal/httpapi/provision.go`,
`internal/httpapi/provision_test.go`, 617 diff lines against `/dev/null`
when marked intent-to-add for inspection, then un-staged again — see
Environment) and so do not appear in that hash; they are reviewed in full
below from their on-disk content.

## Status: FAIL — narrower coverage gap than the prior run, but not closed

This is a re-run of `.agents/docs/testing/2026-09-02-verify-project-create-provisioning.md`,
which FAILED on coverage. Every command passes clean (no regressions), and
the coverage gap has shrunk substantially — the entire Go-side regression
this diff exists to fix (attach-before-provisioning ordering, the broker
wait, the login-shell provisioning switch, the `provisionErrs` plumbing) is
now directly and thoroughly tested. What remains uncovered is narrower but
still real, named below, and per the verification contract ("changed code
without covering tests = FAIL, naming each uncovered package/file") that is
still a FAIL, not a PASS with caveats.

## Commands run

- `go build ./...` → exit 0, no output.
- `go vet ./...` → exit 0, no output.
- `go test -race -count=1 ./...` (forced fresh, not relying on the build
  cache) → exit 0, all `ok` or `[no test files]`:
  ```
  ok  	github.com/kesokaj/styrman-crew/cmd/styrman	1.029s
  ?   	github.com/kesokaj/styrman-crew/cmd/wsprobe	[no test files]
  ok  	github.com/kesokaj/styrman-crew/internal/config	1.012s
  ok  	github.com/kesokaj/styrman-crew/internal/conversation	1.022s
  ok  	github.com/kesokaj/styrman-crew/internal/credbroker	1.027s
  ok  	github.com/kesokaj/styrman-crew/internal/credstore	1.017s
  ok  	github.com/kesokaj/styrman-crew/internal/files	1.019s
  ok  	github.com/kesokaj/styrman-crew/internal/gitrepo	1.027s
  ok  	github.com/kesokaj/styrman-crew/internal/history	1.014s
  ok  	github.com/kesokaj/styrman-crew/internal/httpapi	1.160s
  ?   	github.com/kesokaj/styrman-crew/internal/identity	[no test files]
  ok  	github.com/kesokaj/styrman-crew/internal/links	1.027s
  ok  	github.com/kesokaj/styrman-crew/internal/orchestrator	1.023s
  ok  	github.com/kesokaj/styrman-crew/internal/project	1.014s
  ok  	github.com/kesokaj/styrman-crew/internal/terminal	1.015s
  ok  	github.com/kesokaj/styrman-crew/internal/userstore	1.010s
  ?   	github.com/kesokaj/styrman-crew/web	[no test files]
  ```
- `go test -race -count=1 -v ./internal/httpapi/... -run 'Provision|WaitForBroker'`
  → exit 0, all 9 named tests PASS individually:
  ```
  --- PASS: TestProvisionRepoAttachesBeforeAnyCredentialedCommand (0.00s)
  --- PASS: TestProvisionRepoClonesThroughALoginShell (0.00s)
  --- PASS: TestProvisionRepoSkipsBrokerWaitForPlainInit (0.00s)
  --- PASS: TestProvisionRepoCreatesGitHubRepo (0.00s)
  --- PASS: TestProvisionRepoRecordsCloneFailure (0.01s)
  --- PASS: TestProvisionRepoRecordsNothingOnSuccess (0.00s)
  --- PASS: TestWaitForBroker (0.01s)
  --- PASS: TestProvisionErrorLifecycle (0.00s)
  --- PASS: TestProvisionErrorSurfacesInTheProjectAPI (0.01s)
  PASS
  ok  	github.com/kesokaj/styrman-crew/internal/httpapi	1.063s
  ```
- `go test -cover ./internal/httpapi/... ./internal/orchestrator/...` →
  `ok github.com/kesokaj/styrman-crew/internal/httpapi 0.097s coverage: 39.9% of statements`
  (up from 36.9% in the prior run), `ok
  github.com/kesokaj/styrman-crew/internal/orchestrator (cached) coverage: 7.2%
  of statements` (unchanged — `guestHostname` is a small fraction of that
  package's total statements; it is directly tested regardless, see below).
- `(cd deploy/caddy/ingress && go test ./...)` (separate module) →
  `ok github.com/spgo/devbox/caddy/ingress (cached)`, exit 0.
- `gofmt -l internal/httpapi/ internal/orchestrator/` → exit 0, no output
  (both dirs gofmt-clean).
- `cd web && npm run check` (tsc -b) → exit 0, no output.
- `cd web && npm run lint` (oxlint) → exit 0, no output.
- `cd web && npm run build` (`vite build && tsc -b`) → exit 0. Full SPA +
  SSR build succeeded, prerendered `/`, emitted content-hashed assets
  (matches the untracked `web/dist/client/assets/*` churn already visible
  in `git status`). No errors or warnings in the build output.
- `node --check web/e2e/run.mjs` → exit 0, no output (syntax-valid).
- Dockerfile check: `git diff HEAD --name-only | grep -i dockerfile` → no
  match, exit 1 (grep-not-found). No image build performed — correctly out
  of scope per task instructions.
- `grep -n '"check"\|"lint"\|"build"\|"dev"\|"preview"\|vitest\|jest\|testing-library' web/package.json`
  → confirms `web/`'s only scripts are `dev`/`build`/`lint`/`preview`/`check`;
  no unit-test framework dependency. Matches the testing guide's own note.

## What changed (from the diff, `internal web/src web/e2e`)

Same substance as the prior run's summary, now with the provisioning
sequence extracted to a new file:

1. **NEW** `internal/httpapi/provision.go` — `provisionSpec` (with
   `attach func() bool` and `run func(...string)(int,string,error)`
   injected for testability without Docker), `provisionRepo` (attach →
   `waitForBroker` → clone-or-github, in that order — this ordering is the
   actual regression fix), `waitForBroker` (polls
   `${STYRMAN_CRED_BROKER}/healthz` via a PLAIN shell — not login, so the
   probe itself never materializes credentials — 10× `s.brokerProbeInterval`,
   ctx-aware), `setProvisionError`/`provisionError`/`projectExists`. Clone
   and GitHub-create now run via `bash -lc` (login shell, so
   `/etc/profile.d/zz-styrman-credentials.sh` fires) with `shellQuote()` on
   the clone URL and the project name.
2. `internal/httpapi/server.go` — `provisionErrs` map (mirrors the existing
   `deleteErrs` pattern), `ProvisionError` field on `projectView` (surfaced
   from `listProjects`/`getProject`, cleared on delete), `guestErrMessage`
   (condenses guest command output to an actionable, redacted, length-capped
   line), `redactSecrets` (replaces the deleted `redactURL` — the old
   positional last-`@` scan was lossy: it mangled `o/r@v1.git` and only
   caught one URL per line), `stripControls`/`clampBytes` (extracted from
   `clampTitle`, now shared with `guestErrMessage`; `stripControls` now also
   strips the C1 block and Unicode bidi/invisible-formatting controls, not
   just C0/DEL).
3. `internal/httpapi/server_test.go` — `TestRedactSecrets` (multi-URL,
   scp-style-is-not-a-secret, first-`@`-vs-last-`@` edge case),
   `TestStripControlsAndClampBytes` (C1/bidi/zero-width stripping, exact-fit
   and multi-byte-rune-safe truncation), `TestGuestErrMessage` (fatal-line
   selection, ssh-diagnosis selection, redaction, length cap, control-byte
   stripping).
4. **NEW** `internal/httpapi/provision_test.go` — 9 tests (all read and
   individually run above): the attach-before-any-credentialed-command
   ordering (the actual regression, asserted step-by-step:
   `attach → /healthz probe → clone`, with an explicit check that the probe
   and the clone never collapse into one step), the clone running via
   `bash -lc` with the URL quoted after `--` and TOFU
   (`StrictHostKeyChecking=accept-new`) preserved, plain-init skipping the
   broker wait entirely (asserts the step list is `["attach"]` only), both
   GitHub visibilities (login-shell commit + `gh repo create` with the
   quoted name), clone-failure recording the reason AND leaving an R13 repo
   AND not leaking across users, success recording nothing,
   `waitForBroker` retry/give-up/context-cancel, the `provisionError`
   lifecycle including the "never resurrect a message for a deleted or
   vanished project" contract (`projectExists` gate +
   deleting-in-progress gate), and `TestProvisionErrorSurfacesInTheProjectAPI`
   driving real HTTP through `POST /api/projects`, `GET /api/projects`,
   `GET /api/projects/{id}` and `DELETE /api/projects/{id}` end-to-end
   through `s.Routes()`.
5. `internal/orchestrator/orchestrator.go` — `guestHostname(name)` (unchanged
   from the prior run's summary).
6. `internal/orchestrator/orchestrator_test.go` — `TestGuestHostname`
   (unchanged from the prior run's summary; still the only test in that
   file besides the pre-existing `TestRestartPolicy`).
7. `web/src/lib/api.ts` — `projectNameFromRepoURL()` (unchanged from the
   prior run's summary). `web/src/components/NewProjectDialog.tsx` —
   clone-flow name auto-derives until hand-edited (unchanged).
   `web/src/components/Sidebar.tsx` — new "repo setup failed" badge, gated
   on `p.provisionError && !p.deleteError && p.state !== "deleting"`
   (unchanged from the prior run's summary — this file did not gain any new
   test coverage in this round).
8. `web/e2e/run.mjs` — **NEW** 2 Playwright checks, added between the home
   screenshot and the real project-create flow: "clone mode derives the
   project name from the repo URL" (fills the clone URL, asserts `#np-name`
   becomes `styrman-harness` via `waitForFunction`) and "a name the user
   types overrides the derived one" (hand-types a name, then changes the
   URL, asserts the hand-typed name survives). Both close the dialog with
   `Escape` rather than creating a project, so neither can auto-SKIP. Tier 4
   count updated 55→57 in `testing/styrman/README.md` accordingly (confirmed
   by reading that file — the "Tier 4 — Browser e2e" section states "57
   checks + a final client-error gate" and names these two explicitly).

## Coverage of changed code

### Closed since the prior FAIL (all seven of the prior run's Go-side gaps)

- `waitForBroker()` — was "zero test coverage"; now `TestWaitForBroker`
  covers retry-then-succeed (3 calls), give-up-when-never-answers, and
  ctx-cancellation early exit. **Closed.**
- The attach-before-provisioning reorder itself — was "no test that
  asserts the goroutine calls `EnsureSelfAttached` before `run(...)`"; now
  `TestProvisionRepoAttachesBeforeAnyCredentialedCommand` asserts the exact
  step order (`attach`, then the `/healthz` probe, then `clone`) via an
  injected recorder, and explicitly guards against the probe and the clone
  collapsing into a single step. This is a real regression test: reverting
  the reorder in `provisionRepo` would fail `steps[0] != "attach"`
  immediately. **Closed.**
- `setProvisionError`/`provisionError` and the `ProvisionError` plumbing
  through `listProjects`/`getProject`/`deleteProject` — was "no direct unit
  test"; now `TestProvisionErrorLifecycle` (set/clear, resurrection guard
  for a vanished project, resurrection guard for a project mid-delete) and
  `TestProvisionErrorSurfacesInTheProjectAPI` (real HTTP through create,
  list, get-by-id, and delete — confirms `omitempty` keeps a healthy
  project's JSON free of the field, confirms the message is visible on both
  list and single-project GET, confirms delete clears it within 5s). **Closed,
  and more thoroughly than the pattern it mirrors (`deleteErrs`) is tested
  elsewhere in this package.**
- The `bash -lc` login-shell switch and `shellQuote()` on the clone URL and
  project name — was "no test constructs a project name or clone URL with
  shell metacharacters and asserts the resulting command line is safe"; now
  `TestProvisionRepoClonesThroughALoginShell` asserts the exact command
  string (`bash -lc "git ... clone -- 'git@github.com:o/r.git' ."`, `-lc`
  prefix, TOFU flag present) and `TestProvisionRepoCreatesGitHubRepo`
  asserts the same for the login-shell commit and the quoted
  `gh repo create 'my-proj' --public|--private ...` line. **Closed** for the
  shape and quoting this diff introduces. (A dedicated shell-metacharacter
  adversarial case — e.g. a clone URL or name containing a single quote —
  is still not exercised; `shellQuote()` itself is pre-existing code from
  before this diff and out of this diff's scope, so this is a note, not a
  new gap charged to this change.)

### Still uncovered — named explicitly, as instructed

- `web/src/lib/api.ts` `projectNameFromRepoURL()` and
  `web/src/components/NewProjectDialog.tsx`'s name-auto-derive-until-edited
  behavior — **`web/` still has no unit-test framework** (confirmed again
  this run: `package.json` scripts are only `dev`/`build`/`lint`/`preview`/
  `check`; no `vitest`/`jest`/`testing-library` dependency). Unlike the
  prior run, this logic now DOES have authored coverage: the two new
  Playwright checks in `web/e2e/run.mjs` (`"clone mode derives the project
  name from the repo URL"`, `"a name the user types overrides the derived
  one"`) target exactly this function and this component behavior, using
  the real `#np-name` input and the `aria-label="Repository URL to clone"`
  selector — both confirmed present in `NewProjectDialog.tsx` by direct
  `grep`. **However, Tier 4 (`web/e2e/run.mjs` via `web/e2e/run.sh` or a
  standalone `node web/e2e/run.mjs --base ...`) was NOT executed in this
  session** — it needs a live server plus a working Kata (`mvm`) runtime to
  boot a real microVM for the rest of the suite around these two checks,
  and running it was out of scope for this pass (not in the command list
  given, and no live/isolated server instance was started). So as of this
  verification run, these two checks exist in the tree and are
  syntax-valid (`node --check web/e2e/run.mjs` passed) but their PASS/FAIL
  status is unconfirmed — I did not observe them run.
- `web/src/components/Sidebar.tsx` — the new "repo setup failed" badge
  (`p.provisionError && !p.deleteError && p.state !== "deleting"`) has
  **zero test coverage of any kind**, unchanged from the prior FAIL: no
  unit test (no framework), and `grep -n
  "provisionError\|repo setup failed\|setProvisionError" web/e2e/run.mjs`
  returns nothing — the two new Tier 4 checks added this round cover only
  the New-project dialog's name derivation, not this badge. Note for
  context, not mitigation: the sibling "delete failed" badge this pattern
  is copy-pasted from (`p.deleteError`, same file) also has no e2e check
  (`grep -n "delete failed\|deleteError" web/e2e/run.mjs` likewise returns
  nothing) — so this gap is consistent with an existing, already-accepted
  convention in this codebase rather than a new regression uniquely
  introduced here, but it is still, per the verification contract, changed
  code with no automated coverage.

### Go-side coverage confirmed complete for the diff's core surface

- `internal/orchestrator/orchestrator.go` `guestHostname()` — covered by
  `TestGuestHostname` (prefix removal, pass-through, trailing-dash trim,
  empty→"project" fallback). Unchanged from the prior run, still good.
- `internal/httpapi/server.go` `guestErrMessage()`, `redactSecrets()`,
  `stripControls()`, `clampBytes()` — all covered by
  `TestGuestErrMessage`, `TestRedactSecrets`, and
  `TestStripControlsAndClampBytes` respectively, all read and confirmed
  present in `server_test.go` above.

## Verdict

Every command the task specified ran clean: `go build`, `go vet`,
`go test -race -count=1` (root module, forced fresh), the separate Caddy
ingress module's `go test`, `npm run check` (tsc), `npm run lint` (oxlint),
`npm run build` (full SPA+SSR build with prerender), and
`node --check web/e2e/run.mjs`. `gofmt -l` on both changed Go directories is
clean. No Dockerfile changed, so no image build was required or performed.

Coverage has measurably and substantially improved: every Go-side gap named
in the prior FAIL — the attach-before-provisioning ordering (the actual
regression), `waitForBroker`, the login-shell/`shellQuote` provisioning
path, and the `provisionErrs` plumbing through the API — is now directly,
specifically tested, including a real HTTP-level test
(`TestProvisionErrorSurfacesInTheProjectAPI`) and a step-order regression
test that would catch a reintroduction of the original bug
(`TestProvisionRepoAttachesBeforeAnyCredentialedCommand`).

Two frontend gaps remain, one narrowed and one unchanged:

1. `projectNameFromRepoURL()` / `NewProjectDialog`'s auto-derive behavior
   now has authored Tier 4 coverage, but **Tier 4 was not executed this
   session**, so that coverage is unverified as of this run.
2. `Sidebar.tsx`'s new "repo setup failed" badge has **no coverage of any
   kind**, unchanged from the prior FAIL.

Per the verification contract ("changed code without covering tests = FAIL,
naming each uncovered package/file" — no exceptions), item 2 alone is
sufficient to keep this **FAIL**. This is a narrower, largely closed gap
compared to the prior run — the core bug-fix logic this diff exists for is
now well-tested — but a real, changed, user-visible piece of this diff
(the Sidebar badge) still ships with zero automated coverage, and Tier 4's
non-execution this session means the authored coverage for the other
frontend gap is unconfirmed rather than closed.

**Overall: FAIL — coverage gap, narrower than the prior run.** Not a red
suite: every command run above passed. The fail reason is exclusively (a)
`Sidebar.tsx`'s provisionError badge having no test of any kind, and (b)
Tier 4 not having been executed this session to confirm the two new
Playwright checks that do target the other remaining gap.

## Environment

- Tier: local only (no remote/GCP access used or requested).
- OS: Linux 6.12.107+deb13-amd64 (native, not Colima).
- Go: root module (`github.com/kesokaj/styrman-crew`) and the vendored
  Caddy ingress module (`deploy/caddy/ingress`, module
  `github.com/spgo/devbox/caddy/ingress`) tested independently as two
  separate modules, per `.agents/docs/testing/styrman/README.md`.
- Frontend: `web/node_modules` was already present (not reinstalled).
- Two files in scope, `internal/httpapi/provision.go` and
  `internal/httpapi/provision_test.go`, were untracked (`git status` shows
  `??`) rather than modifications to a tracked file, so
  `git diff HEAD -- internal web/src web/e2e` alone does not show their
  content. They were marked intent-to-add (`git add -N`) briefly to produce
  a full diff for review purposes only (no content was staged or changed),
  then un-staged again with `git reset --` immediately after — confirmed
  by `git status --porcelain` before and after showing `??` in both cases.
  No file content was written or edited at any point in this verification.
- Production containers (`styrman-runtime:1.17.3`, `styrman-caddy:1.17.3`,
  `styrman-server:1.17.3`) were running throughout on this host but were
  not touched, restarted, or used by any command above; no e2e/Playwright
  process was found running (`ps aux | grep playwright` empty), confirming
  Tier 4 was not exercised in this session.
