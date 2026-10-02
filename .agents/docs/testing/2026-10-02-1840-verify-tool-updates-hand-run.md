---
owner: main
date: 2026-10-02
---
# Verification — in-project updates of agy and claude, hand run   (2026-10-02)

Range reviewed: v1.39.0..4dd51bf

## Status: PASS

Scope: the one tier of `plans/2026-10-02-tool-updates-freeze-harness-v2.md` that only the
main agent can run (its "Hand run", D14): the updaters' real downloads, inside a REAL booted
project. Not in a suite because each run downloads about 220 MB and its result moves with
the publishers. Every other tier is the verifier's.

How it was run: `bash .agents/scratch/hand-update-run.sh` at `4dd51bf` — a server built from
the tree, started on `127.0.0.1:18097` with a scratch state dir under `/var/tmp`, image
`styrman-runtime:test` (built 2026-10-02 18:25 from the tree's `image/`, which is unchanged
between `39ccf89` and `4dd51bf`); one project created through the API; every command below
run with `docker exec … runuser -u <project user> -- env HOME=… bash -lic '…'`, an interactive
login shell as the project user. To give the updaters something to do, an old agy (1.2.2,
taken from `styrman-runtime:1.38.0`) was copied over `/opt/agy/agy` and
`npm install -g @anthropic-ai/claude-code@2.1.270` was run as the user. Exit status of the
script: 0. Output kept in `.agents/scratch/hand-update-run-final.log` (not committed).

## Rows
| # | Criterion | Observed | Result |
|---|---|---|---|
| 1 | The project boots in a Kata microVM and the harness install finishes after ready | `runtime: mvm, kernel 6.18.28`; the `project … ready` line, then `agent harness deployed (version test-1790958295, sh-agent v2.3.2; browser MCP registered for claude and agy)` | 1 of 1 |
| 2 | At boot `/opt/agy` is the project user's and the image's binary is root's | `drwxr-xr-x 1 kesokaj kesokaj … /opt/agy`, `-rwxr-xr-x 1 root root 220655824 … /opt/agy/agy` | 1 of 1 |
| 3 | `agy update` replaces an old agy without the directory error | `agy --version` 1.2.2 → `Found new version 1.2.14 … Update successful! Please restart agy.`, exit 0 → `agy --version` 1.2.14 | 1 of 1 |
| 4 | Claude reports its updater usable from the user's npm prefix | with 2.1.270 at `~/.npm-global/bin/claude`: `claude doctor` → `Running: npm-global (2.1.270)`, `Auto-updates: enabled`, `No installation issues found.` | 1 of 1 |
| 5 | `claude update` updates in the project | `New version available: 2.1.287 (current: 2.1.270)` → `Successfully updated from 2.1.270 to version 2.1.287`, exit 0 | 1 of 1 |
| 6 | A new login shell runs the updated Claude | `which claude` → `~/.npm-global/bin/claude`; `claude --version` → `2.1.287 (Claude Code)`; `claude doctor` → `No installation issues found.` | 1 of 1 |
| 7 | The harness the project got is v2.3.2 | 36 entries under `~/.claude/skills`; `~/.local/bin/sh-agent version` → `sh-agent v2.3.2` | 1 of 1 |

7 of 7 rows passed.

## Not examined
- Row 3 replaced a binary owned by the PROJECT USER, not by root: `docker cp` wrote the
  planted file as the invoking uid and a `chown root` inside the guest failed (`File
  exists`). The root-owned case — the one the image ships — was observed once, in a probe of
  the 1.38.0 image with a user-owned directory holding a root-owned binary (1.2.2 → 1.2.14),
  not in this run.
- The unattended, in-TUI auto-update of Claude (only `claude update` and `claude doctor`
  were run), and agy's behaviour when an update is interrupted.
- Any host other than this one.
