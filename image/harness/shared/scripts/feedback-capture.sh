#!/bin/bash
# Auto-capture correction signals from user prompts (Claude Code UserPromptSubmit hook).
# Deterministic complement to the `feedback` skill: pattern-matches likely corrections
# and appends them to the global learning-loop log mined by /retro and /distill --from-log.
#
# Silent by design — UserPromptSubmit stdout is injected into model context, so this
# script must never print. Never blocks the turn — always exits 0.

LOG_DIR="$HOME/.agents/feedback"
LOG_FILE="$LOG_DIR/log.jsonl"

command -v jq >/dev/null 2>&1 || exit 0

input=$(cat)
prompt=$(printf '%s' "$input" | jq -r '.prompt // empty' 2>/dev/null)
[ -z "$prompt" ] && exit 0

# System-injected turns are not user corrections: harness notifications and
# reminders arrive as UserPromptSubmit too (XML-wrapped, task-notification,
# system-reminder) and were the bulk of early log noise. Skip them entirely.
case "$prompt" in
  '<'*) exit 0 ;;
esac
# (No marker-word grep: every observed noise entry starts with '<' and the
# case above catches it; a substring match would also drop genuine corrections
# that merely QUOTE the words task-notification/system-reminder.)

# Correction/preference markers. Deliberately loose — false positives are fine
# (/retro reads this log with judgment); missed corrections are the real failure mode.
# 'instead' requires word-boundary context ([[:alnum:]] guards, portable across
# GNU/BSD grep) so it stops matching inside built-in notification prose.
if ! printf '%s' "$prompt" | grep -qiE \
  '^no[,. !]|not what i|i (said|told you)|i asked (for|you)|that.?s (wrong|not right|incorrect)|don.?t do that|stop (doing|that)|undo th|revert th|why did you|you (didn.?t|ignored|missed|forgot)|(^|[^[:alnum:]])instead([^[:alnum:]]|$)'; then
  exit 0
fi

mkdir -p "$LOG_DIR" 2>/dev/null || exit 0
printf '%s' "$input" | jq -c \
  --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{ts: $ts, project: (.cwd // "unknown"), session: (.session_id // ""), source: "auto-hook", prompt: (.prompt | .[0:400])}' \
  >>"$LOG_FILE" 2>/dev/null

exit 0
