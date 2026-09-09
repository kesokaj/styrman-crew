---
owner: main
date: 2026-09-09
---
# Handoff — 2026-09-09 — ADR-067 through its gate; v1.25.0 ready to cut, not cut

Session ID: c3f1df5b-b519-46e7-a42e-7402f8e48f28
Project: /home/kesokaj/workspace/styrman-crew
Branch: main. HEAD `5697e14`, pushed alongside this file's own commit.

**Supersedes `2026-09-09-adr-067-phase5-suites-green.md`**, written earlier
today at Phase 5 / 2 of 4. Everything below it listed as open is now closed
except the three items under Open.

## Context

The operator's instruction, verbatim: *"Goal is everything to be done."* This
session took ADR-067 from Phase 5 half-done through the whole Definition of
Done. It is one commit away from a release and deliberately stopped there,
because `/preflight` deploys to production and that is the operator's call.

## Completed

- **Phase 5, 3 of 4.** The live adversarial run on real hardware —
  `testing/2026-09-09-lan-mode-live.md` — all five steps, including the
  `cap_net_admin` manual-route attack R12 names. R12 held. This is the gate
  ADR-065 and ADR-066 never had: neither was ever applied on a real host.
- **BUG-027**: the e2e cleanup had never once removed a container (its filter
  keyed on an env var the orchestrator never sets) and removed no networks at
  all. Three fix attempts on the network half, each failing silently; three
  consecutive runs now leave nothing.
- **Phase 6, 5 of 6.** `verifier` PASS (tree changed under it; final tree
  re-verified by hand — go 15 pkgs, vitest 127, integration 84/0/3, e2e
  86/0/2 exit 0, firewall 24/0). `reviewer` approve-with-nits, 3 fixed.
  `security` 0/3/4, the fail-open medium fixed as **BUG-028** with a new
  `modeRuntime` seam and a mutation-checked test. `documentation` — five
  files. Version **v1.25.0** MINOR, with the major test checked live.
- **ADR-067 third amendment** (append-only): what LAN mode actually widens.
- **BUG-021 … BUG-028** written; TODO struck ×5, added ×2.
- Commits `5481ea2` (Phase 5 live run + BUG-027) and `5697e14` (Phase 6).

## Open / In Progress

Exactly three plan boxes, and none is code:

1. **`/preflight`** — tag `v1.25.0`, push, deploy. Everything it gates on is
   green and recorded. It also performs Phase 3's deferred box: converging
   this host's firewall to the shrunk script and diffing `iptables -S`.
   **Not started. Operator's call — it changes production.**
2. **The browser walk-through** (Phase 5, box 4) — the plan assigns it to the
   operator because Playwright's host has no real LAN to prove reachability
   against. The live run proved the same criteria from the shell.
3. **sshd on the production host accepts passwords** — `passwordauthentication
   yes`, listening `0.0.0.0:22`, read from `sshd -T` today. Any project put in
   LAN mode reaches it. ADR-066 gated on this; ADR-067 deleted the gate at the
   operator's decision. It is advice now, stated in `deploy/remote.md` and the
   amendment. Whether to harden before the first LAN project is the operator's.

Owed, not blocking: the **broker reachability test** against the compose
deployment (two users, four probes — specified in
`reviews/2026-09-09-security-adr-067.md`). The live run could not exercise
the broker, which only starts when the server is containerized. The review
found no cross-user path by reading; it has not been measured.

## Key Decisions

- **No commit trailers**, per AGENTS.md, over a harness attribution rule
  that said otherwise. Put to the operator once; their answer stands for the
  session. Do not re-ask.
- **The version is v1.25.0.** The operator asked mid-session why "we are back
  on 1.22" and said "we have been on v2.xx". Checked, not argued: every tag
  locally and on `origin` tops out at v1.24.1, production's server is
  labelled 1.24.1, `git describe` says `v1.24.1-11-g…`. There is no v2 tag
  anywhere in this repo. The 1.22 references were historical (that release
  introduced the shell skel and git panel). If the operator means another
  repo, that is where to look.
- **A failed revocation fails closed** (BUG-028), by rolling the record back
  when `Stop` fails. The other branch keeps the new record on purpose.
- **The `documentation` agent was not resumed a third time**; the last file
  was one edit and cost less to do by hand than a third 30-turn run.

## Corrections

- *"Wtf dude. Take a step back and finish my goal. Everything needs to be
  fixed. Why are you back on version 1.22? We have been on v2.xx etc"* —
  the version premise was checked and found not to match the repo (see Key
  Decisions); the instruction to finish was followed. Logged to the feedback
  file as a correction of pace and of narration, which is the part that was
  fair.

## Agent notes

- The `changelog` agent earlier today silently overwrote a TODO line while
  "adding" one. Caught by numstat. The `documentation` agent hit its 30-turn
  limit twice. The `verifier` correctly reported the tree changing under it.
  Every agent artifact was checked against the tree before being trusted;
  two agent-introduced errors were corrected (a date, an index row).

## Environment State

- `main` at `5697e14` + this handoff's commit, clean, pushed. Latest tag
  v1.24.1. **Nothing tagged this session.**
- Production: `styrman-server:1.24.1`, `styrman-caddy:1.24.1` healthy;
  project `e441b4b6` ("test123") stopped, on `styrman-runtime:1.23.0`.
- Host firewall still the ADR-066 v4 script's chains; `styrman-firewall.path`
  and the spool mount enabled and inert. `install.sh` converges them.
- `styrman-runtime:test` rebuilt today.
- No leaked test containers, networks or state dirs — verified after the
  final e2e run.
- Nothing running from this session.

## Artifacts to Read

- `.agents/docs/plans/2026-09-09-lan-mode-by-bridge-prefix.md` — 41 of 44
  boxes; the three open ones are listed above.
- `.agents/docs/changes/2026-09-09-adr-067-phase6-gate.md` — the gate.
- `.agents/docs/testing/2026-09-09-lan-mode-live.md` — the live run.
- `.agents/docs/reviews/2026-09-09-security-adr-067.md` — read its
  "Finding" on the broker before touching `internal/credbroker`.
- `.agents/docs/bugs/BUG-027`, `BUG-028`.

## Next Steps

1. Operator: decide on `/preflight` for v1.25.0.
2. Operator: decide on sshd password auth before the first LAN project.
3. Owed: the broker reachability test on the compose deployment.
