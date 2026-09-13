---
owner: reviewer
date: 2026-09-13
---
# Cold-reader check — plan `2026-09-13-audit-fixes.md` (v1.30.0)   (2026-09-13)

Range reviewed: the plan file `.agents/docs/plans/2026-09-13-audit-fixes.md`
and the code surfaces it names, at `e56e775` (working tree clean). Source of
the findings: `.agents/docs/reviews/2026-09-13-audit-project.md`. Nothing was
built, run or tested; every claim below is read from the code cited.

## Verdict: design needs revision / implementation not ready

Two of the plan's fixes are wrong as written, not merely underspecified.
**H2** replaces `AbortSignal.any([caller, timeout(20000)])` with
`init?.signal ?? AbortSignal.timeout(20000)` on the stated ground that
"`checkFile` combines its own 20 s (kept)" — it does not: `FileEditor.tsx:189`
hands `checkFile` a bare `new AbortController().signal`, and its own comment at
`FileEditor.tsx:183` says it *relies* on request()'s 20 s bound. Shipping the
plan as written removes every timeout from the check endpoint, which is the
exact regression `api.ts:366-368` records a previous review as having fixed.
**M3** moves the format temp file into the workspace but only rewrites the
`mktemp` call; the same one-line script also carries
`find "$D" -type f -mmin +60 -delete` (`format.go:119`), which with `$D` = the
workspace would delete every file in the user's project older than an hour on
the next Format click. Below those: H1's central premise about the orchestrator
is false (`recreateMissingNetwork` is a real counter-example), M8's test cannot
be written the way the plan describes (the updater is not sourceable), M11
names a check that does not exist in the package it says to move it from, and
the uuid change has a second production caller the plan does not mention.

---

## 1. Gaps — what must be decided before implementation starts

### G1 — H1: "every orchestrator recreate path recreates the container with a fresh env" is false  (high / confidence: high)
`internal/orchestrator/orchestrator.go:1471-1489` `recreateMissingNetwork`
creates a NEW network and `NetworkConnect`s the **existing** container to it —
it does not remove or recreate the container. `createBridgeNetwork`
(`orchestrator.go:891-897`) passes no IPAM config, so Docker allocates the
subnet from its pool; the new subnet can differ from the old one. The
container's `STYRMAN_CRED_BROKER`, injected once at
`orchestrator.go:1017-1019`, is therefore stale on exactly that path — which is
the scenario the "/16 recomputation (M17)" was added for
(`styrman-credentials.sh:19-24`). It is reached from
`ensureRunningLocked` (`orchestrator.go:355-362`) when a start fails because
the network vanished.

The fix (delete the recomputation) is still right — the recomputation is
wrong for `/24` and is a live production defect — but the plan's *rationale*,
which it wants written into an ADR so "nobody re-adds it", is not true as
stated, and the ADR would enshrine a false invariant.

**Decide:** (a) make `recreateMissingNetwork` remove the container and fall
through to the create path (it is already a stopped container — the start just
failed — so no live terminal is killed), or (b) keep the reconnect and state
the residual honestly in the ADR: "one path can leave a stale broker URL; the
MOTD marker is how it surfaces". The plan's Risks section must match whichever
is chosen.

### G2 — H1: the broker-unreachable marker is underspecified  (medium / confidence: high)
Ordering is fine — `entrypoint.sh:98,102` install the MOTD as
`zzz-styrman-motd.sh` and the credentials snippet as
`zz-styrman-credentials.sh`, so the MOTD runs after the marker is written in
the same shell. What is not decided:
- **What counts as "does not answer".** `styrman-credentials.sh:40` is
  `__sc_env="$(curl … || true)"` and the only test is `[ -n "${__sc_env}" ]`.
  A curl failure and a broker that legitimately answers with an empty body are
  indistinguishable there. Decide whether to capture curl's exit status
  separately (and whether `/files` failing independently also sets the marker —
  the plan only mentions `/env`).
- **Where the MOTD line goes.** `profile-motd.sh:4` wraps the single existing
  message in one `if [ ! -f "$HOME/.claude/.credentials.json" ] || ! git config
  …`. The broker line must be its own condition, and the whole block is already
  gated on `[ -t 1 ]` (`profile-motd.sh:2`), so the marker is never *shown* for
  the non-interactive `bash -lc` login shells the provisioner uses
  (`internal/httpapi/provision.go`). Decide whether that is acceptable.
