---
owner: reviewer
date: 2026-09-13
---
# Review — audit scan: web/src/components (batch B) + web/src/routes + e2e harness (2026-09-13)

Range reviewed: working tree at commit e56e775 (clean). Scope named by the dispatch:
`web/src/components/{ui,SidePanelPorts,TerminalList,EditorPane,icons,NewProjectDialog,
SidePanelHistory,SidePanelEnv,LocalNetwork,DiffPane,TerminalStatusBar,
DeleteProjectDialog,SidePanel,SetupBanner,NotificationBell,SidePanelClips,DiffView,
LanBadge}.tsx`, `web/src/routes/*.tsx` (`index.tsx`, `p.$project.$term.tsx`,
`__root.tsx`, `settings.tsx`, `setup.tsx`), `web/e2e/run.mjs`, `web/e2e/run.sh`,
`web/scripts/normalize-shell.mjs`. This is not a diff review (no in-flight change set
against these files at HEAD) — every listed file was read in full and judged against
AGENTS.md, REQUIREMENTS.md (R1–R13), the three newest `.agents/docs/changes/` entries
(v1.29.0 release, browser-review-fixes, v1.28.0 release), the specs the code names
(ADR-065/067/068 as cited inline), and `.agents/docs/bugs/INDEX.md` (BUG-001…BUG-053).

## Verdict: APPROVE WITH NITS

Nothing broken was found in this batch: the components are well-scoped, the recent
BUG-021/024/033/041/043/044/045 fixes (test ordering, EXIT-trap exit status, verify-
bare, per-terminal view latch, screencast visibility gating) are all correctly present
and consistent with their bug records, and the e2e harness (`run.sh`/`run.mjs`) is
careful about the failure modes its own history records. The only two findings are
both dead-code cleanup items with no behavioral impact.

## Findings

