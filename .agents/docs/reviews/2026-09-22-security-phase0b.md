---
owner: security
date: 2026-09-22
---
# Security review — phase0b   (2026-09-22)

Range reviewed: working tree — changed files: internal/files/files.go, internal/files/files_test.go, internal/files/rmcheck_test.go, internal/files/trust_test.go, internal/httpapi/explorer.go; untracked, not ignored: internal/files/listbounds_test.go, internal/httpapi/managedtarget_test.go

## Status: COMPLETE

## Verdict: FINDINGS

The managed-subtree guard's new symlink check looks at the FIRST path
component only, and the bypass it was written to close works one level
deeper: `mkdir -p work/sub; ln -s ../../.dockerd work/sub/link` then acting
on `work/sub/link/<uuid>.img` resolves into the managed subtree while
`linkedFirstComponent(root, "work")` sees a real directory and answers "not
managed". Probed directly against `os.Root` (Go 1.27.1):
`rt.Remove("a/b/disk.img")` with `a/b -> ../.dockerd` returned `nil` and the
target was gone; `rt.Rename("a", "top2/moved")` with `top2 -> .dockerd`
landed inside the managed dir. A red test for exactly this appeared in the
tree while this review was running (`managedtarget_test.go:101`,
`TestManagedHomeTargetRefusesASymlinkNestedDeeper`), and the implementation
is unchanged: `go test ./internal/httpapi/ -run TestManagedHomeTarget
-count=1` exits 1. The tenancy boundary itself (os.Root, R3) is NOT broken —
absolute and escaping symlinks are still refused — so the blast radius is
intra-tenant integrity of platform-managed state plus a confused-deputy
write by the server process, not cross-tenant reach.

## Findings