- **`$HOME/.styrman` may not exist** on a fresh project home → the write needs
  `mkdir -p` and must stay non-fatal (the snippet must never break a login).
- **Host-mode dev**: `STYRMAN_CRED_BROKER` is empty (`brokerIPOnBridge` returns
  "" when `SelfContainer`/`BrokerAddr` are unset, `orchestrator.go:507-511`),
  so the snippet returns at line 34 and the marker is neither written nor
  cleared. Say so, or the local dev MOTD will be stale forever.

### G3 — M3: the `find … -delete` sweep is a project-wipe if `$D` becomes the workspace  (high / confidence: high)
`format.go:119` is one `fmt.Sprintf` containing
`D="$HOME/.styrman/tmp"; mkdir -p "$D" && find "$D" -type f -mmin +60 -delete`.
The plan rewrites only the `mktemp`. If an implementer does the obvious thing
and repoints `$D`, the first Format click deletes every regular file in the
project's working tree older than 60 minutes, recursively. **Decide and write
it into the plan:** the sweep is deleted outright, or rescoped to
`-maxdepth 1 -name '.styrman-fmt-*'`. This is the single most dangerous
unstated edge in the document.

### G4 — M3: the mktemp form is valid GNU, but the plan specifies two different implementations  (medium / confidence: high)
`mktemp -p "$WS" .styrman-fmt-XXXXXX --suffix=.ext` **is** valid GNU coreutils
syntax: the template ends in `X`s (required whenever `--suffix` is given
explicitly), the suffix is appended *after* the random run
(`.styrman-fmt-a1B2c3.go`), and GNU getopt permutes the option after the
operand. The guest is Debian, so coreutils mktemp is what runs. Two caveats
the plan must settle:
- `-p "$WS"` requires `formatScript` to gain a workspace parameter — its
  signature today is `formatScript(ext string, n int, cmd string)`
  (`format.go:119`) and `format.go:85` calls it with three arguments. But the
  exec already sets `WorkDir: ws` (`format.go:92`), and `ws` is the *guest*
  path built at `git.go:68`. The plan's fix text says `-p "$WS"`; the plan's
  test text says the script "targets `$PWD`-relative workspace". Those are
  different implementations. Pick one — the `$PWD` form needs no new parameter,
  no interpolation of a server-built path into a `bash -c` string, and no
  option-after-operand dependence at all.

### G5 — M3: the fix only restores config discovery for the workspace ROOT  (medium / confidence: high)
Formatters resolve config from the *file's own* directory upward. A monorepo
file at `web/src/x.ts` with `web/.prettierrc` still gets the repo-root config
when the temp file sits at the workspace root. Decide whether the temp file
belongs in `dirname(body.Path)` (the actual fix for the stated problem) or at
the root (a partial fix) — and if the root, say in the plan that per-directory
config is still not found, because `api.md`'s corrected text will otherwise
claim more than the code does.
Second, undecided failure mode: `prettier --write` on a path matched by
`.prettierignore`/`.gitignore`-style ignore rules (a leading-dot temp name is a
common ignore pattern) exits 0 having formatted nothing, so the handler's
`STYRMAN_OK` branch (`format.go:104`) returns the user's content **unchanged as
a success**. Decide whether to neutralise ignore files for the temp path.

### G6 — M3: yes, `.styrman-fmt-*` is visible, and the plan does not decide what to do about it  (medium / confidence: high)
This is not speculative — `check.go:427`'s own comment records it: "a
workspace-root temp showed up in git status and the file explorer on every
debounce tick (carried-over review low, 2026-09-01)". The git endpoints run
`status --porcelain=v2 --branch --untracked-files=all`
(`internal/httpapi/git.go:211` and `:985`), which lists dotfiles. The plan's
argument is that format's ~100 ms lifetime makes it harmless. Two things it
does not decide: (1) a user pressing "Stage all"/commit inside that window
stages a temp file; (2) the `trap … EXIT` (`format.go:120`) covers the script
exiting, not the VM being stopped or the exec being killed mid-format, so a
crash leaves the file behind indefinitely. Decide: accept and record, or write
the pattern into `.git/info/exclude` on first use. Note this fix deliberately
reverses a previous review's finding; under AGENTS.md's "second round on the
same mechanism" rule that reversal deserves an explicit sentence in the plan.

