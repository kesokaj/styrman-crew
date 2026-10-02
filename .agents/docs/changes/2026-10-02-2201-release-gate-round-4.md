---
owner: main
date: 2026-10-02
---
# The last five lows, fixed before the tag   (2026-10-02)

The release gate's third review round over `57179a4..624808d` found five lows
and nothing above. That was the review cap. The operator chose "Fix all five,
then ship".

- **The lifecycle wait has a ceiling.** `GuestGoingAway`
  (`internal/orchestrator/ports.go`) waits at most `lifecycleWaitMax` = 20 s,
  whatever the request's own deadline. Before, a client that never gave up
  could keep it asking Docker ten times a second through a toggle's 90 s
  `waitReady`. Test `TestListeningPortsWaitHasACeiling` (security, finding 1).
- **The status path takes plain characters only.** `write_status` refuses any
  path outside `^/[A-Za-z0-9._/-]+$`. A newline cut the ancestor walk short,
  and a status was written below a 0777 `ok<NL>evil`. Test
  `TestStyrmanUpdateStatusWriteRefusesAPathOutsideThePlainSet` (security,
  finding 2). Docs: `versioning.md`, `deploy.md`.
- **The test helper names a bad TMPDIR.** `trustedTempDir` stops the test
  naming the first ancestor the writer would refuse, instead of six
  unexplained failures (code review, finding 1).
- **A stale test header** in `ports_lifecycle_test.go` now describes the
  per-tick ask and the ceiling (docs lens, finding 1).
- **The dead `Config.IngressNetwork`** is removed, with its comment and its
  `STYRMAN_INGRESS_NETWORK` read. Nothing used it since ADR-067 (docs lens,
  finding 2).

Each test failed before its fix. Counts: `TestStyrmanUpdateStatus*` 13,
`ports_lifecycle_test.go` 7. Every round-3 report ends with an `## Outcome`
section. BUG-088 carries the ceiling as its fourth step.

## Learnings for the local host
None.
