---
owner: main
date: 2026-09-08
---

# LAN parity Phase 4: a LAN-enabled project is no longer served the SSH key   (2026-09-08)

Plan: `.agents/docs/plans/2026-09-08-lan-full-parity.md` (Phase 4, 4 of 4
boxes). ADR-066 part (4).

## What it closes, and what it does not

Withholding the operator's SSH key from a LAN-enabled project closes the
**transitive** path ADR-065 follow-up (g) recorded: project → a LAN host that
trusts that key → back to this one. It is **not** what keeps a project off
this host; this host has no `authorized_keys` at all, verified 2026-09-08,
and the code comment says so explicitly so the reasoning is not later
mistaken for a control it is not.

## The union rule

`netpolicy.Exposed(record, uuid, status, now)` sits beside `Derive` and
returns true when **any** of these holds:

1. the project record says `LanAccess` — from the instant it is requested,
   because the rule can be in the kernel 0.2 s later;
2. root still lists the project as applied — this is the one that matters,
   and it covers DISABLE: the record flips at once but the rule may live for
   up to a minute, and re-serving the key into that window would hand one
   project both LAN reach and the key, the exact pairing this phase exists to
   prevent;
3. the status is absent or too old to act on — BUG-014's rule, "cannot
   confirm is never off", applied to the credential side.

Record-alone was the obvious implementation and is wrong; the cold reader
raised it as a gap and it is now the tested behaviour, with eight table cases.

## Who answers it

`*httpapi.Server`, because it is the only type holding both the per-user
project stores and root's last word. The orchestrator that serves as the
broker's `Resolver` has neither — it has no `internal/project` dependency at
all. The Server is built at `main.go:104`, well before the broker at
`main.go:180`, so it is simply passed in.

One guard matters: a deployment with no host firewall half has no LAN feature,
so only the record can count there. Without `netpol.Enabled()` every local dev
run would read as "cannot confirm" and silently strip every project's SSH key.

## Fail closed, including the wiring

`isHostReachingFile` matches `~/.ssh/...`, `$HOME/.ssh/...` and `.ssh/...`,
and deliberately not `~/.sshfoo/` or a file merely named `.ssh-notes.md`.
The broker withholds when the checker says exposed, when it returns an error,
**and when it is nil** — a wiring mistake costs ssh push loudly rather than
granting silently.

## The prune, and why its test was mutation-checked

Enabling also removes any copy already materialized in the project's home by
an earlier login shell. The paths come from the credential store, so the
prune covers exactly what the broker withholds instead of a hardcoded list
that could drift from it.

The project home sits under a tree the guest can write, so a component of
`~/.ssh` can be a symlink aimed elsewhere — BUG-002/003's shape. Deletion
therefore goes through `internal/files`, confined to the tenant base.

The test that asserts this was **mutation-checked rather than trusted**:
replacing `files.Remove` with `os.Remove` makes it fail, reporting the victim
file outside the tenant tree deleted, and restoring the confined call makes
it pass. A test for a confinement property that has never been shown to fail
without the confinement is not evidence of anything.

This remains forward-looking and the ADR says so: a guest that copied the key
elsewhere still has it, so enabling LAN on a project that has already run
means that key **was** exposed to it.

## Verification

- `go vet ./...`, `go test ./...`, the vendored ingress module: clean.
- `deploy/firewall/test.sh` untouched at 120/0 — no shell changed this phase.
- New tests: 8 `Exposed` cases in `netpolicy`, `isHostReachingFile` over ten
  paths, four broker withholding scenarios (not exposed / exposed / checker
  error / nil checker), and three in `internal/httpapi` covering the prune,
  the planted symlink, and `LanExposed`'s five states.
