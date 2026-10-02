---
owner: main
date: 2026-10-02
---
# What the release gate's reviews of 5571b57..57179a4 found, and what was done   (2026-10-02)

`/preflight` for the round of `plans/2026-10-02-stale-keys-browser-liveness-updater-status.md`.
Its reviews of the review-fix commit `57179a4` found nothing high. Each of the
three reports ends with an `## Outcome` section. The operator, during this
pass: "Include in the version go".

## Docs lens (`reviews/2026-10-02-review-5571b57-57179a4-docs.md`)
- **Medium: a false row.** The `STYRMAN_INGRESS_NETWORK` row added to
  `api.md` in `57179a4` described a cross-check that ADR-067 deleted. Nothing
  reads the variable. The row is gone. The dead `Config.IngressNetwork` field
  is a follow-up in the plan. This entry corrects the earlier
  `changes/2026-10-02-2100-review-fixes-v1.40.0-5571b57.md`, which said that
  row was added.
- **Low:** `api.md`'s `/ports` row now says a project being stopped or rebuilt
  answers an empty list (BUG-088).

## Code review (`reviews/2026-10-02-review-5571b57-57179a4-code.md`, APPROVE WITH NITS)
- **The wait loop checked only the lock.** A mode toggle holds the lock up to
  90 s, past the page's patience. `GuestGoingAway` now asks Docker on every
  100 ms tick and answers as soon as the container is down. New test
  `TestListeningPortsAnswersAsSoonAsTheGuestIsGone`, which fails against the
  previous loop. The lifecycle tests wait on the fake's recorded calls, not a
  sleep (6 tests).
- **Weak tests and stale comments.** The updater hook tests run in a scratch
  working directory. Three comments that still read "a held lock means
  going away" were rewritten.

## Security review (`reviews/2026-10-02-security-5571b57-57179a4.md`, two lows)
- **The status path's ancestors.** `write_status` now walks every directory
  from `/` to the status directory's parent (`status_ancestors_ok`). Each must
  be a real directory owned by root or the updater's uid that no one else can
  write, or else sticky. New test
  `TestStyrmanUpdateStatusWriteRefusesAWritableAncestor` (a 0777 ancestor
  refused; the same with the sticky bit accepted) failed before the walk.
  "like the state dir's" is gone from the comment and both specs. This entry
  corrects `57179a4`'s claim that finding 2 of the earlier security review was
  fully fixed. No `/var/lib/styrman` refusal was added: its must-fail test
  would aim the writer at the real production tree (ADR-052).
- **Left to the operator:** how both release-signing keys are protected.

## Counts
`TestStyrmanUpdateStatus*` 12; `ports_lifecycle_test.go` 6; web unit 211
in 18 files. The `documentation` agent wrote the spec edits and the `bugs`
agent updated BUG-088; both were read against the code before staging.

## Learnings for the local host
On this host, Go's `t.TempDir()` is group-writable under umask 002. A test
whose code refuses writable ancestors needs 0755 fixtures (`trustedTempDir`).
