# 2026-08-26 — UI migrated to full TanStack Start (operator decision)

## Summary

Operator chose migration over amending REQUIREMENTS.md. The SPA now runs on
TanStack Start 1.168 in SPA mode: file-based routes (src/routes/), generated
route tree, document rendered by __root.tsx (head/meta/theme script moved
from index.html), prerendered static shell. Go embeds dist/client and serves
_shell.html as the deep-link fallback; dist/server (Start's unused Node
handler) is gitignored. Runtime story unchanged: one Go binary, no Node.

## Notes / gotchas

- routeTree.gen.ts is generated during `vite build`, so the build script is
  now `vite build && tsc -b` (typecheck after codegen).
- index.html and src/main.tsx are gone; the FOWT theme script and <title>
  live in __root.tsx head(); index.css is linked via `?url` import.
- The `html,body,#root` height CSS became `html,body` (Start renders the app
  straight into <body>).

## Verification

go test ./... green · integration 23/23 · Playwright e2e 18/18 (unchanged
suites — the UI behaves identically) · live on http://192.168.86.58:8080