- **[medium / confidence: high]** `internal/httpapi/explorer.go:117-151` —
  `managedHomeTarget` tests only the first component for being a symlink, so
  a link at any deeper component still reaches a managed subtree
  (`.dockerd`, `.ssh`, `.styrman`, `projects`). Measured, not inferred: a
  scratch probe on Go 1.27.1 showed `os.Root.Remove("a/b/disk.img")` with
  `a/b -> ../.dockerd` succeeding (`err=<nil>`, file gone) and
  `os.Root.Rename("a","top2/moved")` with `top2 -> .dockerd` landing in the
  managed dir; an absolute in-root link was refused (`path escapes from
  parent`), so the escape is strictly *within* the home. The guard fronts
  DELETE, PUT (`O_TRUNC`), mkdir, upload and rename, so the concrete harm is
  the one the code's own comments name: truncating or unlinking the
  loop-mounted docker disk of a possibly-running microVM, or the
  set-up-once-ever `.ssh` identity (R3). Two caveats sized honestly: the
  project home is bind-mounted whole into the guest
  (`internal/orchestrator/orchestrator.go:1069`,
  `spec.homePath + ":/home/" + spec.guestUser`) and `.dockerd/<uuid>.img`
  lives inside it (`image/entrypoint.sh:311`), so a process in the guest
  can usually reach those files directly — the marginal capability is (a)
  making the *authenticated browser session* perform it on a path the guest
  chose, and (b) having the SERVER's uid write a target the guest's uid
  cannot (the `.img` and dockerd state are created in-guest as root;
  confidence medium on that sub-claim, it depends on the terminal user's
  uid and the home's ownership). Remediation: walk every component of `rel`
  except the last with `rt.Lstat` through the confined root and return
  "managed" on the first `ModeSymlink` hit — same fail-closed error
  handling as today (`!errors.Is(err, fs.ErrNotExist)`). That preserves the
  deliberate carve-out (acting on a link ITSELF stays allowed, at any
  depth, which `TestFileDeleteSymlinkEscape` and the new nested test both
  require) and costs one lstat per component. Verified safe by probe: the
  carve-out does not reopen the hole, because `os.Root.RemoveAll("top")` on
  a link removed only the link and left `.dockerd/disk.img` intact.

- **[low / confidence: high]** `internal/httpapi/explorer.go:70` and
  `web/src/lib/api.ts:589` / `web/src/components/FileExplorer.tsx:165` —
  the new `Truncated` flag reaches the HTTP response and stops there: the
  client types the body as `request<{ entries: FileEntry[] }>` and the
  explorer reads only `r.entries`, so a cut listing renders exactly like a
  complete one. The cap is therefore a concealment primitive for anything
  running in the guest: create 5000+ siblings and a file in that directory
  vanishes from the pane with no indicator. It is worse than a stable
  prefix, because truncation happens on raw `ReadDir` order and the sort
  runs AFTER the cut (`internal/files/files.go:205-268`), so the human sees
  an alphabetically plausible listing whose missing entries are arbitrary
  and may differ between polls. Server-side the flag answers the question;
  the UI discards the answer. Remediation: add `truncated: boolean` to the
  `listFiles` response type and render a banner ("showing the first 5000
  entries") — no server change needed. Threat model that makes this worth
  fixing on this platform: not a tenant hiding a file from themselves, but
  agent-run or prompt-injected code hiding an artifact from the human who
  audits the tree through the explorer.

- **[low / confidence: high]** `internal/httpapi/explorer.go:132-151` —
  `linkedFirstComponent` lstats and returns; the destructive operation then
  resolves the path again, so a guest can flip a component between a real
  directory and a symlink in the window. Consequence is bounded and worth
  stating plainly: both sides run as the same principal inside the same
  os.Root-confined project home, so the race buys exactly what finding 1
  buys directly and nothing more — no cross-tenant reach, no escape from
  the home. Recorded because the comment claims a boundary ("refused rather
  than resolved") that check-then-act cannot deliver; the per-component
  walk in finding 1's remediation narrows the window but does not remove
  it. A race-free guard would have to perform the operation itself through
  per-component `O_NOFOLLOW` opens. Recommended remediation is the cheap
  one: fix the wording so the guard is documented as protecting
  platform-managed state from the UI, with os.Root remaining the security
  boundary.

- **[low / confidence: high]** `go.mod` — pre-existing, not introduced by
  this diff, on the record because the checklist ran: `govulncheck ./...`
  (rc=3) reports the code as affected by GO-2026-4887 (Moby AuthZ plugin
  bypass on oversized request bodies) and GO-2026-4883 (off-by-one in
  plugin privilege validation), both in
  `github.com/docker/docker@v28.5.2+incompatible`, both `Fixed in: N/A`.
  The traces are client-side `init`/API calls from `internal/orchestrator`,
  and the vulnerable behaviour is the daemon's, so the practical exposure
  here is low; no upgrade exists today. Remediation: none available — track
  the advisories and bump when a fixed release lands. No dependency is
  added or changed by this diff.

- **[info / confidence: high]** `internal/files/files.go:471-481` —
  `ReadRawNoFollow`'s parent open gaining `syscall.O_NONBLOCK` does NOT
  widen what the function will open, which was the question asked. On a
  FIFO parent the open now returns immediately instead of blocking forever,
  and the following `unix.Openat(d, base, …)` fails `ENOTDIR`; on a device
  node it skips the driver's open-time wait; on a directory the flag is
  inert. The final component keeps `O_NOFOLLOW|O_NONBLOCK|O_CLOEXEC` and
  the `IsRegular()` check, and the sole caller
  (`internal/httpapi/setup.go:247`, `.ssh/id_ed25519.pub`) is unchanged.
  Net effect is availability only — the wedged-goroutine DoS the comment
  names — and it makes this open consistent with every other open in the
  package. One weakness in the new test: it asserts only `err != nil`
  within 5s, so it would also pass on an unrelated failure; asserting
  `ENOTDIR` would pin the mechanism.

## Checked

- **Secrets** — none in the diff or the two new test files; no new logging
  of file contents or credentials. The existing `logger.Warn` calls in
  delete/rename log the scope-relative path and error only, unchanged.
- **Injection** — no SQL, shell, or template surface. The parsing boundary
  that did change is the path grammar (`scopeRel` → `path.Clean` →
  `strings.Cut`); analysed as findings 1 and 3. Listing input is a single
  `?path=` resolved through `os.Root`.
- **Authz/authn** — `scopeFor` untouched: the home route still resolves to
  `ProjectHome(user, project)` after `who()` and `projects.Get()`, so R3
  per-project identity and R12's isolation are unaffected by this diff.
  `managedHomeTarget` gained a `root` argument at all five call sites
  (delete, rename ×2, mkdir, write, upload) — no call site was missed
  (grep-verified). `files.List`'s signature change has exactly one non-test
  caller (`explorer.go:63`).
- **Crypto** — nothing cryptographic and nothing hand-rolled in the diff.
- **Dependencies** — no module added or changed; the new test uses
  `golang.org/x/sys/unix`, already a direct dependency of `files.go`.
  `govulncheck ./...` run bare: rc=3, two affecting vulnerabilities, both
  pre-existing in `github.com/docker/docker` (finding 4). `npm audit` not
  run: this diff changes no JavaScript and no `web/package.json`.
  Root-module build/vet/test run bare: `go build ./...` rc=0,
  `go vet ./internal/files/ ./internal/httpapi/` rc=0,
  `go test ./internal/files/ -count=1` rc=0,
  `go test ./internal/httpapi/ -run TestManagedHomeTarget -count=1` rc=1
  (`FAIL: TestManagedHomeTargetRefusesASymlinkNestedDeeper`).
- **GCP surface** — not touched by this diff (no IAM, service account,
  Cloud Run flag, or Secret Manager change).

## Note on a moving tree

`internal/httpapi/managedtarget_test.go` changed on disk during this review
— a concurrent code review added
`TestManagedHomeTargetRefusesASymlinkNestedDeeper`, which asserts finding 1.
`internal/httpapi/explorer.go` did NOT change, so the finding stands and the
suite is red. Everything above describes the tree as of the final read.

## Not examined

- `go test ./...` over the whole root module (only `internal/files` and the
  `TestManagedHomeTarget` subset of `internal/httpapi` were run), the
  integration suite, and the browser e2e suite.
- The vendored Caddy ingress module (`deploy/caddy/ingress`) — separate Go
  module, untouched by this diff, not built and not scanned by the
  `govulncheck` run above, which covers the root module only.
- `internal/files/files.go` was read at its changed regions and around every
  `openRoot`/`OpenFile` call site, not end to end; `image/entrypoint.sh` and
  `internal/orchestrator/orchestrator.go` were grepped for the mount and
  `.dockerd` facts cited in finding 1, not reviewed.
- The uid/ownership question inside finding 1 (whether the guest user can
  already write `.dockerd/<uuid>.img` directly) was not confirmed on a live
  project; it is stated at medium confidence for that reason.
- `web/` was read only where finding 2 needed it (`api.ts` listing type,
  `FileExplorer.tsx` consumption); no frontend tests were run.

## Outcome

Appended by the main agent, in the commit that carries the fixes.

- **Finding 1 — a symlink below the first component still resolves into a
  managed subtree: fixed**, same fix as the code lens's finding 1, which found
  this independently: `traversesALink` in `internal/httpapi/explorer.go` now
  lstats every component but the last. The red test this review saw appear
  mid-run was that fix's must-fail proof, written before the fix and passing
  after it.
- **Finding 2 — `Truncated` reaches the API and nothing consumes it, so a cut
  listing renders as complete: fixed** at `web/src/lib/api.ts` (the field is
  now part of `listFiles`' response type, with a comment saying why it exists)
  and `web/src/components/FileExplorer.tsx`, which carries it in state and
  renders "This directory has more entries than the explorer lists. Some are
  not shown — use a terminal to see all of them." The finding is accepted in
  full, including its framing: the cap was introduced in this very phase, so
  the concealment primitive was this phase's own doing, and truncation on raw
  directory order before the sort does make the omission arbitrary.
  **Not unit-tested:** `FileExplorer.tsx` is one of the 33 `web/src` files
  with no unit test, and the project has no component-testing infrastructure —
  adding one is a dependency decision, not a side effect of this fix. The
  assertion belongs in Tier 4, and is recorded as a task in Phase 5 of
  `plans/2026-09-22-project-review-remediation.md` rather than claimed here.
  `tsc -b`, `oxlint` and `vitest` all pass, which is type-checking and lint,
  not proof that the notice renders.
- **Finding 3 — lstat-then-act race: rebutted as a race, fixed as a claim.**
  The review's own recommendation was to fix the comment's boundary claim
  rather than the race, and that is what was done: `traversesALink`'s doc
  comment now states that this is not a tenancy boundary, that `os.Root`'s is
  and is atomic, and that the race is winnable by the same principal against
  its own home.
- **`ReadRawNoFollow`'s parent open (info): no action.** The review confirms
  the change does not widen what the function opens — a FIFO parent now fails
  `ENOTDIR` instead of wedging, which is the intent.
- **govulncheck rc 3, two `docker/docker` advisories, Fixed in N/A:**
  pre-existing and already accepted at `deploy/vulncheck.sh:9-13`. Unchanged
  by this diff; left.

Verification after the fixes: as listed in the code lens's Outcome — both Go
modules, `tsc -b`, `oxlint` and `vitest` all rc 0, each run bare.
