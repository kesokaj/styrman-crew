---
owner: reviewer
date: 2026-09-13
---
# Cold-reader check, round 3 — plan `2026-09-13-audit-fixes.md` (v1.30.0)   (2026-09-13)

Range reviewed: the plan file `.agents/docs/plans/2026-09-13-audit-fixes.md`
(working-tree revision, ` M` against HEAD `7066301`), both prior cold-reads
(`…-coldread-audit-fixes.md`, `…-round2.md`), and the code surfaces the plan
names. Nothing was built, run, tested or deployed; every claim is read from the
file:line cited, except `cookie samesite` which I read out of the pinned module
in this machine's module cache.

## Verdict: design needs revision / implementation not ready

The three re-checks the dispatch named come out **two clean, one not**: H1's
one-shot recursion is safe and terminates, M11's nil-bridge test compiles and
the first-statement check is reachable before any dereference — but M8's verb,
as specified, verifies tags **in whatever repository the caller's cwd sits in**
(`DEPLOY_DIR` only feeds `-c safe.directory`, which is a trust setting, not a
repo selector; the production `cd` is at `styrman-update:164`, after the verb
exits), and its `verify_tag "$2"; exit $?` call is the one context in the script
where `set -e` is live inside the function body, so the new GNUPGHOME cleanup is
skipped on exactly the refusal path it was added for. Two more decisions round 2
asked for are still unwritten: H1 drops `invalidateSelfAttach` along with the
reconnect it is replacing, and the marker rule "removed on the next success" is
order-dependent across two probes. Round 2's G5 (caddy `samesite`) I could close
with evidence rather than a question.

---

## 1. Gaps — what would change what an implementer writes

