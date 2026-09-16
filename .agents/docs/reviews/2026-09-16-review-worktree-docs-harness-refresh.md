---
owner: reviewer
date: 2026-09-16
---
# Review — worktree-docs-harness-refresh   (2026-09-16)

Range reviewed: working tree, image/harness (uncommitted refresh of the mirror from a81ef4b to 4aa19be; `image/harness/COMMIT` and `SHA256SUMS` confirm the target)

Docs lens only, per dispatch. One living doc now contradicts the mirrored code's behaviour; it sits in `REQUIREMENTS.md`, which is immutable without the operator. Everywhere else the same subject was checked, the wording already matches (notably `image/entrypoint.sh`, already correct and evidently the reference text for this change).

## Findings

- **[high / confidence: high]** `REQUIREMENTS.md:49` — R5 states the agent harness is "installed by `sh-agent`; no hooks, scripts, or settings files". Since harness commit 42ba2f5 (inside this refresh's range, a81ef4b..4aa19be), `sh-agent install` writes `~/.claude/settings.json`: creates the file when absent, inserts `"attribution": {"commit": "", "pr": ""}` when that key is missing, and leaves a present key alone — see `image/harness/cmd/sh-agent/main.go:477-490` (`attributionSet`) and `:646-661` (`attributionHint`, the advisory note `install`/`status` print). "no hooks, scripts, or settings files" is now false for the settings-file clause. This is a `REQUIREMENTS.md` line — flagging per instructions rather than editing; changing it needs the operator's explicit approval, then an ADR, per the global workflow (the harness's own ADR-025 already records the exception on its side).

- **[low / confidence: high — not a defect, noted for completeness]** `README.md:174-175` uses the identical phrase ("no hooks, scripts, or settings files") inside the dated v1.20.0 release-history bullet (2026-09-04). That bullet is a historical record of what was true when v1.20.0 shipped, in the same append-only spirit as `.agents/docs/changes/` entries embedded in README's "one line per release" summary — it predates the attribution change and is not read as a statement of current behaviour. Not raising it as a live contradiction, but note it sits right beside the REQUIREMENTS.md finding and uses the same words, so a reader skimming both could be misled if REQUIREMENTS.md is fixed without a pointer to why the old bullet still says the old thing.

- No contradiction found for the other five behaviour changes named in the dispatch: `image/entrypoint.sh:110-133` already documents the settings.json/attribution write, the "file copier" description, and the plain-copy/no-git/no-network install accurately (this text is apparently the up-to-date reference, not part of the uncommitted diff). No living doc in scope describes the verifier's `testing/<app>/guide.md`-on-first-use behaviour, the security exemption's "hand-rolled" definition, the feature skill's cold-read cap, or the init-project deploy template's placeholders in a way that conflicts with the new code — grepped `guide.md`, `count=1`, `cold-read`, `hand-rolled`, `placeholder`, `sh-agent status`, `sh-agent install`, `no hooks`, `file copier`, `copies the context`, `NOT credentials`, `first use` across `README.md`, `AGENTS.md`, `REQUIREMENTS.md`, `.agents/docs/specs/`, `.agents/docs/deploy/`, `.agents/docs/testing/styrman/README.md`, `.agents/rules/`, `image/entrypoint.sh`, `image/configs/*.sh`, `image/Dockerfile`; no other hits describe the changed behaviours.

## Criteria

1. Dead code — n/a — docs lens
2. Flow tracing — n/a — docs lens
3. Standards conformance — n/a — docs lens
4. Directory layout — n/a — docs lens
5. Simplicity (DIY-except-security) — n/a — docs lens
6. Decoupling — n/a — docs lens

## Not examined

`.agents/docs/testing/styrman/README.md` was grepped but not read in full (large file; only the subject-noun terms were checked, no hits). `.agents/docs/specs/styrman/*.md` beyond `deploy.md`, `api.md`, `terminal-protocol.md` were grepped, not fully read — no hits on the given subject nouns so not opened further. `.agents/rules/*` and `.agents/skills/`, `.agents/agents/` in styrman-crew are effectively empty/absent of harness-behaviour content (confirmed no matches). `image/harness/*` internals beyond the grepped subject nouns were not reviewed for correctness — out of scope for a docs-only lens per the dispatch. `ROADMAP.md` and `TODO.md` were not checked (not named in the dispatch's doc list).

## Outcome

- Finding 1 (high) — fixed with the operator's approval (2026-09-16, "Amend crew's requirement: allow that one key"): `REQUIREMENTS.md` R5 now reads "no hooks or scripts, and no settings file except the one key the harness's own R4 allows — `attribution` in `~/.claude/settings.json`, commit and PR trailers off, written only when absent — … the exception approved 2026-09-16 (ADR-072)"; `image/entrypoint.sh`'s comment says the same. ADR-072 is written by the changelog agent in the refresh commit.
- Finding 2 (low, historical) — left: a dated release-history bullet.
- Round 1 closed 2026-09-16.
