---
owner: main
date: 2026-09-21
---
# Handoff — 2026-09-21 — GCP target built and half brought up; the release is the operator's next move

Session ID: b8591b58-b4fe-48fa-a1f7-4ac3bbcce57f        (same machine: `claude --resume b8591b58-b4fe-48fa-a1f7-4ac3bbcce57f`)
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

The operator asked for a way to install styrman in their GCP project — temporary for now, reusable later ("This is just so we have a deploy process for gcp that I can use later") — without touching the NUC (`dave`, crew.styrman.xyz). Plan `.agents/docs/plans/2026-09-21-gcp-compute-engine-target.md` (active, ROADMAP "Now" names it); ADR-075.

## Completed

- `deploy/gcp/` (`vm.sh`, `install.sh`, `lib.sh`, `vm.conf`) + `internal/config/gcp_scripts_test.go` (11 tests) — committed e5bd830, docs follow-up 9a0e962, both pushed. Guide: `.agents/docs/deploy/gcp.md` (defaults recorded: project `driven-eft-h4it`, zone `europe-north2-a`, `n4-standard-8`, no service account, public 80/443 only, ssh via IAP).
- Learnings applied to the shared path (D14): `deploy/host-setup.sh` Kata step (BUG-070), `deploy/styrman-ctl install` exporting `GH_TOKEN` from `--gh-token-file` (security review).
- Real GCP: instance `styrman` RUNNING at 34.51.142.50 with `/dev/kvm`, Docker CE 29.7.2, `~/styrman-crew` cloned at v1.36.0, `block-project-ssh-keys=TRUE`, no `ssh` tag; static address `styrman-static` reserved. Secrets dir `~/.config/styrman/gcp/` on this machine (700/600, derived from the NUC's root files — the exact pipeline is in the change entry).
- Trail: change entry `.agents/docs/changes/2026-09-21-gcp-compute-engine-target.md`; three cold reads, three verifier logs (all PASS), code review + security review with `## Outcome`, docs-lens review; BUG-069/070/071; retro `.agents/docs/retros/2026-09-21-retro.md`.

## Open / In Progress

- Plan Phase 2, first unticked box: "Operator: release the fixes (`/preflight`, next MINOR), then `deploy/gcp/install.sh`". v1.36.0's `host-setup.sh` cannot install Kata on a fresh host (BUG-070); `install.sh` installs from the released tag only, so the bring-up completes only after the release. The VM bills while it waits (`deploy/gcp/vm.sh stop` keeps only the disk).
- Then (AI): `deploy/gcp/install.sh` → expect `302 https://auth.gcp.styrman.xyz/…` and `vm.sh status` exit 0; E2E criteria 2 and 4; tick the plan; Phase 3 checkpoint; close the plan and clear ROADMAP "Now".
- Then (operator): add the redirect URI `https://auth.gcp.styrman.xyz/oauth2/google/authorization-code-callback` to the Google OAuth client; sign in, create a project, `sh-agent version` (E2E criterion 3).
- Retro proposals 1–2 (project rules `shared-path-learnings`, `remote-check-names-transport-first`) await the operator's yes/no; 3–4 queued for the harness.
- Requirements edits awaiting approval: none.

## Key Decisions

- The plan's D1–D22 (Compute Engine VM on the unchanged host path; gcloud bash scripts, no Terraform; one secrets directory; IAP-only ssh after the bring-up's evidence; disposable instance; learnings back to local). Rejected: Cloud Run/GKE (no KVM), E2 (no nested virt), an attached service account, a public port 22.
- Cold-read cap reached at round 3 (`implementation ready`, three low items); the operator's override "Proceed (Recommended)".

## Corrections

- "make sure you make it simple do deploy in gcp" — feedback log 2026-09-21T21:46:25+02:00
- "take your learnings back to local if any" — feedback log 2026-09-21T21:46:25+02:00

## Agent notes (unverified)

- The port-22 drop between this machine and the VM was attributed to the home router/ISP from Google's firewall log (this host's SYNs stopped arriving; scanners' did not); the device was not identified. IAP sidesteps it; the public path (`GCP_SSH_VIA_IAP=0` + the `ssh` tag) is untested from here.
- `gcloud compute ssh` forwards `LC_ALL=en_US.UTF-8`, which the Debian GCE image lacks (cosmetic warning; plan Follow-up).
- Agents this session: three cold reads (reviewer), verifier ×3, reviewer, security, bugs ×3, documentation, changelog (ADR-075), reviewer (docs lens). Corrections to agent drafts: none needed; the security review noted the tree moved under it (the review fix landed mid-read) and pinned its findings to file hashes.

## Environment State

- Branch: main, clean after this commit; e5bd830 and 9a0e962 pushed; no tag created.
- Running processes: none here. GCP instance `styrman` RUNNING (billing).
- Pending: nothing uncommitted.

## Artifacts to Read

- `.agents/docs/changes/2026-09-21-gcp-compute-engine-target.md` — what changed, every verification, findings and outcomes, the learnings, the state.
- `.agents/docs/deploy/gcp.md` — the procedure, prerequisites, verify, teardown.
- `.agents/docs/plans/2026-09-21-gcp-compute-engine-target.md` — the unticked boxes and D1–D22.
- `.agents/docs/bugs/BUG-069-gcp-ssh-outage-misdiagnosed.md`, `.agents/docs/bugs/BUG-070-kata-download-url-stale.md`, `.agents/docs/bugs/BUG-071-mutation-restore-by-sed-corrupted-install-line.md` — the bring-up's three findings and their prevention rules.
- `.agents/docs/retros/2026-09-21-retro.md` — the four proposals.

## Next Steps

1. Operator: `/preflight` the next MINOR (new `deploy/gcp` surface; two shared-path fixes). Then, same session: `deploy/gcp/install.sh` (defaults to the newest tag), tick the plan's Phase 2 boxes from its output, close the plan (`/preflight`'s gate runs the verifier; the fresh-host Kata path is exercised by this very install — say so in the gate report).
2. Operator: the OAuth redirect URI, then the browser check (E2E criterion 3).
3. Operator: answer retro proposals 1–2 (apply / reject); the next session applies approved ones under the Definition of Done.
4. If the VM will idle for days: `deploy/gcp/vm.sh stop`.
