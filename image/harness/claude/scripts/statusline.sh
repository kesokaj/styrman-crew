#!/bin/bash
# Claude Code status line: model · git branch · GCP project
# Receives session JSON on stdin.

input=$(cat)

model=$(printf '%s' "$input" | jq -r '.model.display_name // "?"')
cwd=$(printf '%s' "$input" | jq -r '.workspace.current_dir // .cwd // "."')

# Git branch (empty if not a repo)
branch=$(git -C "$cwd" branch --show-current 2>/dev/null)

# Active GCP project — read gcloud config file directly (fast; avoids slow `gcloud` startup)
gcp=""
active=$(cat "$HOME/.config/gcloud/active_config" 2>/dev/null)
if [ -n "$active" ]; then
  gcp=$(grep -m1 '^project' "$HOME/.config/gcloud/configurations/config_${active}" 2>/dev/null | sed 's/^project *= *//')
fi

out="⚡ $model"
[ -n "$branch" ] && out="$out  ⎇ $branch"
[ -n "$gcp" ] && out="$out  ☁ $gcp"
printf '%s' "$out"