- **[low / confidence: high]** `web/src/components/icons.tsx:159` — `ExternalLinkIcon`
  is exported but has zero importers anywhere under `web/src` (confirmed by grep across
  the whole tree, not just the reviewed file list). Fix: delete the export (review-
  standards criterion 1 — unreferenced exports must go, not be commented or kept "for
  later").

- **[low / confidence: high]** `web/src/routes/settings.tsx:963-969` — an orphaned
  JSDoc block ("The operator's view of the host half of per-project LAN access
  (ADR-065): whether root's firewall script is installed and current, when it last
  ran...") sits immediately above `RuntimeEditor`, a component that has nothing to do
  with LAN/firewall status. `git log -S` traces the comment to commit `03fedc8a`,
  predating ADR-067 (2026-09-09), which deleted the root-mediated allowlist design
  entirely ("no allowlist, no per-prefix configuration and no root-side per-project
  state" — AGENTS.md, ADR-067). No such admin view exists anywhere in this file or in
  `LocalNetwork.tsx`/`LanBadge.tsx` (both correctly describe the current
  interface-name model). The stale comment misdescribes a feature that was removed,
  which is exactly the kind of drift the six-criteria dead-code rule targets. Fix:
  delete the orphaned comment block (lines 963-969); `RuntimeEditor`'s own doc comment
  two lines above it already describes it correctly.

No other findings met the bar (concrete failure mode, security, or a stated
requirement/rule violation). Specifically checked and clean:
- `LocalNetwork.tsx`/`LanBadge.tsx`/`SidePanelPorts.tsx`'s network block: correctly
  reflect ADR-067 (no host-half polling, `canEnable` read from the server, BUG-019/020
  fixes present, `preferOverride` pattern applied consistently).
- `p.$project.$term.tsx`'s `useViewLatch`: correctly keyed by `${project}/${term}`
  (BUG-043's exact fix — the `ref.current.key !== key` reset), and `BrowserPane` is
  passed an explicit `visible={pane.view === "browser"}` prop (BUG-044's fix), matching
  the "switching to the terminal and back resumes the screencast" e2e check
  (`run.mjs:1309`).
- `web/e2e/run.sh`'s cleanup trap: bare `docker inspect`/`network rm` guarded with
  `|| continue`/`|| true` throughout (BUG-024's exact class of failure — a failing
  command inside an EXIT trap under `set -e` aborting the trap) and the final
  `status=$?; ... exit "$status"` avoids letting the trap decide the exit code.
  `run.mjs`'s `check()`/`finally` block similarly computes `process.exit(failed === 0 ?
  0 : 1)` directly, not through a piped/filtered chain.
- `run.mjs`'s ordering: the LAN-enable/rebuild check (`run.mjs:1213`) and the Browser
  view checks that depend on the rebuilt container both run after all read-only checks
  and before project deletion — the ordering BUG-021 named.
- `normalize-shell.mjs`: wired into `web/package.json`'s `build` script (verified), not
  dead; its byte-exact latin1 round-trip matches the BUG-037 prevention rule (no raw
  control byte, single confirmed match before rewriting).
- `DiffView.tsx`'s `reveal()`: replaces bidi/control characters with a visible
  `<U+XXXX>` token and is rendered through JSX text interpolation (auto-escaped), so no
  XSS vector opens even though the pane's stated purpose is showing untrusted diff
  content.
- No hand-rolled auth/crypto/session code anywhere in this batch (`useModalA11y`,
  credential viewers etc. are UI-only; the broker/session logic lives server-side, out
  of scope here).

## Criteria

1. **Dead code** — FAIL (two low-severity items above: one unused export, one orphaned
   doc comment for a removed feature). Nothing else in this batch was unreferenced.
2. **Flow tracing** — pass. Traced `LocalNetwork`→`api.setProjectLan`→toast/refresh,
   `EditorPane`/`DiffPane` pane-store wiring, `useViewLatch`→`BrowserPane`/`DiffPane`
   mount gating, and the e2e harness's server-start→health-check→Playwright→cleanup
   chain end to end; no dangling caller, no promise left unhandled (`tryUI` wraps every
   fire-and-forget action; `.catch(() => {})` only appears on genuinely best-effort
   teardown paths that are documented as such).
3. **Standards conformance** — pass. Consistent with the `frontend`/`typescript`
   skills: no floating promises, errors are typed `unknown`/`Error` and narrowed before
   `.message` access, modal a11y contract (`useModalA11y`) reused rather than
   reimplemented per dialog.
4. **Directory layout** — pass. Everything sits under `web/src/components`,
   `web/src/routes`, `web/e2e`, `web/scripts` as the project's own layout dictates; no
   stray files.
5. **Simplicity (DIY-except-security)** — pass. Icons are inline SVG with a stated
   justification (avoid a dependency); no new dependencies introduced; no
   auth/crypto/session code in this batch.
6. **Decoupling** — pass. Each side-panel tab is its own file per `SidePanel.tsx`'s own
   comment ("Each tab's implementation lives in its own SidePanel*.tsx file"); `DiffView`
   is correctly shared rather than duplicated between `DiffPane` and the (out-of-scope)
   `SidePanelGit`, exactly as its own comment states.

## Not examined

- `BrowserPane.tsx`, `CodeEditor.tsx`, `FileEditor.tsx`, `FileExplorer.tsx`,
  `Sidebar.tsx`, `SidePanelGit.tsx`, `TerminalPane.tsx` — explicitly excluded by the
  dispatch ("remaining" components; these are presumably covered by the
  browser-review-fixes review already on record).
- `web/src/lib/*.ts` (api.ts, hooks.ts, editorPane.ts, projects.ts, toast.ts, clips.ts,
  lan.ts, sshKey.ts) — not in the named file list; `LocalNetwork`/`DiffPane`/etc. call
  into these but their internals were not independently audited here.
- Go server code (`internal/httpapi/network.go`, `internal/orchestrator`, credential
  broker) that these components talk to — out of scope for this frontend-only pass.
- `web/e2e/run.mjs` was read in full for structure and every LAN/Browser/cleanup
  section, but not every one of its ~100 individual `check()` blocks was traced line by
  line (e.g. the Files/Explorer/CodeMirror sections in the middle third were skimmed
  for structure, not exhaustively re-derived against server behavior).
- No test run was performed (`npm run check`/`lint`/`test`, `web/e2e/run.sh`) — this is
  a static read-only audit; findings are based on source inspection, not execution.
