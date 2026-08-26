# 2026-08-26 — Simplification: conversations are login shells (ADR-014)

Operator decision. Agent picker removed from the New Project dialog and the
project store (agent now optional); every conversation pane is `bash -l` —
both agents preinstalled, launched by hand. paneCommand/session-pinning
removed from the bridge (recoverable from git history, see ADR-014).
e2e updated (rail-scoped prompt check — the any-terminal check raced now
that the center pane is also a shell). Verified: integration 23/23, e2e
18/18, unit green.
