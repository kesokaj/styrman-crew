# 2026-08-27 — Frontend correctness & UX (Batch E)

Fourth remediation batch — the `web/` findings.

- **F3 (ADR-023)** — clipboard history moved from `localStorage` to
  `sessionStorage` + 1-hour TTL, lazy-initialized. Copy-on-select could
  otherwise persist OAuth codes/tokens indefinitely (R10).
- **F6** — `usePolled` now skips overlapping requests (slow VM boots stacked
  polls), drops stale/out-of-order responses (generation guard), and ignores
  results after unmount; `api.request` has a 20 s `AbortSignal.timeout` so a
  wedged server surfaces an error instead of an infinite spinner.
- **F5** — terminal resize is debounced (~120 ms) and skips nonsense sizes;
  dragging the window no longer floods the microVM with pane-resize/redraws.
- **F4** — the Git tab renders the diff error instead of a perpetual
  "Loading…" when a project is stopped or git fails.
- **F2** — the file editor confirms before discarding unsaved edits on
  backdrop/✕ click.
- **F7** — `SetupBanner.markDone` routed through `tryUI` (was an unhandled
  rejection).
- **F8** — link-bar chips have URL-specific `aria-label`s, the "Copied" toast
  is a `role="status"`/`aria-live` region.
- **F10 / F1** — e2e asserts the agent-count sentinel (`agents=2`) instead of
  the echoed command (was a no-op that passed with no agents); a `pageerror`/
  console-error listener fails the suite on genuine runtime errors (recoverable
  React hydration warnings are filtered + TODO'd); by-name self-poisoning was
  already moot under the self-hosting harness.

Tests: `tsc` clean; integration 56/56; e2e 33/33 (new agent-output + no-error
checks); prod redeployed. Known-benign SPA hydration mismatch tracked in
TODO.md.
