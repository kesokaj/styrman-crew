---
owner: main
date: 2026-09-04
---
# E2E — un-nesting the project homes: upgrade fixture (2026-09-04)

Run by the main thread; script `.agents/scratch/unnest-fixture.sh` (gitignored), log copied below. Isolated state dir and loopback server, no production involved. The v1.20.0 server was built from its tag to lay the state dir out the OLD way — a project VM, a setup VM whose bind is the tenant dir, and planted markers — then the working-tree server was started on the same state dir.

```
old setup bind: …/state/homes/kesokaj                         (the tenant dir itself)
old tenant dir entries: .bash_aliases .bash_exports .bashrc .bash_secrets .npmrc p .profile projects .ssh .styrman .tmux.conf

NEW server:
"msg":"migrated base home","user":"kesokaj","to":"…/homes/kesokaj/base","entries":10,"failed":0
tenant dir now: base p
base now: .bash_aliases .bash_exports .bashrc .bash_secrets .npmrc .profile projects .ssh .styrman .tmux.conf
ok: base/.styrman/setup-done        ok: base/projects/legacy-ws/keep.txt        ok: p/<uuid>/keep.txt
setup status: {… "done":true,"vmRunning":true}                 (the marker is read from base/)
"msg":"setup VM home moved — recreating","user":"kesokaj","want":"…/homes/kesokaj/base"
new setup bind: …/state/homes/kesokaj/base
inside the setup VM: .bash_aliases … .tmux.conf — guest cannot see p/
project ready (new server); its keep.txt intact
```

Verdict: PASS — the plan's edge case (upgrade from the old layout) and its golden path (a setup VM whose mount holds no project home) both hold; the migration is idempotent (unit test) and the stale container recreation needs no operator step.

## Third run — after the security round (startup sweep before the migration)

Same fixture, the tree at Phase 1's commit `f3882ac`. The server log now shows the intended order: the pre-ADR-064 setup VM is force-removed at startup, then the base home migrates, and the next `/ws/setup` creates a fresh VM bound to `base/` (the lazy "setup VM home moved — recreating" line no longer appears because nothing stale is left to find):

```
"msg":"stale setup VM removed (home layout changed, ADR-064)","user":"kesokaj"
"msg":"removed stale setup VMs (home layout changed, ADR-064)","count":1
"msg":"migrated base home","user":"kesokaj","to":"…/homes/kesokaj/base","entries":10,…
new setup bind: …/homes/kesokaj/base        guest cannot see p/        project ready; keep.txt intact
```

Verdict: PASS (three runs: the lazy path, then the sweep-first path twice).
