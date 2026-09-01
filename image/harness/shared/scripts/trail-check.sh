#!/bin/bash
# Definition-of-Done trail gate — shared by BOTH harnesses:
#   - Claude Code:      Stop hook (default mode; JSON on stdin). If source changed
#                       but no changes/ entry dated today exists, emits
#                       {"decision":"block","reason":...} so the agent dispatches
#                       the owed DoD agents. Always exits 0 — a hook must never
#                       break the session; blocking happens only via the JSON.
#   - Antigravity CLI / /preflight:  `trail-check.sh --check` — human-readable
#                       trail-freshness report; exit 1 if stale, 0 if fresh.
# Activates only in projects whose docs root has a changes/ directory — the
# harness opt-in marker. No network. Tolerates repos with zero commits.
# When in doubt, does NOT block.

today=$(date +%F)

# Portable mtime (epoch seconds): GNU stat first (-c fails cleanly on BSD),
# then BSD stat (on GNU, -f would "succeed" with filesystem info instead).
mtime_of() {
  stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || echo 0
}

# --- docs root: .agents/docs if it exists, else docs/, else .agents/docs ----
# Sets globals: docs (absolute), docs_rel (relative to $root, for path filtering).
resolve_docs_root() {
  if [ -d "$root/.agents/docs" ]; then
    docs="$root/.agents/docs"
  elif [ -d "$root/docs" ]; then
    docs="$root/docs"
  else
    docs="$root/.agents/docs"
  fi
  docs_rel=${docs#"$root"/}
}

# Source file the DoD rows fire on? Pure-docs paths (*.md, the docs root,
# .agents/) never trigger the gate.
is_source_path() {
  case "$1" in
    *.md) return 1 ;;
    .agents/*) return 1 ;;
    "$docs_rel"/*) return 1 ;;
    *.go|*.ts|*.tsx|*.js|*.sh|*.py|*.tf|*.yaml|*.yml|*.json|*.toml|*.sql) return 0 ;;
  esac
  # Extensionless source files match on basename (Dockerfile-only diffs are
  # exactly the ones the verifier row's image builds exist for).
  case "${1##*/}" in
    Dockerfile*|Containerfile*|Makefile|makefile|Justfile|justfile|go.mod|go.sum|*.mk) return 0 ;;
  esac
  return 1
}

# Source files modified, staged, or brand-new right now. Untracked (??) files
# count too — a session that only creates new files owes the same DoD rows
# (-uall lists files inside untracked directories individually).
dirty_source() {
  git -C "$root" status --porcelain -uall 2>/dev/null | while IFS= read -r line; do
    p=${line:3}
    case "$p" in *" -> "*) p=${p##* -> } ;; esac
    p=${p%\"}; p=${p#\"}
    is_source_path "$p" && printf '%s\n' "$p"
  done
}

# Source files committed today (committed-but-unrecorded counts as changed).
head_source_today() {
  [ "$(git -C "$root" log -1 --format=%cd --date=short 2>/dev/null)" = "$today" ] || return 1
  # diff-tree lists names for merge commits too (plain `git show` suppresses them)
  git -C "$root" diff-tree -r --no-commit-id --name-only HEAD 2>/dev/null | while IFS= read -r p; do
    [ -n "$p" ] || continue
    is_source_path "$p" && printf '%s\n' "$p"
  done
}

# Is there an artifact of the given class (changes|reviews|testing) dated
# today under the docs root? $2 is the filename infix after the date — the
# DoD rows share directories (reviewer and security both write to reviews/),
# so each row matches its own infix, never the whole directory.
today_artifact_exists() {
  for f in "$docs/$1/$today$2"*.md; do
    [ -e "$f" ] && return 0
  done
  return 1
}

# ============================ Stop-hook mode =================================
hook_mode() {
  input=""
  [ ! -t 0 ] && input=$(cat 2>/dev/null || true)

  # Loop guard FIRST, on the raw text, so it holds even when jq is not on
  # PATH: we already blocked once this stop — never block twice.
  if printf '%s' "$input" | grep -Eq '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
    exit 0
  fi

  dir="$PWD"
  if [ -n "$input" ] && command -v jq >/dev/null 2>&1; then
    # Same loop guard, structured form — kept as a second line of defense.
    if [ "$(printf '%s' "$input" | jq -r '.stop_hook_active // false' 2>/dev/null)" = "true" ]; then
      exit 0
    fi
    hook_cwd=$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)
    [ -n "$hook_cwd" ] && [ -d "$hook_cwd" ] && dir="$hook_cwd"
  fi

  root=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)
  [ -n "$root" ] || exit 0
  resolve_docs_root
  # Harness opt-in marker: the gate binds only projects that keep a trail —
  # no changes/ directory under the docs root, no DoD contract here.
  [ -d "$docs/changes" ] || exit 0

  # Cheapest exit first: all three of today's artifacts settle every DoD row.
  if today_artifact_exists changes - && today_artifact_exists reviews -review- &&
    today_artifact_exists testing -verify-; then
    exit 0
  fi

  changed=$(dirty_source)
  [ -z "$changed" ] && changed=$(head_source_today)
  [ -z "$changed" ] && exit 0

  # One entry per missing artifact class, tagged with the DoD row that owes it.
  missing=""
  today_artifact_exists changes - || missing="$missing changes/$today-<slug>.md (changelog row);"
  today_artifact_exists reviews -review- || missing="$missing reviews/$today-review-<slug>.md (reviewer row);"
  today_artifact_exists testing -verify- || missing="$missing testing/$today-verify-<slug>.md (verifier row);"
  missing=${missing# }
  missing=${missing%;}

  n=$(printf '%s\n' "$changed" | grep -c .)
  sample=$(printf '%s\n' "$changed" | head -n 5 | paste -sd, -)
  reason="Definition of Done check: $n source file(s) changed this session ($sample) but today's trail under $docs_rel/ is incomplete. Missing: $missing. Dispatch the owed agent for each missing row now — pass today's date $today in each dispatch prompt — or tell the user explicitly why they are not needed."

  if command -v jq >/dev/null 2>&1; then
    jq -cn --arg reason "$reason" '{decision:"block", reason:$reason}'
  else
    # No jq: $missing and $today are built above from fixed words plus the
    # date — no quotes or backslashes, so no JSON escaping is needed.
    printf '{"decision":"block","reason":"Definition of Done check: source files changed this session but today'\''s trail is incomplete. Missing: %s. Dispatch the owed agent for each missing row now — pass today'\''s date %s in each dispatch prompt — or tell the user explicitly why they are not needed."}\n' "$missing" "$today"
  fi
  exit 0
}

