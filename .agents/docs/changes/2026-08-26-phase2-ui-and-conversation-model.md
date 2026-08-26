# 2026-08-26 — Phase 2: real UI + durable conversation model

## Summary

Phase 2 delivered: the Antigravity-style web UI (TanStack Router + Vite +
Tailwind v4 SPA, embedded in the Go binary, dark default + light theme) and
the ADR-011 conversation model — conversations are now durable records with
agent-native resume, not tmux sessions. The /dev/ test console is deleted.
Integration suite grew to 23 checks, all passing.

## What was built

**Backend**
- `internal/conversation/store.go`: persisted records {id, title,
  agentSession (UUID), createdAt, lastOpened}; atomic+fsync'd JSON.
- Bridge: claude conversations run `claude --session-id <uuid>` first time,
  `claude --resume <uuid>` after — history survives container restarts,
  idle stops, and reboots. agy relaunches plain `agy` (resume TODO).
- Shells split into ephemeral "terminals" (tmux `term-*`, no records) with
  their own list/create/delete endpoints; conversations gained PATCH rename;
  project delete cleans its conversation records.

**UI (`web/`)**
- Sidebar: projects (state dot, agent tag, stop/start/delete menu) with
  nested conversations (rename via header, delete, live dot), New Project
  modal (agent picker, git-init), first-time-setup banner with live
  credential checklist, theme toggle.
- Center: conversation header + xterm terminal implementing the full wire
  protocol (4000 no-reconnect, 4001 never-reconnect, transient auto-retry,
  ResizeObserver-driven resize).
- Right rail: collapsible Terminals panel with its own embedded terminal.
- Routes /, /p/$project/$conv, /setup (deep-linkable; Go serves index.html
  fallback). Setup page boots the hidden VM and attaches it.

## Failures / gotchas during the work

1. **Shell cwd mixup silently skipped writing web/index.html** — the built
   dist shipped Vite's default title and no FOWT script; caught by the two
   new SPA integration checks (23-check suite), then fixed. Lesson: the
   Bash tool's persistent cwd + `cd X && …` chains are a footgun; use
   absolute paths.
2. TS strictness: `erasableSyntaxOnly` rejects constructor parameter
   properties; WebSocket.send needs an ArrayBuffer, not
   Uint8Array<ArrayBufferLike> (TextEncoder). Both trivial once seen.
3. Docker SDK v28 pin from Phase 1 carried over cleanly; no new deps beyond
   the UI toolchain.

## Decisions

- ADR-012: TanStack Router + Vite static SPA instead of full TanStack Start
  (no SSR/server functions possible in an embedded binary); dist/ committed
  so `go build` needs no Node.

## Verification

- `go test ./...` green (project + conversation stores), `go vet` clean,
  `tsc --noEmit` clean.
- Integration: 23/23 (new: SPA served, deep-link fallback, terminals API,
  durable records, UUID pinning, rename, live-after-attach, record delete).
- Live on http://192.168.86.58:8080 — new UI serving, demo projects intact.
