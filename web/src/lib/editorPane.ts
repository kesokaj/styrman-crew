import { useCallback, useSyncExternalStore } from "react";
import type { Scope } from "./api";

/**
 * Per-terminal main-pane view state (operator request 2026-09-01).
 *
 * The main pane shows EITHER the terminal or the editor, toggled from the
 * terminal header. State is keyed per `${project}/${term}` so one terminal can
 * sit on the editor while another in the same project shows a shell.
 *
 * It lives in a module-level store rather than route state on purpose — but
 * for the OPPOSITE reason to the one first recorded here. The route component
 * is NOT remounted on a terminal switch (TanStack Router only keys a match
 * when remountDeps is configured, and none is; that is precisely why
 * TerminalPane carries its own explicit key). So plain useState would PERSIST
 * across the switch and leak terminal A's view — and its open file — onto
 * terminal B. Keying by `${project}/${term}` is what makes the state
 * per-terminal. Deliberately NOT persisted to localStorage: which file you had
 * open is session context, not a durable preference.
 */
export type PaneFile = { scope: Scope; path: string };
export type PaneState = {
  view: "terminal" | "editor";
  /** The file the editor pane is showing, or null before one is opened. */
  file: PaneFile | null;
  /**
   * The pane has unsaved edits. Lives here, not inside the pane, because the
   * TERMINAL view has to show it: the editor is hidden, so its only other
   * dirty cue (an enabled Save button) is invisible exactly when the user has
   * navigated away and might forget. The old modal could not lose this state
   * because it blocked the screen.
   */
  dirty: boolean;
};

const DEFAULT: PaneState = { view: "terminal", file: null, dirty: false };

const states = new Map<string, PaneState>();
const listeners = new Set<() => void>();

const keyOf = (project: string, term: string) => `${project}/${term}`;

function emit() {
  for (const l of listeners) l();
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

function get(project: string, term: string): PaneState {
  return states.get(keyOf(project, term)) ?? DEFAULT;
}

function set(project: string, term: string, next: PaneState) {
  const key = keyOf(project, term);
  const prev = states.get(key) ?? DEFAULT;
  // useSyncExternalStore compares snapshots by identity, so bail on no-ops to
  // avoid an infinite render loop.
  if (
    prev.view === next.view &&
    prev.file === next.file &&
    prev.dirty === next.dirty
  ) {
    return;
  }
  states.set(key, next);
  emit();
}

/** Show a file in the terminal's editor pane and switch the view to it. */
export function openFileInPane(
  project: string,
  term: string,
  scope: Scope,
  path: string,
) {
  noteRecent(project, scope.kind, path);
  // PRESERVE dirty: when the pane parks this open behind the discard-confirm
  // (unsaved edits), the previous file's edits still exist — resetting the
  // flag here hid the unsaved dot exactly while it mattered (review finding).
  // The pane republishes the real flag once a load actually lands.
  set(project, term, {
    view: "editor",
    file: { scope, path },
    dirty: get(project, term).dirty,
  });
}

/** Last path segment — the display name for chips and the header badge. */
export function basename(path: string): string {
  return path.includes("/") ? path.slice(path.lastIndexOf("/") + 1) : path;
}

// ── Recently opened files (operator request 2026-09-01) ─────────────────────
// Per PROJECT, newest first, deduped, capped. Unlike the open-file state
// above (session context by design), the recents LIST is a convenience worth
// keeping across reloads — localStorage, guarded: a blocked or empty store
// must never break the explorer, so every touch is try/catched and the
// feature silently degrades to session-only.

export type RecentFile = { scopeKind: Scope["kind"]; path: string };

const RECENTS_CAP = 8;
const recentsKey = (project: string) => `styrman.recentFiles.${project}`;
// Arrays are replaced, never mutated: useSyncExternalStore compares snapshots
// by identity.
const recents = new Map<string, RecentFile[]>();
const NO_RECENTS: RecentFile[] = [];

function loadRecents(project: string): RecentFile[] {
  const cached = recents.get(project);
  if (cached) return cached;
  let list: RecentFile[] = NO_RECENTS;
  try {
    const raw = localStorage.getItem(recentsKey(project));
    if (raw) {
      const parsed: unknown = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        // Validate hard: this crosses a storage boundary another tab (or a
        // legacy schema) may have written. Reject non-relative and dot-nav
        // paths outright, bound the length, and dedup (duplicate entries
        // would become duplicate React keys).
        const seen = new Set<string>();
        list = parsed.filter((r): r is RecentFile => {
          if (!r || typeof r !== "object") return false;
          const { scopeKind, path } = r as RecentFile;
          if (scopeKind !== "project" && scopeKind !== "home") return false;
          if (typeof path !== "string" || !path || path.length > 512)
            return false;
          if (path.startsWith("/") || path.split("/").includes(".."))
            return false;
          const k = `${scopeKind}/${path}`;
          if (seen.has(k)) return false;
          seen.add(k);
          return true;
        });
        // Bound the ARRAY too, not just each path — a corrupted or foreign
        // blob must not render an unbounded chip strip.
        list = list.slice(0, RECENTS_CAP);
      }
    }
  } catch {
    // blocked storage / bad JSON — start empty
  }
  recents.set(project, list);
  return list;
}