# ============================== --check mode =================================
check_mode() {
  root=$(git -C "$PWD" rev-parse --show-toplevel 2>/dev/null)
  if [ -z "$root" ]; then
    echo "trail-check: not a git repository: $PWD"
    exit 1
  fi
  resolve_docs_root
  stale=0

  echo "Trail freshness — $root"
  echo "docs root: $docs_rel/"
  if [ ! -d "$docs/changes" ]; then
    # Same opt-in rule as hook mode: no trail means this repo never adopted the
    # harness contract — report it and pass, rather than failing a foreign clone.
    echo "note: no trail exists yet — $docs_rel/changes/ not found (harness opt-in marker absent)"
    echo "RESULT: OK (no harness trail to check; run /init-project to adopt the contract)"
    exit 0
  fi

  head_date=$(git -C "$root" log -1 --format=%cd --date=short 2>/dev/null)
  # Globs expand sorted, so the last date-prefixed match is the newest entry.
  change_date=""
  for f in "$docs/changes"/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-*.md; do
    [ -e "$f" ] || continue
    b=${f##*/}
    change_date=${b:0:10}
  done

  # Newest changes/ entry must cover HEAD (the /preflight trail-freshness rule).
  if [ -z "$head_date" ]; then
    echo "commits: none yet — HEAD coverage vacuously OK"
  elif [ -z "$change_date" ]; then
    echo "STALE: repo has commits (HEAD $head_date) but no dated changes/ entries"
    stale=1
  elif [[ "$head_date" > "$change_date" ]]; then
    echo "STALE: newest changes/ entry ($change_date) older than HEAD commit ($head_date)"
    stale=1
  else
    echo "OK: newest changes/ entry ($change_date) covers HEAD ($head_date)"
  fi

  # Today's entry — required only when uncommitted source changes exist.
  if today_artifact_exists changes -; then
    echo "OK: changelog entry for today ($today) present"
  else
    changed=$(dirty_source)
    if [ -n "$changed" ]; then
      echo "STALE: uncommitted source changes but no changelog entry dated $today"
      stale=1
    else
      echo "note: no changelog entry dated today ($today); no uncommitted source changes either"
    fi
  fi

  # Filename date sanity: no future dates, no wrong-decade, no malformed dates.
  for f in "$docs/changes"/*.md; do
    [ -e "$f" ] || continue
    b=${f##*/}
    case "$b" in
      [0-9]*)
        if ! printf '%s' "$b" | grep -qE '^20[2-9][0-9]-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])-'; then
          echo "STALE: implausible date in changes/ filename: $b"
          stale=1
        elif [[ "${b:0:10}" > "$today" ]]; then
          echo "STALE: future-dated changes/ entry: $b (today is $today)"
          stale=1
        fi
        ;;
      *)
        echo "note: undated file in changes/: $b"
        ;;
    esac
  done

  # ROADMAP/TODO mtime vs HEAD — informational only, never fails the gate.
  head_epoch=$(git -C "$root" log -1 --format=%ct 2>/dev/null)
  for f in ROADMAP.md TODO.md; do
    if [ ! -f "$root/$f" ]; then
      echo "note: $f missing at repo root"
    elif [ -n "$head_epoch" ] && [ "$(mtime_of "$root/$f")" -lt "$head_epoch" ]; then
      echo "note: $f not touched since HEAD commit"
    else
      echo "OK: $f current relative to HEAD"
    fi
  done

  if [ "$stale" -eq 1 ]; then
    echo "RESULT: STALE — the documentation trail does not cover the code."
    exit 1
  fi
  echo "RESULT: FRESH"
  exit 0
}

case "${1:-}" in
  --check) check_mode ;;
  *) hook_mode ;;
esac
