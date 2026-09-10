---
owner: main
date: 2026-09-10
---

# Broker-test user leftovers removed     (date: 2026-09-10)

## Summary

Host-side cleanup only. On operator instruction, removed three directories left behind when the `broker-test-b` user was deleted from the allowlist: `/var/lib/styrman/users/broker-test-b` (8 K), `/var/lib/styrman/homes/broker-test-b` (141 M, containing the seeded home for the already-deleted test project), and `/var/lib/styrman/projects/broker-test-b` (the project workspace). Directories were inspected before deletion (`du`, `find -maxdepth 2`) confirming no running container referenced them and no other user's store mentioned the name. After cleanup, only `ostling-simon` remains under both `users/` and `homes/`. Server remained healthy throughout.

## Type

task

## Context

Removing a user via the API is an allowlist change and leaves their data in place by design (ADR-052: no agent touches `/var/lib/styrman` unbidden). Manual removal was authorized by operator instruction.

## Verification

- Pre-deletion inspection: `du -sh /var/lib/styrman/{users,homes,projects}/broker-test-b` confirmed sizes (8 K, 141 M, workspace).
- Pre-deletion confirmation: `find /var/lib/styrman/{users,homes,projects} -maxdepth 2` showed no cross-references to broker-test-b in other user stores.
- Post-deletion: only `ostling-simon` present under `users/` and `homes/`; `docker ps` confirmed no container held stale references.
- Server health: no errors or warnings in logs post-cleanup.

## Requirements

n/a
