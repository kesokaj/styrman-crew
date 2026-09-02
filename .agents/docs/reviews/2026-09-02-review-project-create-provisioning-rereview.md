# Review — project-create provisioning, re-review after remediation   (2026-09-02)

Scope: `503fc8a..working-tree (re-review)` — `git diff HEAD -- internal web/src web/e2e`
(diff sha1 `2d6539357c6b`) **plus the two untracked files the diff cannot show**,
`internal/httpapi/provision.go` and `internal/httpapi/provision_test.go`.
Combined content sha1 (diff + both untracked files): `8c872456aede`;
`provision.go` md5 `1c497b1299a4…`, `server.go` md5 `0c2d982f3aaf…`.
Out of scope: `web/dist/**` (build output), `.claude/agent-memory/**` deletions,
`.agents/docs/**`, `deploy/**`.

Supersedes `.agents/docs/reviews/2026-09-02-review-project-create-provisioning.md`
(4 mediums + 8 lows) and cross-checks
`.agents/docs/reviews/2026-09-02-security-project-create-provisioning.md` (6 findings).

## Verdict: APPROVE WITH NITS

All four mediums from the first review and all three in-scope security findings are
genuinely closed in code, not merely relocated — I re-derived each one from the source
rather than from the summary, and re-ran the regex against every case asked for (results
below). The worst thing left is not a defect in the new code: it is a **deferral that is
narrower than it looks**. The security review's headline sequence — a private key and two
OAuth tokens stranded in a project home after a non-wipe delete — was blamed on the
readiness probe, and the probe is fixed, but the clone and the initial commit still run
`bash -lc` **login** shells, so a clone/GitHub create is *still* the first login shell a new
project runs and *still* materializes `~/.ssh/id_ed25519`, gcloud ADC and the agent tokens
into the project home seconds after the 201, before the user opens any terminal. The fix
turned ten materializations into one; it did not remove the exposure, and the deferral note
must say so. Behind that: M4 is only 90% closed — `redactURL` is gone but its doc comment
is not, and now sits on `var userinfoRe`, so the file has **two** godoc comments naming
symbols that do not exist (the second, `guestErrMaxBytes`, is newly introduced here). And
the four deferrals are **not** in `TODO.md` — that file is unmodified in this tree; the only
record is two untracked review documents. `go vet`, `gofmt -l`, `go test ./internal/httpapi
./internal/orchestrator` and `tsc -b` are all clean, re-run against this tree.

### Item-by-item confirmation of the nine claimed fixes

