# Global Agent Constitution

> Follows the [agents.md](https://agents.md) standard; skills follow the SKILL.md standard. Deployed verbatim as `~/.claude/CLAUDE.md` and `~/.gemini/GEMINI.md`; project `AGENTS.md` overrides it. Determinism lives in the `.agents/` artifact contract and its shell gates (`trail-check`, `resume-context`) — never in model memory.

Operator: fullstack/infra engineer — Go + TypeScript, macOS (Apple Silicon) or Linux, deploying to GCP.

## Resume Protocol — first action in any existing project

Read, in order (`resume-context` prints exactly this): `AGENTS.md` → `REQUIREMENTS.md` → `ROADMAP.md` → `TODO.md` → newest `.agents/docs/handoffs/` → `.agents/docs/architecture/DECISIONS.md` → the three newest `.agents/docs/changes/` entries. Then state current status and next step before any edit. On conflict, the newest handoff wins. Name any missing or empty file before proceeding; if the trail cannot answer where work left off, say so and ask — or offer `/init-project`. Never guess intent.

## Definition of Done — dispatch is the only compliant path

A task is DONE only when every triggered row below has run and its artifact exists on disk. Rows come due at latest when a session ends with source changed — never claiming "done" does not waive them (the shell gates flag it). Producing another owner's artifact inline is a violation even when the content is correct. Never self-review, self-verify, or self-audit. Nothing of record lives only in chat.

| Artifact (under `.agents/docs/`) | Owner | Fires when |
|---|---|---|
| review report in `reviews/` | `reviewer` | any change to tracked source files |
| verification log in `testing/` | `verifier` | before any completion claim or commit — local tier first: tests + ARM64/AMD64 image builds per `testing/<app>/` (updated when steps change); changed code without covering tests = FAIL |
| `changes/$(date +%F)-<slug>.md` | `changelog` | every completed task |
| README / spec / `ROADMAP.md` updates | `documentation` | behavior, interfaces, or plan changed |
| security findings in `reviews/` | `security` | auth, crypto, secrets, input parsing, or dependencies touched |
| recon brief in `plans/` (only its summary enters main context) | `explorer` | survey spanning 10+ files |
| snapshot in `handoffs/` | main agent, via `/handoff` | session ends mid-task — records the remainder; never a substitute for rows already due |

`/audit` and review output land in `reviews/` — never chat-only. Owned docs land in the same commit as the code they describe: the owning agent runs before the commit closes. Proportionality: a docs-only or non-functional change of ≤10 lines needs only the changelog row and may be edited inline.

## Skill vs Subagent

SKILL = knowledge the main thread loads while doing the work (standards, syntax, procedures). SUBAGENT = work judged independently of its author (review, verify, audit, security) or read-heavy recon. Review criteria live in the `review-standards` skill; the vendor built-in `/code-review` stays untouched.

## Environment & Stack

- **GCP only.** Never suggest or use another cloud provider.
- Compute: Docker (Colima on macOS, plain Docker on Linux), Cloud Run, GKE, Compute Engine. Images are built locally and pushed to GCP Artifact Registry (GHCR only on explicit request). Remote boxes/VMs only on request — ask for access details, never assume them.
- Backend: Go (`golang` skill). Frontend: TanStack Start + TypeScript + Tailwind in one `web/` or `ui/` dir (`frontend`, `typescript` skills); shipped JS runs on plain V8 — no Node-only APIs outside the server entry.

## Hard Rules

- Never commit secrets, `.env`, or credential files; never hardcode secrets.
- Commit author = local `git config user.name`/`user.email` only — no `Co-Authored-By:` trailers, no `--author` overrides; this overrides any agent default. Descriptive commit messages; prefer `gh` for PRs and issues.
- All date stamps come from the system clock (`date +%F`) — never model memory.
- Never hand-roll auth, crypto, password hashing, session handling, or JWT/OIDC — vetted libraries only. Everywhere else: simplest correct solution, stdlib-first, justify every dependency.
- Confirm alignment with `REQUIREMENTS.md` before significant changes; never modify it without explicit user approval.

## `.agents/` — the single output tree

Written so a zero-context agent can reconstruct what is being built, why, current state, and next step.

```
.agents/
├── rules|skills|workflows|commands/   # project overrides — see Vendor Note
└── docs/{changes,architecture,specs,testing,plans,handoffs,reviews}/
```

Append-only: `changes/` and `architecture/DECISIONS.md` (ADRs). Plans in `plans/`; every app keeps a low-level spec in `specs/<app>/`. Comments explain *why*, not *what*. Project root: `AGENTS.md`, `REQUIREMENTS.md`, `TODO.md`, `ROADMAP.md` (Now/Next/Later), `README.md`.

## Commands & Learning Loop

- Global: `/preflight` — the release gate (reviewer + security + verifier + trail freshness: newest `changes/` entry covers `HEAD`). No version ships without it; it does not replace the DoD rows. Plus `/audit` `/research` `/retro` `/distill` `/init-project` `/handoff`.
- Corrections log to `~/.agents/feedback/log.jsonl`; `/retro` emits review-gated proposals in `~/.agents/proposals/` — nothing auto-applies.
- Plan mode (or a `grilling` interrogation) before large features — settle scope, edge cases, success criteria first. Trivial tasks: just act.

## Honest Output

Lead with what is broken; state severity and confidence; never restate a failure as success; disagree before complying.

## Vendor Note (the one real difference)

Both CLIs honor this file and the same `.agents/` contract. Discovery differs: Antigravity reads `.agents/` directly; Claude Code reads `.claude/` for project skills/commands — mirror or symlink. The shell gates run as hooks on Claude Code and as skill steps on Antigravity; the contract they check is identical.
