# UI + git improvements phase 1: GOPATH to ~/.go     (date: 2026-09-04)

## Summary

Guest shell skel files rewritten to place `GOPATH` at `$HOME/.go` instead of `$HOME/go`, keeping the home's top level clean. Scope is new project homes only — existing homes keep their `$HOME/go` directory due to the write-once-per-file guard in `entrypoint.sh:84-92` (`[ ! -e "$dst" ]`); anything already `go install`-ed there stays on the old path. Runtime image rebuilt AMD64 and confirmed to carry the new skel. Integration and unit checks green (65 → 68 total checks; new "shell skel" section). `REQUIREMENTS.md:31` uses `~/go` as prose example but was deliberately NOT edited — immutable without explicit operator approval per AGENTS.md hard rules.

## Type

feature, phase 1 of 6 in v1.22.0

## Files changed

- `image/skel/bash_exports:7` — `export GOPATH="$HOME/go"` → `export GOPATH="$HOME/.go"`
- `image/skel/bashrc:24` — PATH `$HOME/go/bin` → `$HOME/.go/bin` (unchanged guard: `[[ -d ]]`)
- `README.md:34` — prose example `~/go` → `~/.go` (single occurrence in context block)
- `.agents/docs/specs/styrman/runtime-image.md` — new bullet in item 6 documenting that skel changes reach only new project homes (guarded by `[ ! -e "$dst" ]`, one file per home creation, NOT version-keyed); existing homes unaffected by version bumps

## Agents dispatched

reviewer (three rounds: cold-read v1, v2, v3 at `.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements*.md`); no security (Phase 1 touches no auth, crypto, secrets, IAM, parsing, or dependencies).

## Verification

Unit: `bash -n` syntax check on all changed shell files (all PASS). Git: `git grep GOPATH` confirms no other prose/code references. Config: runtime image rebuilt AMD64 (`docker build image/`, exit 0, x86_64 platform), unpacked and verified new skel carries the `.go` path. Integration: three new checks in a new "shell skel" section exercised against a container seeded exactly like the entrypoint's own loop (lines mirroring `:84-92` with the write-once guard). Checks confirm `GOPATH=/home/<user>/.go`, `$PATH` includes `$HOME/.go/bin` once the directory is created, and a fresh-project checkout reads the correct env (test uses `bash -lic` because skel bashrc returns early for non-interactive shells, lines 7-9; a real styrman terminal is interactive login). Full integration.sh run (68 checks) PASS against real Kata.

Edge case — the integration checks were written wrong first: they used `bash -lc` (non-interactive), which triggered the early return and left GOPATH empty. Caught before commit, fixed to `bash -lic`. Tests also discovered that `~/.go/bin` only joins PATH once the directory exists, so checks create it and read PATH from a SECOND shell invocation.

## Decisions

1. **New project homes only.** Operator decision, verified in code: `entrypoint.sh:46` creates the user with `useradd -M` (no `/etc/skel` copy), and the skel loop at `:84-92` is guarded by `[ ! -e "$dst" ]`. Second and subsequent runs over an existing home skip the loop entirely — existing paths survive and are NOT overwritten on STYRMAN_VERSION bumps. Anything already `go install`-ed in an old project stays on `$HOME/go` path.

2. **REQUIREMENTS.md left untouched.** It contains a prose example at line 31 using `~/go`. AGENTS.md hard rules: "`REQUIREMENTS.md` is immutable without the user's explicit approval." This plan does not ask for that approval, so the example remains stale. Marked in the plan's Follow-ups section as needing the operator's separate go-ahead.

## Related

- Plan: `.agents/docs/plans/2026-09-04-ui-git-improvements-v1.22.0.md` (Phase 1 complete, phases 2–6 pending)
- Review round 1: `.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements.md` (five findings accepted; all addressed)
- Review round 2: `.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements-round2.md` (six findings, all for later phases)
- Review round 3: `.agents/docs/reviews/2026-09-04-coldread-ui-git-improvements-round3.md` (cold-read of Phase 1 isolation and the skel guard)

## Requirements

n/a — this phase does not touch any `REQUIREMENTS.md` items. The later phases (appearance store, explorer, git) will map to R13 (git substrate) and browser-based preferences (no R items today).
