---
owner: reviewer
date: 2026-09-21
---
# Review — plan cold-read: GCP Compute Engine target   (2026-09-21)

Range reviewed: `.agents/docs/plans/2026-09-21-gcp-compute-engine-target.md` (a plan document — no code exists yet; nothing under `/var/lib/styrman`, `/etc/styrman`, `/opt/styrman` was touched, no `gcloud` command was run).

## Status: COMPLETE

## Verdict: REQUEST CHANGES — `design needs revision`, `implementation not ready`

The worst problem: the plan's own Interfaces section for `install.sh` step 4 specifies a git credential-helper shape that the codebase's own security review already rejected in the near-identical `setup_auto_update` function it claims to follow (D6) — the plan text and its own cited decision disagree with each other. A second concrete gap: the nested-virtualization/`/dev/kvm` assumption is claimed "checked" in Risks but no Phase 2 task performs the check, and `host-setup.sh` does not hard-fail on the condition it is meant to catch. Everything else — the six-key env file, the `--dry-run`/`run()` seam, the `private_file` reuse, the disposability/no-SA/no-Terraform decisions, the docs surface list — is well-reasoned and internally consistent.

## Findings (Gaps)

- **[high / confidence: high]** Plan Interfaces → `deploy/gcp/install.sh` step 4 (plan lines ~195–199) vs `deploy/styrman-ctl:566–571` and plan D6 (line 334). The plan's literal clone command is:
  `git -c credential.helper= -c 'credential.https://github.com.helper=!f() { echo username=x-access-token; printf "password=%s\n" "$(cat ~/.styrman-install/gh-token)"; }; f' clone …`
  — the token-file **path** is interpolated directly into the `-c` config string that becomes the credential helper's shell snippet. `deploy/styrman-ctl`'s `setup_auto_update` (the function D6 says install.sh should follow) does exactly this task today, but was fixed by a named security review to *not* do it this way: "The token path reaches the helper through the environment of the one git call (`STYRMAN_GH_TOKEN_FILE` on the env line), never interpolated into the snippet the helper's shell parses (security review 2026-09-16)" (styrman-ctl:568–571). The plan reproduces the pre-fix shape it explicitly says it is copying. This must be decided before implementation: either the Interfaces text is corrected to route the path through an env var the way `setup_auto_update` does, or the plan states why this occurrence is safe despite deviating from the established, reviewed pattern. As written, two implementers diverge — one follows D6's stated intent (copy the reviewed pattern), one follows the literal script in Interfaces.

- **[medium / confidence: high]** Risks (plan line ~312–313) asserts the nested-virtualization assumption is "checked by `vm.sh ssh -- ls /dev/kvm`, first task after create," but no Phase 2 checklist item performs this check — Phase 2's task list (lines 296–302) goes straight from the machine-types `describe` assumption check to the env-file derivation to `vm.sh create` to `install.sh`, with nothing between `create` and `install.sh` running `ls /dev/kvm`. Worse, `deploy/host-setup.sh:34–42` does not hard-fail on a missing `/dev/kvm` when `/proc/cpuinfo` reports `vmx`/`svm` (it only WARNs and continues) — exactly the situation nested virtualization on an unsupported path could produce (device node not yet present though the CPU flag is exposed). `styrman-ctl install` could therefore report success while Kata is non-functional, surfacing only when the first project's microVM fails to boot — potentially not until E2E criterion 3, deep into the bring-up. The plan needs an explicit, gating Phase 2 task for this check, run and read before `install.sh`.

- **[low / confidence: high]** Why section (plan line 23) cites "(D8)" for the one-time NUC→GCP secrets derivation, but the Decisions section only defines D1–D6 (lines 329–334); no D7 or D8 exists. D4 (line 332) is the decision that substantively covers this ("the derivation is a one-time session command … by: implementer"), so the content is present, but the citation is dangling — a reader following it finds nothing, and may wonder whether an intended decision was dropped rather than mistyped.

## Scope

