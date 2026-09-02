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
  /**
   * The unsaved editor buffer, parked here so it SURVIVES the pane unmounting.
   *
   * Keying EditorPane per terminal (2026-09-01) stopped terminal A's dirty
   * state leaking onto B, but the buffer itself still died with the remount:
   * opening another file prompts to save, closing the editor prompts to save,
   * switching terminals silently threw the edits away. The draft carries its
   * own path so a stale buffer can never be restored over a different file.
   */
  draft: { path: string; text: string } | null;
};

const DEFAULT: PaneState = { view: "terminal", file: null, dirty: false, draft: null };

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
  // PRESERVE the draft for the same reason as `dirty`: when this open is
  // parked behind the discard-confirm, the previous file's unsaved buffer must
  // still be there if the user backs out.
  set(project, term, {
    view: "editor",
    file: { scope, path },
    dirty: get(project, term).dirty,
    draft: get(project, term).draft,
  });
}

/** Last path segment — the display name for chips and the header badge. */
export function basename(path: string): string {
  return path.includes("/") ? path.slice(path.lastIndexOf("/") + 1) : path;
}

// ── Recently opened files (operator request 2026-09-01) ─────────────────────
// Per PROJECT (shared by that project's terminals), newest first, deduped,
// capped. Unlike the open-file state above (session context by design), the
// recents LIST is a convenience worth keeping across reloads — localStorage,
// guarded: a blocked or empty store must never break the explorer, so every
// touch is try/catched and the feature silently degrades to session-only.
// Known limits, accepted: no cross-tab coherence (last writer wins) and the
// per-project keys are never garbage-collected.

export type RecentFile = { scopeKind: Scope["kind"]; path: string };

const RECENTS_CAP = 8;
const recentsKey = (project: string) => `styrman.recentFiles.${project}`;
// Arrays are replaced, never mutated: useSyncExternalStore compares snapshots
// by identity.
const recents = new Map<string, RecentFile[]>();
const NO_RECENTS: RecentFile[] = [];

/**
 * Canonical relative path or null: no leading slash, no empty/dot segments,
 * no dot-dot at all (the server's cleanRel NORMALISES ".." — the client is
 * stricter on purpose: a recent that only exists after normalisation is not a
 * key the listing ever produces), no trailing slash, no control characters
 * (defensive — such a name would be unusable in the UI), bounded length. "./a.txt", "a//b" and "subdir/"
 * all normalise to the key the listing uses (code review, 2026-09-02).
 */
function canonicalPath(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length > 512) return null;
  // eslint-disable-next-line no-control-regex
  if (raw.startsWith("/") || /[\x00-\x1f\x7f]/.test(raw)) return null;
  const segs = raw.split("/").filter((s) => s !== "" && s !== ".");
  if (segs.length === 0 || segs.includes("..")) return null;
  return segs.join("/");
}

/**
 * ONE constructing parser for every path into the recents list — the storage
 * boundary (another tab, a legacy schema) AND our own writes — so what is
 * shown, deduped, highlighted and pruned is always the canonical shape.
 * First-wins dedup, capped; foreign properties are dropped, not re-serialised.
 */
function sanitizeRecents(input: unknown): RecentFile[] {
  if (!Array.isArray(input)) return NO_RECENTS;
  const out: RecentFile[] = [];
  const seen = new Set<string>();
  for (const r of input) {
    if (!r || typeof r !== "object") continue;
    const kind = (r as { scopeKind?: unknown }).scopeKind;
    if (kind !== "project" && kind !== "home") continue;
    const path = canonicalPath((r as { path?: unknown }).path);
    if (path === null) continue;
    const k = `${kind}/${path}`;
    if (seen.has(k)) continue;
    seen.add(k);
    out.push({ scopeKind: kind, path });
    if (out.length >= RECENTS_CAP) break;
  }
  return out.length ? out : NO_RECENTS;
}

function loadRecents(project: string): RecentFile[] {
  const cached = recents.get(project);
  if (cached) return cached;
  let list: RecentFile[] = NO_RECENTS;
  try {
    const raw = localStorage.getItem(recentsKey(project));
    if (raw) list = sanitizeRecents(JSON.parse(raw));
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
  // The new entry goes first; sanitizeRecents dedups (first wins, so the
  // older duplicate drops), canonicalises and caps — same rules as the read.
  storeRecents(
    project,
    sanitizeRecents([{ scopeKind, path }, ...loadRecents(project)]),
  );
}

/**
 * Whether an entry (a pane's file or a recents chip) is `target` or lives
 * under it when `target` is a directory — in the SAME scope. Scope is part
 * of identity: project:notes.md and home:notes.md are different files.
 */
function matches(
  entry: { scopeKind: Scope["kind"]; path: string },
  target: { scopeKind: Scope["kind"]; path: string },
): boolean {
  return (
    entry.scopeKind === target.scopeKind &&
    (entry.path === target.path || entry.path.startsWith(target.path + "/"))
  );
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
  scopeKind: Scope["kind"],
  path: string,
) {
  evictFromPanes(project, { scopeKind, path });
  // A deleted/renamed path leaves the recents strip too. Milder hazard than
  // the pane sweep (a stale chip's open just fails to load — it cannot
  // recreate the file), but a strip of dead chips is noise. Files removed
  // from inside the guest (rm in a terminal) are not seen here and age out of
  // the cap instead — accepted.
  const target = { scopeKind, path };
  const prev = loadRecents(project);
  const kept = prev.filter((r) => !matches(r, target));
  if (kept.length !== prev.length) storeRecents(project, kept);
}

/**
 * Drop a file from every pane in the project WITHOUT touching its Recent
 * chip — for a file that still exists but whose on-disk content was replaced
 * underneath the editor (an upload): the stale buffer must go, the chip is
 * still valid (code review, 2026-09-02).
 */
export function evictFromPanes(
  project: string,
  target: { scopeKind: Scope["kind"]; path: string },
) {
  let changed = false;
  for (const [key, st] of states) {
    if (!key.startsWith(`${project}/`) || !st.file) continue;
    // Scope-aware (code review, 2026-09-02): the old path-only sweep cleared a
    // pane holding home:<p> — and its unsaved edits — when project:<p> was
    // deleted. One predicate for panes and chips.
    if (!matches({ scopeKind: st.file.scope.kind, path: st.file.path }, target))
      continue;
    // Keep the view where it is; just drop the file. The pane then shows its
    // "no file open" state rather than silently flipping to the terminal.
    // The draft goes with the file: a buffer for a path that no longer exists
    // must never be restored into whatever opens next.
    states.set(key, { ...st, file: null, dirty: false, draft: null });
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
  /** Park (or clear, with null) the unsaved buffer for this terminal. */
  const setDraft = useCallback(
    (draft: PaneState["draft"]) => {
      set(project, term, { ...get(project, term), draft });
    },
    [project, term],
  );
  return { state, setView, setFile, setDirty, setDraft };
}
