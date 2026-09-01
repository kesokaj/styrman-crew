---
name: project-styrman-docs-structure
description: Where styrman-crew's docs live and how the guide is organized (docs root, style, ownership)
metadata:
  type: project
---

Docs root is `.agents/docs/` (not a bare `docs/`) — confirmed present at repo root
`/home/kesokaj/workspace/styrman-crew/.agents/docs/`. Working directory for this
project is `web/`, a subdir of the main repo, but the docs tree is at the repo root,
not under `web/`.

The single "how it works" doc for the whole platform is
`.agents/docs/specs/styrman/guide.md` — a dense, numbered-section reference (§1 What
styrman is, §2 Architecture, §3 Install, §4 Operating it, §5 Troubleshooting, §6
Development, §7 Roadmap). Frontend/UI behavior (Files panel, editor pane, terminal
header, etc.) lives inline in §1 idea 4 ("The side panel is project context") as
dense prose bullets, not a separate frontend doc. Detail specs referenced from the
top: `api.md`, `terminal-protocol.md`, `runtime-image.md`, `deploy.md` (all under
`.agents/docs/specs/styrman/`).

Root `README.md` is thin and does NOT describe the Files/editor panel at
implementation detail — only the guide does. Check README with grep before assuming
it needs a matching update for UI-level changes; often it doesn't.

Style: extremely dense, single-paragraph-per-idea prose bullets citing ADR/PR/version
numbers and dates inline (e.g. "(v1.7.2)", "(2026-09-01)"), not separate subsections
per feature. New behavior gets folded into the existing relevant bullet rather than
appended as a new one, matching voice/density.

**Why:** this is the only place a zero-context agent reconstructs current UI/backend
behavior from — README stays a thin quick-start, guide.md is the living spec.
**How to apply:** for frontend behavior changes (editor, file explorer, terminal
header, etc.), edit `.agents/docs/specs/styrman/guide.md` §1 idea 4 in place; only
touch README.md if grep confirms it already documents that surface at that detail
level.
