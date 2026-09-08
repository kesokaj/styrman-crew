---
owner: bugs
date: 2026-09-08
id: BUG-009
status: fixed
area: deploy/firewall (install.sh)
---
# BUG-009: root installer chowned/chmodded through a guest-plantable symlink — same class fixed once already

## Symptom
Security review (`.agents/docs/reviews/2026-09-08-security-lan-phase1.md`,
finding 1): `deploy/firewall/install.sh:97-99` did root `mkdir -p
"${STATE_DIR}/netpolicy/lan"` then `chown`/`chmod` on both levels.
`/var/lib/styrman` is 0755 owned by the server's uid, which can pre-create
`netpolicy` (or replace `lan`) as a symlink — e.g. `ln -s /etc
/var/lib/styrman/netpolicy` — and the next root run of `install.sh` hands
`chown`/`chmod` the symlink's target (`/etc`). Mechanism verified in scratch
by the security agent. Separately (finding 2, medium): `install -d -m 0700 -o
root "${ETC_DIR}"` re-moded/re-owned an *existing* directory, a verbatim
repeat of a finding already fixed at `deploy/update/styrman-update:139-152`
(the earlier `--check` chmod-0700'd `/tmp` itself).

## Repro
Not reproduced against a live host; mechanism confirmed in scratch
(`mkdir -p` traverses a symlink-to-directory; `chown`/`chmod` follow it).

## Root cause
`install.sh` was written fresh rather than reusing the primitive already
hardened in `styrman-update` for the identical shape (root creating/owning a
path below a lower-privileged uid's directory, and re-moding a
possibly-existing directory). The existing fix was not searched for before
writing the same code again.

## Fix
Plain `mkdir` per level with symlink refusal and owner assertion before
`chown -h`; `install -d` guarded by `[ -d ] ||` then assert ownership/mode
instead of forcing them, matching `styrman-update:148-152`.
`deploy/host-setup.sh:170-171` has the identical shape for `homes`/`projects`
and is called out to be fixed in the same pass.

## Prevention
Root code that creates or chowns a path below a lower-privileged owner's
directory never uses `-p`/`-R`; it asserts non-symlink + expected owner at
each level before touching it, then `chown -h`. `install -d` on a path that
may already exist is always guarded by `[ -d ] ||`. Before writing either
primitive, grep the repo for the prior fix (`styrman-update:139-152`) —
this is `.agents/rules/guest-writable-paths.md`'s family, now with a second
instance; if a third recurs, promote the mkdir/chown pattern into a shared
helper or an explicit rule file.

## Related
`.agents/rules/guest-writable-paths.md`; BUG-002/BUG-003 (same symlink-race
class in Go code); BUG-008 (same review, styrman-update).
