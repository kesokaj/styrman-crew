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
  set(project, term, { view: "editor", file: { scope, path }, dirty: false });
}

/**
 * Drop `path` (or anything under it, when it is a directory) from EVERY
 * terminal's pane in this project. Called when a file is deleted or renamed:
 * an editor left open on a stale path would recreate the deleted file on the
 * next Save — the hazard the explorer's closeIfOpen has always guarded for the
 * modal, which the pane must honour too (review finding, 2026-09-01).
 */
export function closeFileInPane(project: string, path: string) {
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
