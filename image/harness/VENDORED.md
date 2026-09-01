# Vendored harness snapshot

This is a snapshot of `devshell_prompt/harness` (the agent harness for
`claude` and `agy`), vendored into the runtime image so every project gets
the SAME defaults from the base image (R5/ADR-030) — no shared home, no
host-side deploy dependency.

Synced 2026-09-01 from upstream commit `6132b97` ("harness: rehaul the agent
spine, vendor skills, fix shell env"). Contents: the Global Agent Constitution
(`shared/AGENTS.md`), 28 skills, 6 subagents (changelog, documentation,
explorer, reviewer, security, verifier) and the two shell gates.

The image entrypoint runs `setup.sh` into each project home write-once
(marker `~/.styrman/.harness-version`, keyed on `/styrman/VERSION`). It
deploys to `~/.claude`, `~/.gemini` AND `~/.agents/bin/` — the last holds the
vendor-neutral shell gates (`resume-context.sh`, `trail-check.sh`) that the
Claude `SessionStart`/`Stop` hooks and the Antigravity skill steps call.

Re-running setup is also what RETIRES dropped content in an already-seeded
home: `claude/setup.sh` explicitly removes retired agents (e.g. `testing.md`)
and skills sync with `rsync --delete`. So a home's harness only ever changes
on a version bump.

## How to refresh (the exact command)

```bash
# from the repo root — note the ANCHORED exclude: without it the --delete
# wipes THIS file, and an unanchored `--exclude VENDORED.md` would also skip
# upstream's unrelated shared/skills/VENDORED.md.
rsync -a --delete --exclude /VENDORED.md \
  ~/workspace/devshell_prompt/harness/ image/harness/
diff -rq image/harness ~/workspace/devshell_prompt/harness   # → only VENDORED.md
```

Then update this file's sync date/commit, bump `STYRMAN_VERSION`, rebuild the
image, and redeploy — the version bump is the ONLY delivery path (ADR-046).
Verification steps: `.agents/docs/testing/styrman/README.md`, Tier 5.

Source of truth remains `devshell_prompt/harness`; do not edit here.