### G1 — H1: the new `recreateMissingNetwork` drops `invalidateSelfAttach`, and the recursion never re-does it  (medium / confidence: medium)
The plan's new body is enumerated as "recreate the network, then
`ContainerRemove` the stopped container (its start just failed) and return
true". Today's function ends with three steps: `createBridgeNetwork`,
`NetworkConnect` (`orchestrator.go:1480`), and `invalidateSelfAttach`
(`:1486`, added by the 2026-08-30 review with the comment "The recreated bridge
is a new network: drop any stale self-attach memo"). The plan replaces the
middle one and is silent on the third, and an implementer reading it as the
spec deletes the tail wholesale.

That matters on this exact path, not in theory: on the recursive call `create`
inspects the network, finds it present **with the wanted bridge name** (it was
just built by `createBridgeNetwork` under the same `lan`), so the
`invalidateSelfAttach` at `orchestrator.go:992` is not reached
(`createdNetwork` stays false) and the one at `:978` is a mode-change branch
that does not apply. Meanwhile `EnsureSelfAttached` short-circuits on a memo
younger than a minute (`:624`), and the broker address the freshly created
container receives (`:1017-1019` via `brokerIPOnBridge`, `:502-517`) is an
address only the server's **attach** binds on that bridge. Net effect: for up
to 60 s the new container's `STYRMAN_CRED_BROKER` points at an IP nobody is
listening on — the same symptom (no brokered credentials in the login shell)
that H1 exists to fix, now as a short window instead of a permanent one.
**Decide:** state that the function keeps `invalidateSelfAttach(netName)` and
drops only `NetworkConnect`.

Adjacent, mechanical but worth a word since the plan rewrites this function:
its doc comment (`:1468-1470`, "and reconnects the container to it … the caller
should retry the start") describes behaviour the change removes.

### G2 — M8: `DEPLOY_DIR="$4"` does not make git operate on the scratch repo  (high / confidence: high)
The verb is specified as `REQUIRE_SIGNED=1; ALLOWED_SIGNERS="$3";
DEPLOY_DIR="$4"` (so `git()`'s `safe.directory` names the scratch repo), then
`verify_tag "$2"`. `git()` is `command git -c safe.directory="${DEPLOY_DIR}"
"${GIT_CRED_ARGS[@]}" "$@"` (`styrman-update:229`): `safe.directory` disables
git's ownership guard for a path, it does not select a repository. Repository
selection in this script comes from `cd "${DEPLOY_DIR}"` at `:164` — which the
verb deliberately runs *before* (it is dispatched ahead of the arg loop `:60-66`
and the root check `:77`, both of which sit above `:164`).

So `verify_tag`'s `git cat-file -t "${tag}"` (`:297`), `git cat-file tag`
(`:313`) and `git … verify-tag` (`:334`) resolve against the **process cwd**. A
Go test that does not set `cmd.Dir` runs them inside styrman-crew itself, where
`v1.29.0` and friends genuinely exist and genuinely verify — a green run that
measured the wrong repository (BUG-040/BUG-034 family), and a red one for
unrelated reasons on a machine whose checkout lacks the tags.
**Decide:** the verb `cd "$4" || fail …` (or `git -C`), and say so; `cmd.Dir`
in the test is not a substitute, because the script must be correct for a
hand-run too.

### G3 — M8: `verify_tag "$2"; exit $?` is the one errexit-live caller, so the GNUPGHOME cleanup is skipped on refusal  (high / confidence: high)
The plan specifies `gh="$(mktemp -d)"; GNUPGHOME="$gh" git -c
gpg.ssh.allowedSignersFile=… verify-tag "$tag"; rc=$?; rm -rf "$gh"` and calls
it "cleanup on both outcomes". It is, for the two production callers —
`verify_tag "${LATEST}" || fail …` (`:344`) and `verify_tag "${ref}" || return
1` (`:563`) — because bash ignores `set -e` (`:27`) inside a function body
invoked in a `||` list, so a non-zero `git verify-tag` falls through to `rc=$?`.
The verb calls it **bare** (`verify_tag "$2"; exit $?`), which is precisely the
context where errexit is live: the failing `git verify-tag` exits the script
from inside the function, `rc=$?` and `rm -rf "$gh"` never run, and `exit $?` is
never reached. Round 2's Q8 asked for a removal point on the refusal path; the
answer works everywhere except in the new code path the same finding adds.
**Decide:** `if verify_tag "$2"; then exit 0; else exit 1; fi` (restores the
suppressed-errexit context and keeps the exit status), or wrap the temp dir in
its own trap — noting the script's existing note that its EXIT trap
(`:131-133`) must be chained, not replaced.

### G4 — H1: with two probes and one marker, "removed on the next success" loses the failure  (medium / confidence: high)
The plan defines the rule once: "did not answer" = curl exit non-zero, "for
`/env` and for `/files` alike", marker "removed on the next success". The
snippet probes `/env` first (`image/configs/styrman-credentials.sh:40`) and
`/files` second (`:54`). Read literally — write on failure, remove on success,
per probe — a shell where `/env` fails and `/files` succeeds ends with **no**
marker and therefore no MOTD line, while the env tier (`GH_TOKEN`, the tokens
BUG-054 is about) is exactly what did not arrive. Round 2 raised this as Q14;
the revision did not change the wording.
**Decide:** one flag, one write — any probe failing marks, and the marker is
removed only when every probe succeeded (`__sc_fail=1` … single
`mkdir -p`/write or `rm -f` after both tiers).

### G5 — M4: the persisted-overrides WARN fires on a valid file  (low / confidence: high)
`loadOverrides` is `if o.DockerDiskGB >= 5 && o.DockerDiskGB <= 500 { … }`
(`internal/config/config.go:297`). An overrides file carrying only
`idleTimeout` unmarshals `DockerDiskGB` as 0, so the plan's "it gains a WARN
line naming the file and the value" prints an out-of-range line on every boot
for a perfectly valid file — the operator-facing noise that trains people to
ignore WARNs. Round 2 asked this twice (G6/Q11) and the M4 text is unchanged.
**Decide:** the WARN skips the zero value, which is `SetOperator`'s own "keep"
sentinel (`:326`).

---

## Re-checks the dispatch named

**H1 — recursion under the lock, and the "no loop" argument: both hold.**
`ensureRunningLocked` (`orchestrator.go:264`) takes no lock itself; the only
lock acquisitions are in `EnsureRunning` (`:193-195`) and `WithProject`
(`:217-222`), and the recursive call is a direct call on the same goroutine, so
the non-reentrant `sync.Mutex` from `lockProject` (`:110`) is never re-acquired
— no deadlock. Termination holds on both branches, not just the one the plan
argues: if `ContainerRemove` succeeded the recursion's inspect returns NotFound
and takes the create arm, which never calls `recreateMissingNetwork`; if it
failed and the function still returned true, the recursion re-enters
`case err == nil:`, the start fails again, and `recreateMissingNetwork` now
finds the network it just created and returns false (`:1472-1474`) — one extra
level, then the error. The create arm is fully supplied: all four `RunOpts`
construction sites pass `Name`, `Dir`, `GuestUser` and `Lan`
(`internal/httpapi/server.go:276,909,1105`, `internal/httpapi/network.go:228`),
so the recursion cannot reach `create` with an empty `Dir`/`GuestUser`. One
detail left unstated (non-blocking, terminates either way): what the function
returns when `ContainerRemove` fails.
The ADR premise as now written is also sound: `bridgeModeMismatch`'s rebuild
path stops the container and `create` removes the network before rebuilding
(`:962-979`), so post-fix every path that changes a project's network also
recreates the container.

**M8 — the hoisted function block: no forward references.** `log`/`fail`
(`:68-69`) reference only `$*`; `trusted_dir` (`:73-75`) only `$1`; `git()`
(`:229`) reads `DEPLOY_DIR` (`:29`, above any hoist point) and `GIT_CRED_ARGS`,
which the plan hoists and which the token block only appends to (`:228`);
`verify_tag` (`:293`) reads `REQUIRE_SIGNED` and `ALLOWED_SIGNERS`, which the
verb assigns itself, plus `log` and `git`, both hoisted. Shadowing `git` from
the top of the file is harmless: the first actual invocation is `git fetch` at
`:237`, well below the current definition point. The placement answer is
correct — the verb must precede `:60-66` (`exit 2` on a non-flag) and `:77`
(root), and it also lands above the DOCKER_CONFIG trap (`:129-133`) and the
`flock` (`:162`), which is what makes an unprivileged `go test` possible. The
two defects are G2 and G3, not the hoist.

**M11 — `NewBridge(nil, …)` compiles and nothing is dereferenced first.**
`NewBridge` (`bridge.go:60-69`) takes `*orchestrator.Orchestrator`,
`*config.Config`, `*slog.Logger` and four func values — every parameter is
nilable, and the constructor only stores them. With the check as the **first**
statement, `ServeHTTP` (`:114`) returns 403 before `b.storeFor(user)` (`:119`,
its first dereference) and `ServeSetup` (`:192`) before `b.guestFor(user)`
(`:195`) and `b.orch.EnsureSetup` (`:196`) — so the cross-origin case cannot
panic on the nil bridge. The plan is right to test the absent/same-host cases
through the helper rather than the handlers: a passing request would fall into
`b.storeFor`/`b.guestFor` and panic. The move direction is sound —
`sameOriginOrAbsent` (`internal/httpapi/browser.go:327-334`) depends only on
`net/url`, `strings` and `*http.Request`, has no test of its own to relocate
(three references repo-wide: `:323` comment, `:327` def, `:345` call), and
`httpapi` already imports `terminal` (`server.go:244`). Route coverage is
complete: the three WS routes are `:389` (`s.bridge`), `:390` (`ServeSetup`)
and `:396` (`browserSocket`, already checked).

**Round 2's G5, closed with evidence (not a gap).** `cookie samesite lax` is
supported by the pinned caddy-security v1.1.64 — parsed at
`caddyfile_authn_cookie.go:92` and documented at `caddyfile_authn.go:55`
("cookie samesite <lax|strict|none>"). The plan's string-assertion acceptance
is therefore adequate; no `caddy validate` gate is needed for this directive.
(For the record, there is no automated `caddy validate` anywhere in `deploy/`
— only the one-off `.agents/docs/testing/2026-09-04-verify-session-lifetime.md`
— so a future new directive has no net under it.)

---

## 2. Readiness — questions an implementer must ask

**BLOCKING**
1. H1: does `recreateMissingNetwork` keep `invalidateSelfAttach(netName)`
   (`orchestrator.go:1486`) after dropping `NetworkConnect`? (G1)
2. M8: does the verb `cd "$4"` (or use `git -C`), given that `safe.directory`
   selects no repository and the production `cd` is at `:164`, below the verb?
   (G2)
3. M8: how is the GNUPGHOME temp dir removed when `verify-tag` fails, given the
   verb's bare `verify_tag "$2"` runs the body with `set -e` live? (G3)
4. H1: is the marker rule "any probe failed → marker set; removed only when all
   succeeded", or per-probe write/remove in probe order? (G4)

**NON-BLOCKING**
5. M4: does the persisted-overrides WARN skip `DockerDiskGB == 0`? (G5)
6. H1: what does `recreateMissingNetwork` return if the `ContainerRemove`
   fails — true (recurse, start fails again, clean error) or false (today's
   "start %s: %w")?
7. H1: `styrman-credentials.sh:35,38` return early when `curl` or `jq` is
   absent, before any probe — no marker written, no stale marker cleared.
   Accepted? (Round 2's Q15, still unanswered; both tools are in the image.)
8. M8: the plan does not state the consequence round 2 asked for — the verb
   sets `REQUIRE_SIGNED` itself, so `/etc/styrman/update.env`'s opt-out and its
   ownership/mode guards (`:261-273`) stay untested by this batch. Record it
   with M6's accepted-FAIL line?
9. M9: `ARG GO_VERSION=1.27.1` drops the `go` prefix today's
   `image/Dockerfile:88-90` carries (`${GO_VERSION}` is `go1.27.1`, interpolated
   straight into the tarball URL) — the URL becomes
   `go${GO_VERSION}.linux-amd64.tar.gz`. Confirm, so a 404 is not blamed on the
   sum.
10. M9: the Node change replaces `curl | bash` + `apt-get install nodejs` with a
    tarball; the plan does not say how it is unpacked (`tar -C /usr/local
    --strip-components=1`, PATH for `npm`). Mechanical, and `claude --version`
    at `image/Dockerfile:99` is the acceptance — confirm no later step depends
    on the apt package.
11. M3: `--ignore-path /dev/null` belongs in `formatInPlace`'s prettier arm
    (`internal/httpapi/format.go:47`), not in `formatScript` (`:119`); the Where
    column names only `:119`. Confirm the other formatter arms are untouched.
12. M3: the two contradicting comments both change — `format.go:84` already
    claims the temp file is in the workspace, `format.go:120` says it is
    outside. Covered by "+ comments"?

---

## Criteria (applied to the plan as the artifact)

1. **Dead code** — pass. `ContainedPath` + its test, `AuthCookieName`,
   `ExternalLinkIcon`, `BrowserPane.attached`, the orphan JSDoc: deletion, not
   comments. `recreateMissingNetwork`'s `NetworkConnect` is deleted rather than
   left dangling — but see G1 for the line beside it that should stay.
2. **Flow tracing** — fail, on two hops: M8's verb never reaches the repo it is
   given (G2) and its cleanup never runs on the path it was written for (G3).
   H1's trace is now correct end to end (lock, recursion, create-arm inputs)
   except the self-attach memo (G1); M11's trace is correct and complete.
3. **Standards conformance** — pass. The verb is gated (`STYRMAN_UPDATE_TEST=1`,
   else `fail`) as `styrman-ctl`'s `__` hooks are; ADR-052 scratch prefixes and
   the BUG-006 removal procedure are honoured; M6's untested branch is recorded
   as an explicit FAIL line rather than hidden.
4. **Directory layout** — pass. New tests sit beside their subjects or follow
   the established script-test convention (`internal/config/styrman_ctl_test.go`,
   `internal/httpapi/browser_launcher_test.go`).
5. **Simplicity (DIY-except-security)** — pass. No new dependency; M8 hardens an
   existing signature gate, M9 pins rather than vendors, M10 tightens the vetted
   plugin's own allowlist.
6. **Decoupling** — pass. The Origin helper moves with the `httpapi → terminal`
   dependency edge, and the four phases touch disjoint trees.

## Not examined

- Nothing was built, run, tested, linted or deployed: no `go test`, no vitest,
  no `docker build`, no shellcheck, no integration/e2e.
- M9's published sums (Go 1.27.1, Node 22.23.2, the three base-image digests)
  were not verified against the publishers — no network was used.
- M1/M2 (`SidePanelGit.tsx:429,528`), M7, M12, M13's handler tests, and every
  item in the Low — *do* list except uuid, `ContainedPath`, the config bound and
  the format script.
- `deploy/host-setup.sh:372`, `deploy/styrman-ctl:646`, the ingress lows (Cookie
  strip, `path.Clean`, `sanitizeHostname`) and the web lows.
- H2 was re-audited in round 2 and found correct; I did not re-open it.
