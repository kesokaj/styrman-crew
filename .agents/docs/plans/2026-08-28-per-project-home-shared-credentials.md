# Plan — per-project home, shared live credentials (no re-auth)

Date: 2026-08-28 · Status: PLAN (operator: "plan then do" — do NOT implement
until this is approved). Supersedes the narrower "per-project tool cache"
idea discussed the same day.

## The requirement (operator's words)

- Projects must be **truly individual**. Today the home is shared across a
  user's projects, so every tool that writes to `$HOME` (Go modules, npm
  cache + global installs, pip, terraform plugins, `~/.cache`, shell
  history, app state) is silently shared — "even tho they are separate,
  they are not."
- **Not sharing caches is fine** — disk cost accepted.
- **Credentials are the key**: they must be shared across all projects AND
  **kept updated** (a token refresh in one project must not leave the others
  stale). "I don't want to re-auth all the time."
- Operator is OK with **each project having its own home**, as long as
  credentials are shared and stay current.

## The principle

Split what lives in `$HOME` into two classes:

- **Identity** — authenticates you; the reason a shared store exists.
  Must be shared and LIVE (single source of truth, written through).
- **Working state** — caches, modules, build output, global installs,
  history, app state. Regenerable, and what "individual projects" means.
  Must be per-project.

## Two designs considered