| # | Claim | Verdict |
|---|---|---|
| 1 | M1 boot-failure silence on the GitHub path | **CLOSED.** `server.go:734-747` switches on `CloneURL` / `GitHub`; `EnsureRepo` stays under the clone arm, correctly, because the GitHub path was already `git init`'d before the 201 (`server.go:687-697`). I traced every `return` in `createProject`'s goroutine and `provisionRepo`: none exits without either a `setProvisionError` or a success. |
| 2 | M2 `guestErrMessage` split into `stripControls` + `clampBytes` | **CLOSED.** Ellipsis logic is right: strip first, measure after (`server.go:571-574`), so stripped characters can no longer fake a truncation. `clampBytes` is rune-safe and cannot return `""` because `strings.Map` normalizes invalid guest bytes to U+FFFD first. `clampTitle`'s observable change is a strict widening of the stripped set — see finding 4 for the one gap and finding 9 for the one behavioural cost. |
| 3 | M3 `waitForBroker` host-mode guard | **CLOSED.** `provision.go:181` returns false immediately when `SelfContainer == ""`, and `provision.go:113` suppresses the warning. The ~30-35 s host-mode penalty is gone. |
| 4 | M4 `redactURL` deleted | **CLOSED IN CODE, NOT IN COMMENT** — see finding 3. |
| 5 | `attach` returns `bool`, delay comment corrected | **CLOSED.** `provision.go:105-107` logs the `fresh` return; the comment at `:91-95` now describes the real mechanism (the client's first terminal request serialises on the same boot lock). The claim is consistent with `EnsureRunning` holding the per-project lock through `waitReady`. |
| 6 | server.go split | **CLOSED.** `server.go` 1308 → 1181, `provision.go` 202, `provision_test.go` beside it. Partial, though — see finding 8. |
| 7 | Probe is a plain `sh -c`, not a login shell | **CLOSED, and the reasoning is sound as far as it goes** — see finding 1 for what it does *not* cover, and findings 10-11 for two weaker points in the probe itself. |
| 8 | `userinfoRe` splits at the last `@` of the authority | **CLOSED. Verified empirically**, not by inspection — I compiled the regex and ran all five requested cases plus ten more (results below). Go's `regexp` is RE2 but implements leftmost-first submatch semantics with greedy preference, so `[^\s/]*@` does resolve to the last `@` of the authority. |
| 9 | `setProvisionError` drops writes for deleted/deleting projects | **CLOSED for the reported leak; a much narrower window remains** — see finding 5. No deadlock is possible today: `projectExists` runs *outside* `deletingMu`, and `space()` releases `spacesMu` before returning, so `deletingMu` is never held across another lock. |

Regex verification (compiled and executed, not reasoned about):

```
"https://user:AA@BB@github.com/o/r"  -> "https://***@github.com/o/r"   ✅ last @, tail of password gone
"https://github.com/o/r@v1.git"     -> unchanged                       ✅ @ in path is not userinfo
"ssh://git@host/o/r"                -> "ssh://***@host/o/r"            ✅
"git@github.com:o/r.git"            -> unchanged                       ✅ scp-style username stays readable
"…https://u:t1@h/x and https://u:t2@h/y…" -> both redacted             ✅ every occurrence
"HTTPS://USER:TOK@GITHUB.COM/x"     -> "HTTPS://***@GITHUB.COM/x"      ✅ (?i)
"https://user:tok@github.com:8443/x"-> "https://***@github.com:8443/x" ✅ port kept
"git://user:tok@host/x"             -> "git://***@host/x"              ✅
```

**Is `net/url.Parse` required?** No, not here. The redactor's main input is multi-line
guest *output*, which is not a URL and cannot be parsed; a regex is the correct tool for
that job and this one is correct for every realistic shape. Where `net/url.Parse` *is* the
right tool is one layer up, at the API boundary — `cloneURLRe` should reject
userinfo-bearing URLs outright (finding 2), which deletes this whole class instead of
scrubbing it downstream.

## Findings

- **[medium / confidence: high]** `internal/httpapi/provision.go:119-158` +
  deferral #3 ("the project home surviving a non-wipe delete") — **the probe fix narrowed
  the credential-at-rest exposure; it did not close it, and the deferral note reads as if it
  did.** The clone (`:125-127`) and the initial commit (`:140-141`) both run `bash -lc`,
  deliberately and correctly, precisely so `/etc/profile.d/zz-styrman-credentials.sh`
  materializes `~/.ssh/id_ed25519`, gcloud ADC and the agent OAuth tokens into the
  brand-new per-project home. For every `cloneUrl` or `github:` create that is therefore
  still the first login shell the project ever runs, ~1-6 s after the 201 and before the
  user has opened a terminal. The security review's realistic sequence survives intact:
  create a clone project → clone fails → user deletes it *without* ticking wipe →
  `deleteProject` removes the record but not `ProjectHome` (`server.go:852-873`, the
  `RemoveAll` loop is inside `if wipe`) → private key + two OAuth credentials sit on the
  host with no record pointing at them, no UI path, and no sweeper. Why it matters: the
  first review and the security review both treated the probe as *the* aggravating factor,
  so closing it can read as closing the finding. Fix (pick one, it is a product decision):
  remove `ProjectHome` on **every** delete (the home holds no user work — R13 says
  durability is git, and the workspace is a separate directory that non-wipe deliberately
  keeps), or add the orphan sweeper, or record the deferral with this residual spelled out
  rather than attributing it to the probe.

- **[medium / confidence: high]** `internal/httpapi/server.go:622` (`cloneURLRe`) —
  **I disagree with deferring this one.** `cloneURLRe` admits `https://user:token@host/...`
  and `git clone` writes the remote verbatim into `<workspace>/.git/config`, on a host path
  that survives a non-wipe delete and that nothing on that path ever redacts — the same
  disposal problem as the finding above, compounded. The fix is three lines at the boundary
  (`net/url.Parse` → `u.User != nil` → 400) and it deletes an entire class, including the
  two residual redaction gaps in finding 7 below; the deferral costs more bookkeeping than
  the fix costs code. The one real objection is usability — a user cloning from a host the
  broker has no credential for reaches for a token URL — so if you want to keep accepting
  them, the cheap middle path is to strip userinfo from the persisted remote after a
  successful clone (`git remote set-url origin <bare>`), which keeps the flow and removes
  the artefact. What is not defensible is deferring it *silently*: it needs a decision.

- **[low / confidence: high]** `internal/httpapi/server.go:626-632` and `:552-558` — **two
  doc comments name symbols that do not exist**, one of them a direct leftover of the M4
  remediation. At `:626` the comment still opens `// redactURL strips any userinfo … (R10).`
  and is immediately followed by `// userinfoRe matches …`, both attached to
  `var userinfoRe` — `redactURL` was deleted, so godoc now documents `userinfoRe` under a
  dead identifier. This is the exact defect the first review's M4 asked to fix ("let the
  surviving comment sit on `userinfoRe`/`redactSecrets` where it belongs"); the function
  went, the comment stayed. Newly introduced by this diff, the same shape at `:552`: the
  `// guestErrMessage condenses …` block is attached to `const guestErrMaxBytes = 300`, so
  `func guestErrMessage` has no doc comment and the const is documented under the wrong
  name. Fix: delete the two orphan `redactURL` lines; move the `guestErrMessage` block below
  the const, onto the func.

- **[low / confidence: high]** `internal/httpapi/server.go:1156-1169` (`stripControls`) —
  the hand-rolled range list still misses characters the first review named. U+2028 LINE
  SEPARATOR and U+2029 PARAGRAPH SEPARATOR are not in any of the six ranges (they are `Zl`/
  `Zp`, not `Cf`), and neither is U+061C ARABIC LETTER MARK — all three survive into the
  sidebar `title=` tooltip, and U+2028 renders as a line break in several engines. Fix:
  replace the switch with `unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028
  || r == 0x2029`, which is shorter, covers the class rather than a snapshot of it, and
  cannot drift as Unicode adds format characters. (Confidence high on the gap; medium on the
  exact category membership of U+200B across Unicode versions — verify with a table test.)

- **[low / confidence: medium]** `internal/httpapi/provision.go:28-40` (`setProvisionError`)
  — the reported unbounded leak is closed, but a strictly narrower window remains, and there
  is still no GC. The interleaving: `projectExists` returns true → the *entire* delete runs
  to completion (record removed, and the deferred `delete(s.deleting, key)` at
  `server.go:828-832` fires) → this goroutine then acquires `deletingMu`, sees
  `deleting[key] == false`, and writes an entry nothing will ever clear. That requires a
  multi-second destroy to fit between two adjacent statements, so it is practically
  unreachable — I rate it low and would not block on it. Note that moving `projectExists`
  *inside* the critical section (the obvious fix) is safe today but introduces a new
  `deletingMu → spacesMu → store` lock ordering that nothing currently enforces; the cheaper
  and complete alternative is to prune in `listProjects`, which already enumerates every one
  of the user's projects and can drop `provisionErrs` keys with no matching record.

- **[low / confidence: high]** `internal/httpapi/provision.go:28` +
  `internal/httpapi/provision_test.go:289-304` — **the first review's "empty message branch
  is unreachable" finding was neither fixed nor deferred.** Nothing in production ever calls
  `setProvisionError(user, id, "")`; only the test does. Consequence, unchanged: a project
  that recorded `"clone skipped: the project VM did not start"` keeps the red "repo setup
  failed" badge until it is deleted or the server restarts — including after the user boots
  the VM and clones by hand. The in-memory map also means the badge silently disappears on
  restart while the empty repo it describes does not, so the signal is less durable than the
  condition. Fix: clear on the success paths in `provisionRepo` (and on a successful
  `startProject`), or delete the branch and its test case.

- **[low / confidence: medium]** `internal/httpapi/server.go:571` (`guestErrMessage`) —
  redact-then-strip is the wrong order. `redactSecrets` runs first, so a format character
  planted inside the scheme (`ht​tps://tok@host`) defeats the regex, and
  `stripControls` then removes it, emitting the token in the clear. Only a self-directed
  threat — the guest is the project owner and the token is their own — so severity is low,
  but the fix is free: strip first, then redact.

- **[low / confidence: high]** `internal/httpapi/server.go:552-576` + `:626-646` +
  `:1147-1180` — the extraction is half done. `guestErrMessage` has three callers, all in
  `provision.go`, and none in `server.go`; `redactSecrets`/`userinfoRe` have one production
  caller, in `provision.go`; `stripControls`/`clampBytes` are shared with `clampTitle`. The
  net effect is that the sanitizers stayed in the god-file the split was meant to relieve,
  scattered across three separate regions of it, 600 lines apart. Fix: move
  `guestErrMessage` into `provision.go` and the three shared sanitizers into a
  `internal/httpapi/sanitize.go` with their tests, so `server.go` keeps routing and CRUD.

- **[low / confidence: high]** `internal/httpapi/provision_test.go:244-255`
  (`TestProvisionRepoRecordsNothingOnSuccess`) — **the test cannot fail.** It provisions as
  `user: "u", project: "p"`, which `newProject` never registered, so `setProvisionError`'s
  own `projectExists` guard drops *every* write for that pair; the assertion
  `s.provisionError("u","p") == ""` would hold even if the clone had failed and recorded a
  message. Fix: use `newProject(t, s, …)` as `TestProvisionRepoRecordsCloneFailure` does, so
  the success assertion actually distinguishes success from failure.

- **[low / confidence: medium]** `internal/httpapi/provision.go:200` (`brokerProbeCmd`) —
  the probe proves less than the failure it guards. `/healthz` is the broker's one
  unauthenticated route (`credbroker/broker.go:46`) and answers `ok` to any caller,
  including one `ResolveProjectByIP` cannot map to a project; `/git` and `/env` return 403
  in that state. So the probe confirms route + listener, not "the broker can identify me",
  which is the condition the clone actually needs. In practice the observed outage was
  route-level, so this is adequate today — but if a resolution race ever appears, the probe
  will report ready. If you want the stronger signal without materializing anything,
  `GET /env` returns 200 only for a resolved caller (its body is discarded by the existing
  `>/dev/null`), at the cost of moving tokens over the bridge for a readiness check.

- **[low / confidence: high]** `internal/httpapi/provision.go:180-195` — the probe's
  correctness in production hinges on one untested assumption: that `docker exec`'s `Env`
  is *additive* to the container environment, so `${STYRMAN_CRED_BROKER}` (injected at
  create time, `orchestrator.go:766`) is still visible even though `ExecSpec.Env` is set to
  `HOME`/`USER` (`server.go:753-758`). It is — the daemon merges exec env over the
  container's — but every test injects `run`, so nothing anywhere exercises it. If it were
  wrong the failure is quiet, not loud: 10 fast curl failures, ~5 s added to every
  credentialed create, one warning line, then provisioning proceeds. Fix: assert it once in
  `.agents/docs/testing/styrman/integration.sh` (a real project + `docker exec … sh -c 'echo
  $STYRMAN_CRED_BROKER'`), which is cheap and pins the one link the unit tests cannot reach.

- **[low / confidence: high]** `TODO.md` (unmodified) — **none of the four deferrals is
  recorded there.** `git status --porcelain TODO.md` is empty and a grep for `clone`, `wipe`,
  `provision` and `.env` in it returns nothing; the only record of the deferrals is two
  *untracked* files under `.agents/docs/reviews/`. `TODO.md`'s own Inbox is the mechanism the
  project uses to pick deferred work back up ("Add new items below as they arise"), and
  nothing feeds `/preflight` from `reviews/`. Fix: add the four items to the Inbox with a
  one-line rationale each and a pointer to the review that found them, before the commit
  closes.

- **[low / confidence: high]** `deploy/vulncheck.sh:38` (deferral #1, out of this diff's
  scope) — I agree it does not belong in *this* change set, and I disagree with deferring it
  past the next release. `-v /dev/null:/src/deploy/.env:ro` cannot mount when `deploy/.env`
  is absent (a fresh clone, CI, any host but this one), so `docker run` fails before
  govulncheck starts and the script returns a docker exit code; a gate that fails *open* on
  a clean checkout is worse than no gate, and `/preflight` depends on it. One-line fix:
  only add the shadow mount when the file exists, and assert exit ∈ {0,3}.

- **[low / confidence: high]** carried from the first review, still open and **not** on the
  deferral list — each was judged low there and remains low, listed so nothing is lost:
  `provision_test.go:65,75-89` `recorder.reply`'s `n` parameter is computed on every call
  and read by nobody (dead parameter, dead work); `provision_test.go:390-402` still issues a
  real `DELETE`, whose goroutine reaches `orchestrator.Destroy` → `docker.ContainerRemove`
  with no `cfg.Image == ""` escape hatch, making a unit test depend on the host daemon;
  `NewProjectDialog.tsx:106-110` `nameEdited` still latches permanently on the first
  keystroke, so clearing the field strands the user with an empty name and a "Create"
  button that stays disabled; `api.ts:210-219` `projectNameFromRepoURL` still derives
  confident wrong names from half-typed URLs (`https://github.com` → `github-com`).

- **[low / confidence: high]** `internal/httpapi/server.go:744` — the GitHub-visibility
  predicate `req.GitHub == "private" || req.GitHub == "public"` is duplicated inline three
  statements above the `provisionSpec` literal, while `provisionSpec.wantsGitHub()`
  (`provision.go:75-77`) exists for exactly this. Two places to change if a third visibility
  is ever added. Fix: give the predicate a package-level helper
  (`wantsGitHubRepo(req.GitHub)`) and call it from both.

## Criteria

1. **Dead code — pass, with two comment-level exceptions.** No unreferenced function, type or
   file survives the restructure: `redactURL` and `TestRedactURL` are gone with no callers
   left behind (grep across `*.go`/`*.ts`/`*.tsx`/`*.mjs`), and every new symbol —
   `provisionSpec`, `wantsGitHub`, `provisionRepo`, `waitForBroker`, `stripControls`,
   `clampBytes`, `projectExists`, `guestHostname`, `projectNameFromRepoURL` — has at least
   one production caller. What remains is dead *documentation* (finding 3), one dead branch
   with only a test caller (finding 6) and one dead test parameter (last finding). `go vet`
   and `gofmt -l internal/` are clean; no commented-out blocks were introduced.
2. **Flow tracing — pass.** `createProject` → goroutine → `EnsureRunning` →
   `provisionRepo(attach, waitForBroker, run)` → `setProvisionError` → `provisionErrs` →
   `listProjects`/`getProject` → `projectView.ProvisionError` → `Project.provisionError` (TS)
   → Sidebar badge: every hop agrees on types, and the map key is `user+"/"+projectID` at all
   six sites. The M1 gap is closed on both boot-failure arms, and R13 still holds on each —
   the clone arm re-inits, the GitHub arm needs no re-init because `server.go:687-697`
   already init'd it before the 201. `ctx` reaches every wait (`provisionDelay` and
   `brokerProbeInterval` are both cancellable `select`s) and every `run` (`Exec` guarantees a
   30 s deadline of its own). Errors: no `err` is discarded; every failure path both logs and
   records. Lock discipline: `deletingMu` is never held across another lock, and
   `projectExists` deliberately runs outside it, so no deadlock is reachable — only the
   vanishingly narrow window in finding 5.