function storeRecents(project: string, list: RecentFile[]) {
  recents.set(project, list);
  try {
    localStorage.setItem(recentsKey(project), JSON.stringify(list));
  } catch {
    // storage blocked/full — keep the in-memory copy for this session
  }
  emit();
}

function noteRecent(project: string, scopeKind: Scope["kind"], path: string) {
  const prev = loadRecents(project);
  const rest = prev.filter(
    (r) => !(r.scopeKind === scopeKind && r.path === path),
  );
  storeRecents(project, [{ scopeKind, path }, ...rest].slice(0, RECENTS_CAP));
}

/** A project's recently opened files, newest first. */
export function useRecentFiles(project: string): RecentFile[] {
  return useSyncExternalStore(
    subscribe,
    () => loadRecents(project),
    () => NO_RECENTS,
  );
}

/**
 * Drop `path` (or anything under it, when it is a directory) from EVERY
 * terminal's pane in this project. Called when a file is deleted or renamed:
 * an editor left open on a stale path would recreate the deleted file on the
 * next Save — the hazard the explorer's closeIfOpen has always guarded for the
 * modal, which the pane must honour too (review finding, 2026-09-01).
 */
export function closeFileInPane(
  project: string,
  path: string,
  scopeKind?: Scope["kind"],
) {
  let changed = false;
  for (const [key, st] of states) {
    if (!key.startsWith(`${project}/`) || !st.file) continue;
    const p = st.file.path;
    if (p !== path && !p.startsWith(path + "/")) continue;
    // Keep the view where it is; just drop the file. The pane then shows its
    // "no file open" state rather than silently flipping to the terminal.
    states.set(key, { ...st, file: null, dirty: false });
    changed = true;
  }
  if (changed) emit();
  // A deleted/renamed path leaves the recents strip too. Milder hazard than
  // the pane sweep above (a stale chip's open just fails to load — it cannot
  // recreate the file), but a strip of dead chips is noise. Same path-prefix
  // coarseness; files removed from inside the guest (rm in a terminal) are
  // not seen here and age out of the cap instead — accepted.
  // Scope-aware when the caller knows it (the explorer does): deleting
  // project:notes.md must not drop a home:notes.md chip (review finding).
  const prev = loadRecents(project);
  const kept = prev.filter(
    (r) =>
      (scopeKind !== undefined && r.scopeKind !== scopeKind) ||
      (r.path !== path && !r.path.startsWith(path + "/")),
  );
  if (kept.length !== prev.length) storeRecents(project, kept);
}

// A save in the editor pane changes a file's size/mtime, but the pane is a
// SIBLING of the explorer, not its child, so it cannot call load() directly
// (the deleted modal could). This counter is the signal; the explorer
// re-lists on every bump. Cheap, and it keeps the two decoupled.
let savedTick = 0;

export function notifyFileSaved() {
  savedTick++;
  emit();
}

/** Re-renders on every file save so a subscriber can refresh its listing. */
export function useFileSavedTick(): number {
  return useSyncExternalStore(
    subscribe,
    () => savedTick,
    () => 0,
  );
}

/** Read + update one terminal's pane state. */
export function useEditorPane(project: string, term: string) {
  const state = useSyncExternalStore(
    subscribe,
    () => get(project, term),
    () => DEFAULT,
  );
  const setView = useCallback(
    (view: PaneState["view"]) => {
      set(project, term, { ...get(project, term), view });
    },
    [project, term],
  );
  const setFile = useCallback(
    (file: PaneFile | null) => {
      set(project, term, { ...get(project, term), file });
    },
    [project, term],
  );
  const setDirty = useCallback(
    (dirty: boolean) => {
      set(project, term, { ...get(project, term), dirty });
    },
    [project, term],
  );
  return { state, setView, setFile, setDirty };
}
