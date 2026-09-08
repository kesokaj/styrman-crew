---
owner: main
date: 2026-09-08
---

# Audit fixes   (2026-09-08)

Report and full dispositions:
`.agents/docs/reviews/2026-09-08-audit-project.md`. Commits `271a7e8`,
`00dd134`, `80765ca` (and `05db066` for the spec drift).

Ten findings fixed, one identified as a false finding, nine deliberately not
fixed with reasons recorded. Every fix has a test that fails without it —
each was mutation-checked by re-introducing the defect and watching the suite
go red, because this session has already produced several tests that asserted
nothing.

## The one that mattered

`internal/files` sets `O_NONBLOCK` on its opens and its comment says exactly
why: a guest-planted FIFO or device would otherwise wedge the goroutine
forever. Six sibling opens on the **same guest-writable trees** did not have
it, and `history.searchOne`'s `IsRegular` check sat *after* the blocking
open, so it never ran. Each blocked request pins a goroutine *and* an OS
thread; enough of them walk the runtime into its 10k-thread limit and the
process death takes every tenant with it — a cross-tenant DoS reachable by
any project owner from inside their own microVM.

## The shape, again

Three of the fixes are the same story: a guard correct where written, absent
where not. The FIFO discipline. `styrman-update` asserting `.git` ownership
but not the worktree root it then builds as root — its own comment names that
exact threat. The ingress stripping Caddyfile-significant bytes from
hostnames but writing label-supplied emails verbatim into the same file.

## Two mistakes made while fixing, both caught before pushing

- Fixing the updater, I called `trusted_dir` at line 171 where it was defined
  at 376 — **the third call-before-definition of the day**, this time in
  root-run code, and `bash -n` and shellcheck both accept it. The durable
  answer was to extend `test.sh`'s ordering check to cover `styrman-update`
  too, which also gives that script its first automated test of any kind.
- I wrote a comment saying `files.openRoot`'s fallback "is logged loudly" and
  did not implement the logging — the same false-claim shape as `dc5bad4`
  earlier today. Caught on re-read; the warning is now real.

## A gate that worked

`verify-commit.sh` failed the first time it ran with the new dependency
guard, because the `go.mod` change was still uncommitted and the gate checks
out the **commit**, not the working tree. That is precisely the v1.18.0
failure it was built for, demonstrated on itself.

The guard blocks on a `docker/docker` divergence and warns on the eight other
shared dependencies that still disagree between the modules; making all eight
blocking today would either break the gate or force an untested eight-package
bump, so `TODO.md` carries that work.
