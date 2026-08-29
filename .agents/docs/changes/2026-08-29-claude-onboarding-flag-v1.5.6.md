# claude re-prompted for login in fresh projects — onboarding flag (v1.5.6)

Date: 2026-08-29. Follow-up to the v1.5.5 broker work: with credentials now
flowing to every project, `agy` worked but **interactive `claude` still asked
the operator to sign in** in a new project (jumped straight to the OAuth
authorize URL), even though the token was valid.

## Root cause

The broker syncs `~/.claude/.credentials.json` (the token), and that token is
valid — `claude -p "…"` authenticates and answers fine. But interactive
`claude` also gates on `~/.claude.json`'s `hasCompletedOnboarding`. That file
is per-project and is NOT synced by the broker, so a fresh per-project home has
`hasCompletedOnboarding` unset while the operator's shared home (where setup ran
onboarding) has it `true`. With the flag unset, the CLI re-runs first-run
onboarding and prompts for login despite having working credentials. `claude -p`
skips onboarding, which is why it worked and the interactive path did not.

## Fix

`image/configs/styrman-credentials.sh` (the pull-on-login materializer) now, on
every login shell where `~/.claude/.credentials.json` is present, sets
`hasCompletedOnboarding: true` in `~/.claude.json` — jq-merged so the file's
other per-project state (oauthAccount, projects, history) is preserved; a
minimal file is created if none exists. A materialized token means the user has
already signed in and onboarded elsewhere, so the flag is correct to set. Verified
the merge preserves all existing keys (24 → 25, oauthAccount intact) and that
interactive `claude` no longer emits the `oauth/authorize` URL once set.

## Verification (live)

Set the flag in the running "test" project → interactive `claude` no longer
shows the login URL (confirmed the `oauth/authorize` line is gone; `claude -p`
already worked). Rebuilt `styrman-runtime:dev` (1.5.6) with the script fix; the
server creates new projects from that image, so every new project gets a
ready-to-use claude with no login re-prompt. The existing "test" project was
patched in place (its VM still runs the pre-fix image until recreated).

## Files changed

- `image/configs/styrman-credentials.sh` — set `hasCompletedOnboarding: true`
  when the claude token is present (runtime-image rebuild).

## Note

This is the general shape of a broader question — the broker syncs credential
tokens but not the small amount of per-tool *client state* that makes an
interactive CLI treat itself as set up. claude's onboarding flag is the one that
bit here; watch for similar per-tool state if other interactive tools are added.
