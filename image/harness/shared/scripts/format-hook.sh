#!/bin/bash
# Shared auto-format hook for BOTH harnesses:
#   - Claude Code:      PostToolUse hook on Edit|Write            (~/.claude/format-hook.sh)
#   - Antigravity CLI:  PostToolUse hook on write_to_file et al.  (~/.gemini/format-hook.sh,
#                       registered via the harness plugin's hooks.json)
# Reads the hook JSON on stdin, formats the edited file by extension.
# Never blocks the turn — always exits 0.

# Ensure Go workspace bin + Homebrew bin are on PATH (this script is bash and does
# not source ~/.zshenv, so goimports installed at $GOPATH/bin must be added here).
# Covers macOS (Homebrew, ~/.go) and Linux (~/go, /usr/local/go) install layouts.
export PATH="$HOME/.go/bin:$HOME/go/bin:/usr/local/go/bin:${HOMEBREW_PREFIX:-/opt/homebrew}/bin:$PATH"

# Extract the edited file path. The two vendors use different JSON shapes:
#   Claude Code:      .tool_input.file_path (Edit/Write)
#   Antigravity CLI:  .toolCall.args.TargetFile (write_to_file/replace_file_content;
#                     verified against agy-cli 1.1.x PostToolUse payloads)
f=$(jq -r '
  .tool_input.file_path
  // .tool_input.absolute_path
  // .tool_input.path
  // .toolCall.args.TargetFile
  // empty' 2>/dev/null)
[ -z "$f" ] && exit 0
[ -f "$f" ] || exit 0

case "$f" in
  *.go)
    if command -v goimports >/dev/null 2>&1; then
      goimports -w "$f" 2>/dev/null
    else
      gofmt -w "$f" 2>/dev/null
    fi
    ;;
  *.ts|*.tsx|*.js|*.jsx|*.mjs|*.cjs|*.css|*.scss|*.json|*.md|*.yaml|*.yml|*.html)
    # --no-install: only runs if prettier is a project dependency; silent no-op otherwise
    npx --no-install prettier --write "$f" >/dev/null 2>&1
    ;;
esac

exit 0
