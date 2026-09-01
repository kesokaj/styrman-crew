#!/bin/bash
# Resume Protocol context injector — the deterministic half of the Resume Protocol.
# Shared by BOTH harnesses:
#   - Claude Code:      SessionStart hook (JSON on stdin; stdout is injected into
#                       model context — every line printed here reaches the agent)
#   - Antigravity CLI:  skill step / manual invocation (no stdin, or /dev/null)
# Prints AT MOST 4 short lines orienting the agent in an existing harness project;
# prints nothing outside one. Never blocks the session — always exits 0. No network.

# --- resolve project dir: hook JSON .cwd, else $PWD -------------------------
# Stdin may be absent (standalone/tty), empty, or garbage — tolerate all three.
dir="$PWD"
if [ ! -t 0 ]; then
  input=$(cat 2>/dev/null || true)
  if [ -n "$input" ] && command -v jq >/dev/null 2>&1; then
    hook_cwd=$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)
    [ -n "$hook_cwd" ] && [ -d "$hook_cwd" ] && dir="$hook_cwd"
  fi
fi

# Only speak inside a git repo.
root=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)
[ -n "$root" ] || exit 0

# --- docs root: .agents/docs if it exists, else docs/, else .agents/docs ----
if [ -d "$root/.agents/docs" ]; then
  docs="$root/.agents/docs"
elif [ -d "$root/docs" ]; then
  docs="$root/docs"
else
  docs="$root/.agents/docs"
fi

# Harness opt-in marker: only projects that keep a trail (a changes/ directory
# under the docs root) get resume context — print nothing elsewhere.
[ -d "$docs/changes" ] || exit 0

# (1) The reading order — resume-context prints exactly this, per AGENTS.md.
echo "Resume Protocol: AGENTS.md -> REQUIREMENTS.md -> ROADMAP.md -> TODO.md -> newest handoff -> DECISIONS.md -> 3 newest changes/"

# (2) Latest handoff — newest by FILENAME date, like the changes/ scan below.
# mtime lies after clone/checkout/rsync (everything gets the same timestamp);
# handoff filenames are date-prefixed by contract, so the name is the truth.
hf=""
for f in "$docs/handoffs"/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-*.md; do
  [ -f "$f" ] || continue
  hf="$f"
done
if [ -n "$hf" ]; then
  hb=${hf##*/}
  hf_epoch=$(date -d "${hb:0:10}" +%s 2>/dev/null || date -j -f %Y-%m-%d "${hb:0:10}" +%s 2>/dev/null || echo 0)
  if [ "$hf_epoch" -gt 0 ]; then
    age=$(( ($(date +%s) - hf_epoch) / 86400 ))
    echo "Latest handoff: ${hf#"$root"/} (${age}d old)"
  else
    echo "Latest handoff: ${hf#"$root"/}"
  fi
else
  echo "no handoffs recorded"
fi

# (3) Newest changes/ entry date (from filename) vs last commit date.
# Globs expand sorted, so the last date-prefixed match is the newest entry.
change_date=""
for f in "$docs/changes"/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-*.md; do
  [ -e "$f" ] || continue
  b=${f##*/}
  change_date=${b:0:10}
done
commit_date=$(git -C "$root" log -1 --format=%cd --date=short 2>/dev/null)
if [ -n "$commit_date" ]; then
  if [ -z "$change_date" ] || [[ "$commit_date" > "$change_date" ]]; then
    echo "WARNING: trail behind code (last change entry ${change_date:-none}, last commit $commit_date)"
  fi
fi

# (4) Missing or empty root files the Resume Protocol expects.
missing=""
for f in REQUIREMENTS.md ROADMAP.md TODO.md; do
  [ -s "$root/$f" ] || missing="$missing $f"
done
[ -n "$missing" ] && echo "Missing or empty:$missing"

exit 0