- `deploy/gcp/vm.sh status --json` (Interfaces, line ~156) is a second output shape for the same `status` operation. No task in Phase 2, no E2E criterion, and no other script in this plan consumes the JSON form; the Why/Scope text never asks for machine-readable status. No D-row names who asked for it — per the rubric this is a gap, though a cheap and low-risk one to build as specified.
- `vm.sh stop`/`start` are not named in the operator's quoted request, but are reasonably motivated by D5 (disposability) and the Risks "Cost" note ("`vm.sh stop` leaves only the disk"); no explicit D-row covers them, but the connection to an existing decision is closer than the `--json` flag's. Lower-severity than the item above.

## Readiness — questions a first-pass implementer would have to ask

1. **BLOCKING** — Should `install.sh`'s git credential helper route the token-file path through an environment variable (matching `setup_auto_update`'s reviewed pattern) instead of interpolating it into the `-c` config string as the Interfaces text currently shows? Two careful implementers build genuinely different scripts here, and one of them reproduces a previously-rejected shape. (Finding 1 above.)
2. **BLOCKING** — Where does the `/dev/kvm` nested-virt sanity check live in Phase 2, and does it gate `install.sh` (hard stop) or only warn? The plan claims it is checked but never schedules it, and the code it relies on (`host-setup.sh`) does not itself hard-fail on the exact condition this check exists to catch. (Finding 2 above.)
3. **NON-BLOCKING** — Is `vm.sh status --json` needed at all, given nothing in this plan consumes it? A reasonable implementer builds it per spec (cheap, harmless) or defers it; either is defensible without asking.
4. **NON-BLOCKING** — Is "(D8)" a typo for D4? An implementer can infer this from context and proceed; worth a one-line fix but not a design fork.
5. **NON-BLOCKING** — Exact shell expression to derive the address's `--region` from `GCP_ZONE` (e.g. `${GCP_ZONE%-*}`) is unspecified but trivial; any correct implementation satisfies the spec.

## Verified against code (no gap found)

- `deploy/styrman-ctl:428–501` (`cmd_install`) is genuinely unattended under `--yes --gh-token-file … --allowed-signers-file …` as the plan claims: `ask()` (line 78–94) takes the default under `YES=1` and dies only on an empty default, and the auto-update branch (lines 484–491) reaches `setup_auto_update` because both file flags are non-empty, matching D2's "auto-updater enabled."
- `MUST_FILL` (styrman-ctl:54) and `deploy/.env.example`'s must-fill block match the plan's six keys exactly.
- `deploy/caddy/ingress/cloudflare.go:137–171` (`findZoneID`) walks progressively shorter domain suffixes, so `BASE_DOMAIN=gcp.styrman.xyz` correctly resolves to the `styrman.xyz` zone — the plan's Cloudflare-scope assumption is sound.
- ADR-075 is indeed the next free ADR number (ADR-074 is the latest in `DECISIONS.md`).
- `.agents/docs/deploy/gcp.md` currently reads "Not applicable," consistent with the plan's Scope claim.
- `deploy/verify-commit.sh:168,170` currently lists `deploy/host-setup.sh`/`deploy/firewall/*`/`deploy/update/styrman-update` for `bash -n`/shellcheck (not `deploy/styrman-ctl` — a pre-existing gap unrelated to this plan); adding `deploy/gcp/*.sh` to those two lines, as the Phase 1 task specifies, is straightforward.
- `.claude/settings.json` grants blanket `Bash` permission, consistent with the plan's "AI, standing root permission" note for the Phase 2 NUC-read task.

## Not examined

- `internal/config/styrman_ctl_test.go` was only skimmed for its fake-tool/`ctlEnv` pattern (first ~60 lines of 889); the full six-test suite's assertions were not cross-checked against the file line by line.
- `deploy/host-setup.sh` was read only through line ~60 (virtualization check, Docker install) plus the cited line 259–260 area was not directly opened (the `GH_TOKEN`/`gh-token` file handling for the image build step referenced in Surfaces).
- `deploy/verify-commit.sh` was read only around lines 150–185; the rest of the file (web/CSS check earlier, anything after line 185) was not read.
- The actual GCP project state (`driven-eft-h4it`) was not queried — per the dispatch's instruction, no `gcloud` command was run; the Surfaces section's GCP inventory is taken on faith from the plan text.
- ADR-053 (version-format rule) and ADR-052 (state-dir convention) were not opened directly; the plan's citations to them were taken at face value.