### Option A — shared home, redirect caches to a per-project volume
Keep one shared home; point tool cache/state env vars (GOPATH, npm cache +
prefix, pip, terraform plugin cache, XDG_CACHE_HOME) at a second per-project
mount. Credentials are shared for free (they're just in the shared home).

- **Pro:** small, low-risk, no home migration; delivers no-re-auth for free;
  moves the big space hogs and the actual leak (caches, `-g` installs).
- **Con:** carve-out model — isolation is only as good as the list of paths
  we redirect; anything we forget (shell history, `~/.local/share` app
  state, a tool's bespoke `~/.foo` dir) stays shared. Never feels "fully
  individual."

### Option B — per-project home, shared credentials mounted in (RECOMMENDED)
Each project gets its **own** home volume (individual by construction — every
`$HOME` write is per-project automatically). A single **credentials** volume
is the source of truth for identity, wired into every project's home so all
projects see the same live files; a refresh in any project writes back to the
one store.

- **Pro:** matches the operator's model exactly — individual by default, no
  ongoing "is this path a cache?" curation; new tools are isolated for free.
- **Con:** more machinery (credential wiring), a real one-time migration of
  the existing shared home, and the credential inventory must be exactly
  right (miss one → re-auth; over-share → leak). Higher risk, hence this
  plan.

**Recommendation:** Option B is what actually delivers "truly individual."
Its risk is entirely in (1) the credential inventory and (2) the migration —
both addressed below. If we want to de-risk, we can ship A first (immediate
no-re-auth + cache isolation) and layer B later; but B is the destination.

## Option B — mechanism

### Storage layout
- `StateDir/credentials/<user>/` — the ONE credential store (source of truth).
- `StateDir/homes/<user>/<uuid>/` — per-project home (NEW: keyed by project
  UUID, was one shared `homes/<user>`).
- `StateDir/projects/<user>/<uuid>/` — workspace (ADR-027, unchanged).

Each project VM mounts: its per-project home at `$HOME`, its workspace at
`~/projects/<dir>`, and the credentials store at a fixed path
(`$HOME/.styrman-cred`, read-write).

### Wiring credentials in (two clean techniques, avoid fragile file binds)
1. **Config files via env var** — no file mounts, no rename-on-save breakage:
   - `GIT_CONFIG_GLOBAL=$HOME/.styrman-cred/gitconfig`
   - `NPM_CONFIG_USERCONFIG=$HOME/.styrman-cred/npmrc`
   - (curl/`~/.netrc`: no clean env; symlink or `--netrc-file` in wrappers —
     open question below.)
2. **Credential DIRECTORIES via symlink into the mounted store** (dir
   symlinks are stable across rename-on-save, which happens on files inside):
   `~/.ssh`, `~/.config/gh`, `~/.config/gcloud`, `~/.claude`, `~/.gemini`,
   `~/.docker`, `~/.local/share/keyrings` → `$HOME/.styrman-cred/<name>`.

### Credential inventory (SHARE — everything else is per-project)
| Path | Kind | Wire via |
|---|---|---|
| `~/.ssh/` | git-over-ssh keys, known_hosts | dir symlink |
| `~/.config/gh/` | GitHub CLI token | dir symlink |
| `~/.config/gcloud/` | gcloud ADC + config | dir symlink |
| `~/.claude/` | Claude Code auth (+ settings/history — see Q) | dir symlink |
| `~/.gemini/` | agy / Antigravity auth | dir symlink |
| `~/.docker/` | registry auth (config.json) | dir symlink |
| `~/.local/share/keyrings/` | gnome-keyring (agy Secret Service, S4) | dir symlink |
| `~/.gitconfig` | identity + config | env (GIT_CONFIG_GLOBAL) |
| `~/.npmrc` | registry auth token | env (NPM_CONFIG_USERCONFIG) |
| `~/.netrc` | curl/git private creds | open question |
| `~/.bash_secrets` | operator-exported secrets (R10) | file symlink or source-from-cred |

Per-project (because the home itself is per-project): `~/go`, `~/.npm`,
`~/.npm-global`, `~/.cache`, `~/.local` (except keyrings), shell history,
build artifacts, everything not on the list above.

### First-run + refresh (the "keep updated" requirement)
- The **setup VM** (styrman-setup) writes credentials straight into the
  credentials store (its home's credential paths ARE the store), so
  onboarding populates the single source.
- Because credential paths are symlinks/env-pointers to the ONE store, a
  token refresh in any project writes through to the store → every other
  project sees it immediately. No copy, no sync job, never stale. This is
  the crux of "shared and kept updated."

### New-project home bootstrap
`ensureHome` seeds each new per-project home from a skeleton: dotfiles
(internal/dotfiles), the standard dirs, and the credential symlinks/env.
Dotfiles stay per-project (config, cheap to regenerate); credentials are
always the shared store.

### Migration (existing shared home → split)
One-time, boot-time, guarded like ADR-027:
1. Move the credential paths out of the existing `homes/<user>` into
   `credentials/<user>/` (the new store).
2. Turn the existing `homes/<user>` into the first project's per-project
   home (or archive it and re-seed), with credential symlinks pointing at
   the new store.
3. Subsequent projects get fresh per-project homes.
Must run with project VMs stopped (like the ADR-027 sweep), be idempotent,
and guard against half-done state (no silent data loss — the ADR-027
review lessons apply directly).

## Risks / open questions (resolve before building)
1. **`~/.claude` / `~/.gemini` granularity** — these dirs hold auth AND
   settings/MCP/project history. Sharing the whole dir shares history across
   projects (maybe fine, maybe not). Option: share only the credential file
   within, keep the rest per-project. NEEDS A DECISION.
2. **`~/.netrc`** — no clean env-var redirect; file symlink risks
   rename-on-save. Acceptable to symlink? Or skip (rare)?
3. **`~/.bash_secrets`** — share (so exported secrets are everywhere) or
   keep per-project? Currently R10 per-home.
4. **Keyring** — the /run plumbing (profile-keyring.sh, S4) assumes the
   keyring dir in the home; verify it works when that dir is a symlink to
   the shared store, and that concurrent projects unlocking the same keyring
   don't corrupt it (gnome-keyring locking across VMs — needs a real test).
5. **Concurrent writes to one credential store** from multiple running
   projects (e.g., two gcloud token refreshes at once) — last-writer-wins on
   a single file is usually fine, but verify gh/gcloud/claude tolerate it.
6. **Migration safety** — the existing home is the operator's only home; the
   split must be reversible/guarded. Highest-risk step.
7. **REQUIREMENTS R3** already needs a wording update (it still says "one
   shared home"); Option B rewrites R3's model. Fold both into one approval.

## Recommendation to the operator
Adopt **Option B** as the target. Before I build:
- Decide Q1 (`~/.claude`/`~/.gemini`: whole dir vs auth-only) and Q3
  (`~/.bash_secrets` shared?).
- Confirm the credential inventory above is complete (anything you auth to
  that I missed?).
- Confirm you want the full per-project-home migration, or prefer to ship
  Option A first (cache isolation + free no-re-auth) and stage B.
Then I implement behind an ADR, adversarially reviewed, with the migration
guarded and verified on prod that credentials survive and NOTHING re-auths.

## Explicitly NOT doing yet
Nothing is implemented. No mounts changed, no home split, no env vars added.
This document is the deliverable for this round.