### G7 — M8: `styrman-update` cannot be sourced, so the described test cannot be written  (high / confidence: high)
`deploy/update/styrman-update` is a linear top-level script, not a library.
Sourcing it executes, before `verify_tag` is even defined at line 293:
`set -euo pipefail` (:27), the argument loop (:59-66, which `exit 2`s on an
unknown flag — Go's test binary flags would hit this), the GH-token block
(:207-228), the `git()` wrapper override (:229), `git fetch --tags --force
--prune --prune-tags origin || fail` (:238) — a network call that exits 1 —
and then `verify_tag "${LATEST}"` itself at :344, plus the whole deploy path.
There is no `BASH_SOURCE`/`return`-if-sourced guard and no `__`-prefixed test
hook (which is what `deploy/styrman-ctl` has, and what
`internal/config/styrman_ctl_test.go:11-17` documents as the seam).

**Decide:** add a source guard or a `STYRMAN_UPDATE_LIB=1` early return to the
script; or extract `verify_tag` into a sourceable file both the script and the
test read; or drive the whole script with fakes on `PATH` the way `ctlSetup`
(`styrman_ctl_test.go:19-60`) drives styrman-ctl. Whichever is chosen, the test
must also initialise `REQUIRE_SIGNED`, `ALLOWED_SIGNERS` and the `git()`
wrapper the sourced function depends on.

*Harness answer for the dispatch's question:* yes, there is a harness worth
copying — `internal/config/styrman_ctl_test.go` (scratch prefix per ADR-052,
fake binaries on `PATH` recording argv, `--dry-run`/`__` hooks as seams), and
`internal/config/firewall_script_test.go` is a second instance. Neither sources
a function out of a script; both run the script.

### G8 — M8: the fixture test's environment prerequisites are undecided  (low / confidence: medium)
"an honestly SSH-signed tag with a scratch allowed_signers" needs `ssh-keygen`,
a git new enough for `gpg.format=ssh`, and a writable `HOME`. Decide whether
the test `t.Skip`s when those are absent or is allowed to fail — `go test
./...` must stay green on a machine without ssh tooling.

### G9 — M11: the check the plan says to "move" does not exist in `internal/terminal`  (high / confidence: high)
`sameOriginOrAbsent` is defined at `internal/httpapi/browser.go:327` and used
at `browser.go:345`. `internal/terminal/bridge.go` has **no** pre-upgrade
Origin check at all — its only Origin verification is the default inside
`websocket.Accept`, as `bridge.go`'s comment at the top of `serve` says. And
`internal/terminal` cannot import `internal/httpapi`: the dependency runs the
other way (`internal/httpapi/browser.go:20` and `server.go:38` import
`internal/terminal`). **Decide:** duplicate the helper in `internal/terminal`,
or lift it into a shared package and update `browser.go`'s caller.

**Scope edge the plan inherits from the audit and should close:**
`ServeTerminal` has the same defect. `bridge.go:165` calls `b.ensure(...)`,
which boots/starts the **project** microVM, before any upgrade or Origin check;
the `boot=1` gate at `bridge.go:157` only covers projects that are
*deliberately stopped*. A forged cross-origin GET boots a project VM exactly as
it boots the setup VM. Decide whether M11 covers both entry points — fixing
only `ServeSetup` leaves the larger of the two open.

### G10 — uuid: a second production caller the plan does not name  (medium / confidence: high)
The plan says "callers: createProject → 500". `NewUUID` (`internal/project/uuid.go:10`)
has two non-test callers: `Store.Add` (`store.go:155`, the createProject path
the plan names) **and** the legacy-record migration inside the store loader at
`store.go:100`, which runs once per user at load and returns `(s, nil)` on
every other recoverable problem (see the quarantine branches at `store.go:95`
and `store.go:109`). Decide what a `crypto/rand` failure does there: quarantine
the record into `.rejected` like its siblings, or fail the whole `Load` (which
500s every request for that user). The plan's failure-modes section covers only
the create path.

### G11 — M4: the bound is added in one of the two places that set the value  (medium / confidence: high)
`Load` reads `dockerDisk: envIntOr("STYRMAN_DOCKER_DISK_GB", 20)` at
`internal/config/config.go:170`. But `applyOperator` (`config.go:~295-299`)
re-applies `DockerDiskGB` from the persisted operator-overrides JSON with no
range check — the 5-500 check exists only in `SetOperator` (`config.go:326`)
and the PATCH handler (`internal/httpapi/explorer.go:475`). A hand-edited
overrides file still yields an unbounded value. **Decide:** whether the bound
also goes in `applyOperator`, and whether an out-of-range *persisted* value is
fatal at boot (fail-closed, but bricks the server until the operator hand-edits
a file) or clamped/ignored with a WARN. The plan says only "fail closed like
the sibling checks", which does not answer it.

### G12 — M10: a fix the plan marks "pending the refuter"  (medium / confidence: high)
Phase 2 cannot start on a fix that may be withdrawn. For what it is worth I
confirmed the mechanism from the vendored dependency:
`go-authcrunch@v1.1.41/pkg/redirects/match.go` `Match()` matches `u.Host` with
the configured strategy, and `generator.go:264` emits `domain suffix
<BASE_DOMAIN>` — so `BASE_DOMAIN=styrman.xyz` trusts `evilstyrman.xyz`. The
leading-dot fix does not break the app host, which is
`${STYRMAN_HOSTNAME}.${BASE_DOMAIN}` (`deploy/docker-compose.yml:123`). Two
things still to decide: the **apex** (`styrman.xyz` itself) stops being a
trusted redirect target — confirm nothing redirects there; and `u.Host`
carries the port, so a redirect_url with an explicit `:port` no longer matches
(irrelevant behind 443, but the generator test should pin the intent).

### G13 — M6 ships an error branch with no test  (low / confidence: high)
The plan itself says "a reader that fails mid-read is hard to plant through
os.Root; … add a comment". That is changed behaviour with no covering test,
which AGENTS.md's Definition of Done step 1 calls a FAIL. Decide: introduce a
reader seam so the branch is testable, or record the exception explicitly so
`verifier` does not have to discover it.

### G14 — M9: the Antigravity decision is not a decision  (medium / confidence: high)
"download `install.sh` to a file, run it, and RECORD the accepted floating
version in the comment (no upstream pin)" leaves the audit's actual finding —
an unverified root `curl | bash` in the image build — exactly where it was; the
only change is that the pipe becomes two commands. An implementer cannot tell
whether Phase 3 is supposed to change Antigravity's install at all, what text
goes in the comment, or what the ADR is supposed to say. Decide: no change +
one ADR paragraph of accepted risk, or a real mitigation (vendored installer,
version-pinned artifact). Related: the plan lists "ADR for the image pins"
under Surfaces but assigns the writing to `changelog` in Phase 4 — name the
ADR number and its author.

### G15 — M9: `--ignore-scripts` on `@anthropic-ai/claude-code` is unverified  (medium / confidence: medium)
If that package relies on a postinstall step (native helper download,
bin shim), `--ignore-scripts` produces an image where `claude` does not run.
The plan's verification for Phase 3 is "docker build … styrman-runtime:test
boots in integration", which proves the build and the boot, not the CLI. Decide
the acceptance check — the honest one is `claude --version` (and `agy`) inside
the built image.

### G16 — release gate vs. the one finding that is live  (low / confidence: high)
The plan's own Verification section says H1 can only be confirmed in production
after the updater deploys ("the host-run server has no broker, so this is
production-only"). Phase 4 nonetheless ends at `/preflight v1.30.0`. Decide the
ordering: the release ships with the headline fix unverified on the only path
where it matters, and the handoff is the record. That is defensible, but it
should be a stated decision rather than a gap between two sections.

---

## 2. Readiness — questions a first-pass implementer must ask

**BLOCKING**
1. H2: `checkFile` does **not** carry its own bound — `FileEditor.tsx:189`
   passes a bare `AbortController` signal and `FileEditor.tsx:183` says it
   relies on request()'s 20 s. Does H2 (a) add an explicit bound at the
   `checkFile` call site before switching `request()` to `??`, or (b) keep the
   combination and give only the long-budget callers an opt-out? As written the
   plan removes the check endpoint's only timeout.
2. M3: is `find "$D" -type f -mmin +60 -delete` deleted or rescoped when the
   temp file moves into the workspace?
3. M3: `-p "$WS"` (new `formatScript` parameter) or `$PWD` (rely on the exec's
   existing `WorkDir`)? The fix text and the test text disagree.
4. M8: how is `verify_tag` reached from a Go test, given the script is not
   sourceable — source guard, extraction, or full-script-with-fakes?
5. M11: duplicate `sameOriginOrAbsent` in `internal/terminal`, or lift it to a
   shared package? (`internal/terminal` cannot import `internal/httpapi`.)
6. M11: is `ServeTerminal` in scope, or only `ServeSetup`?
7. uuid: what does the store-load migration path (`store.go:100`) do on a
   `crypto/rand` failure?
8. H1: does `recreateMissingNetwork` change (recreate the container), or is the
   stale-broker residual accepted and recorded in the ADR?
9. M10: is the fix committed, or still conditional on a refuter that has not
   landed? Phase 2 cannot start without an answer.

**NON-BLOCKING**
10. H1: what exactly distinguishes "broker did not answer" from "broker
    returned nothing", and does a `/files` failure set the marker too?
11. H1: does the MOTD line live outside the existing onboarding `if`, and is
    the `[ -t 1 ]` gate (no marker shown in `bash -lc`) acceptable?
12. H1: `internal/credbroker/snippet_test.go` is a reasonable home, but the
    in-repo precedent for reading an image script from a Go test is
    `internal/httpapi/browser_launcher_test.go:23`. Confirm the package choice
    and whether `bash -n` may assume `bash` on `PATH`.
13. M3: temp file at the workspace root or beside the edited file? And do
    ignore files need neutralising so `prettier --write` cannot no-op into a
    false success?
14. M3/M6: is `.styrman-fmt-*` added to `.git/info/exclude`, or accepted as
    visible for the duration?
15. M4: does the bound also apply in `applyOperator`, and is an out-of-range
    persisted value fatal or a WARN?
16. M6: is the untested error branch accepted, or is a reader seam introduced?
17. M9: what is the Antigravity comment/ADR text, and which ADR number?
18. M9: what proves `claude`/`agy` still run after `--ignore-scripts`?
19. M8: does the signing fixture test skip when `ssh-keygen` is unavailable?
20. Phase 4 ships `/preflight` before H1 can be verified in production —
    confirm that ordering is intended.

---

## Criteria (applied to the plan as the artifact under review)

1. **Dead code** — pass. M5 (`ContainedPath` + its test), the ingress
   `AuthCookieName`, `ExternalLinkIcon`, `BrowserPane.attached` and the orphan
   JSDoc are all scheduled for deletion, not for comments. Good.
2. **Flow tracing** — fail. Four traces are wrong or incomplete: H1's
   orchestrator premise (G1), H2's caller audit (blocking Q1), M11's package
   boundary and its second entry point (G9), and uuid's second caller (G10).
3. **Standards conformance** — mixed. The plan honours the project's rules it
   cites (BUG-006 for the `agent-memory` removal, ADR-052 scratch prefixes in
   the new script tests). M6 knowingly ships changed behaviour without a test,
   against Definition of Done step 1 (G13).
4. **Directory layout** — pass with one question. New tests land beside their
   subjects; `internal/credbroker/snippet_test.go` and
   `internal/config/styrman_update_test.go` both test files that live under
   `image/` and `deploy/`, which is the existing convention
   (`browser_launcher_test.go`, `styrman_ctl_test.go`, `firewall_script_test.go`).
5. **Simplicity (DIY-except-security)** — pass. No new dependency, no
   hand-rolled security primitive; M8 hardens an existing signature gate rather
   than replacing it, M9 pins rather than vendors.
6. **Decoupling** — one real seam problem: M11 needs an Origin helper on the
   correct side of the `httpapi → terminal` dependency edge (G9). Otherwise the
   phases are independently implementable; Phase 1 and Phase 3 touch disjoint
   trees.

## Not examined

- Nothing was built, run, tested, linted or deployed. No `go test`, no vitest,
  no image build, no shellcheck, no integration or e2e run.
- M1/M2 (`SidePanelGit.tsx:429,528`) — read only via the audit's description;
  I did not trace `treeChanged()`/`run()` or the 409 path myself.
- M12 (`api.md` ADR-067 rewrite) and the Phase 4 documentation/changelog work.
- M13's handler-test shape (`projectenv.go`, `stats.go`, the `guestexec_test.go`
  seam) — not read.
- The ingress lows (Cookie strip on the public block, `path.Clean` before the
  prefix exemptions, `sanitizeHostname`), `deploy/host-setup.sh:372`,
  `deploy/styrman-ctl:646`, and every web low in the *do* list.
- `image/Dockerfile` was not read line by line; M9's assessment is based on the
  audit's description plus the plan's text, except where noted.
- The two "refuter pending" items (M10, M11) — I reasoned M10 from the vendored
  `go-authcrunch` source, which is not the same as the refuter's verdict.
