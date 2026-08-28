# web/ — the styrman SPA

TanStack Start + React 19 + TypeScript (strict) + Tailwind v4. This is the
entire frontend; the Go server embeds the built output (`web/embed.go` does
`go:embed dist/client`), so **`dist/client` is committed** — rebuilding the
frontend and rebuilding the server binary are separate steps.

```bash
npm ci          # once
npm run build   # typecheck + vite build + prerender → dist/client (commit it)
npx tsc --noEmit   # typecheck only
```

Layout (`src/`): `routes/` (file-based routes: dashboard `index.tsx`,
`p.$project.$term.tsx` terminal view, `settings.tsx`, `setup.tsx`) ·
`components/` (Sidebar, TerminalPane, SidePanel, FileExplorer, dialogs, ui
primitives) · `lib/` (`api.ts` — the ONLY fetch layer, typed against
`.agents/docs/specs/styrman/api.md`; `hooks.ts` — `usePolled` etc.;
`terminal.ts` — xterm wiring; `toast.ts`, `clips.ts`, `theme.ts`).

Conventions: every server call goes through `lib/api.ts`; mutations wrap in
`tryUI` (toast on failure); polling uses `usePolled` (in-flight guard,
stale-response guard, hidden-tab pause, queued refresh). Theme = Material 3
tokens in `src/index.css` (light/dark/system via `data-theme`).

Tests: `e2e/run.sh` self-hosts the server (real Kata VMs) and drives
headless Chromium through 35 checks (`e2e/run.mjs`). See
`.agents/docs/testing/styrman/README.md`.