3. **Standards conformance — pass, minus godoc.** `gofmt -l` clean, `go vet ./internal/...`
   clean, `go test ./internal/httpapi ./internal/orchestrator` green, `tsc -b` clean, all
   re-run against this tree. Two violations of the `golang` skill's "a doc comment begins
   with the identifier it documents" rule remain (finding 3). Comments throughout explain
   *why* — the `provision.go` header and the `waitForBroker` rationale are unusually good
   examples of it. R10 holds: no secret reaches a log or a response unredacted on any path I
   traced. R13 holds on all four create paths.
4. **Directory layout — pass.** `provision.go` and `provision_test.go` are exactly where an
   unfamiliar engineer would look, beside each other, in `internal/`; no `pkg/`, no stray
   top-level TS, frontend entirely under `web/`. The one nit is the *incomplete* move
   (finding 8): the sanitizers the provisioning path owns stayed in `server.go`.
5. **Simplicity (DIY-except-security) — pass.** No new dependency; stdlib `regexp`,
   `strings`, `unicode/utf8`, `time` only. Nothing hand-rolled in the security exemption
   area: auth is unchanged, the credential path is the existing broker, host-key policy is
   explicit TOFU with a stated rationale, and both shell interpolations are `shellQuote`d
   *and* regex-gated upstream — `p.Name` is now quoted where it previously was not, which is
   a strict improvement. `provisionSpec`'s two injected funcs are the one abstraction added,
   and they buy something observable: `TestProvisionRepoAttachesBeforeAnyCredentialedCommand`
   pins the exact ordering bug this change exists to kill, with no Docker daemon.
6. **Decoupling — pass.** The `attach`/`run` seam is the right one: `provisionRepo` depends on
   two function values instead of on the orchestrator or Docker, so provisioning and
   orchestration are now independently editable, and the file split means a second engineer
   can work `server.go`'s routing while a first works the provisioning sequence. Deduction
   for finding 8 — `server.go` at 1181 lines is still the package's god-file, and the
   sanitizers left in it are precisely the code the two engineers would collide on.

## Not re-verified here

The verification log on disk
(`.agents/docs/testing/2026-09-02-verify-project-create-provisioning.md`, status FAIL on
coverage) predates this restructure: it describes a login-shell probe, `waitForBroker` with
"zero test coverage", and provisioning living in `server.go` — none of which is true of this
tree. The coverage gap it failed on is now closed by `provision_test.go`. A fresh `verifier`
run is owed against this diff before the commit closes, and no run of any kind has yet
exercised a real clone against a real microVM — which is the only thing that proves the
outage this change set exists to fix is actually gone.
