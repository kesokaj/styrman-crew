---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 2 — the gates that measured the wrong thing   (2026-09-22)

Phase 2 of `plans/2026-09-22-project-review-remediation.md`, acting on findings
1 and 8 of `reviews/2026-09-22-review-project-merged.md`. Both changes are to
`deploy/verify-commit.sh` — the gate the release procedure runs before a tag is
signed — and both were green on arrival, which is exactly why each one had to
be seen red before it could be believed.

## What changed

- **The web/dist freshness check compares content, not file names.** It built
  a `find client -type f | sort` set before and after a rebuild. The bundles
  carry a content hash in their filenames, so a changed bundle changed the set
  — but `_shell.html` and `favicon.svg` have fixed names, so an edit to
  `web/index.html` or the favicon committed without a rebuild compared EQUAL.
  It now compares `find client -type f -exec sha256sum {} + | sort -k2`, and
  the failure message distinguishes a differing hash on the same path from a
  path present on only one side.
- **The shellcheck list reaches the shared fresh-host path.** It covered
  `deploy/firewall/*.sh`, `deploy/update/styrman-update` and
  `deploy/gcp/*.sh`. It now also covers `deploy/host-setup.sh`,
  `deploy/styrman-ctl` and `image/entrypoint.sh` — BUG-068 names the first
  two, and `.agents/rules/shared-path-learnings.md` makes `host-setup.sh` THE
  shared fresh-host path, the one file a second target proved could be broken
  upstream for months while every run on an already-installed host stayed
  green (BUG-070). The `bash -n` line in the same section was missing
  `deploy/styrman-ctl` and `image/entrypoint.sh` too; both added. All three
  were clean when added, so this buys the NEXT change to them, not this one.

## The proof

Neither fix is worth anything on the word of the person who wrote it. Both were
run in the must-fail and the must-pass state.

**The freshness check, against the real gate on a real commit.** A throwaway
local branch (`phase2-mustfail-probe`, never pushed, deleted afterwards)
carried one content-only edit to `web/dist/client/favicon.svg` — the file name
unchanged, which is precisely the case the finding names.

| Gate version on that stale commit | Result |
|---|---|
| **Old** (`find … \| sort`) | **rc 0 — ran to completion and printed `committed assets match a fresh build`** |
| **New** (`sha256sum` set) | **rc 1 at `==> web/dist freshness`**, with the new message |

The old gate did not merely miss it in passing: it went on through the host
scripts, gofmt, the frontend suite and the server image build and passed the
whole commit. A release tag signed on that evidence would have shipped a stale
asset inside `//go:embed all:dist/client`.

**A detail worth recording.** The first probe edited `_shell.html` instead, and
the gate failed earlier than expected — at `go test -race`, in
`internal/httpapi/server_test.go:62`, which asserts the served SPA index
contains `<title>crew</title>`. So that one string inside `_shell.html` is
incidentally covered by a unit test; nothing else in the file is, and
`favicon.svg` is covered by nothing at all. The probe was moved to the favicon
rather than declaring the finding already handled.

**The shellcheck list**, with an SC2034 (warning-level) unused global planted
in `deploy/host-setup.sh`:

| Line | Result |
|---|---|
| **Old** (without `host-setup.sh`) | **rc 0 — the defect was invisible to it** |
| **New** | **rc 1**, naming the file, the line and the rule |

`deploy/host-setup.sh` was then restored from a checksummed copy and verified
byte-identical, and the new line re-run: rc 0.

**A second detail.** The first shellcheck probe planted `ls $d` — SC2086, which
is *info* severity and sits below the gate's `-S warning` threshold, so it did
not trip. That was a defect in the probe, not in the gate, and it is recorded
because a probe that fails to fire is indistinguishable from a check that does
not work unless you look at why.

## Verification

Each command run bare, status read (`.agents/rules/verify-bare.md`):

- `bash -n deploy/verify-commit.sh` → 0
- `shellcheck -S warning deploy/verify-commit.sh` → 0
- The four probe runs tabulated above
- `deploy/verify-commit.sh HEAD` on this phase's own commit — the must-pass
  state of both changed checks, recorded in the commit that follows this entry

No behaviour of the platform changed: this phase touches only the gate.

## Learnings for the local host

The shellcheck widening is itself a learning taken back to the shared path:
`host-setup.sh` is where BUG-070 hid, and it is now linted by the same gate
that lints the GCP scripts written after it.
