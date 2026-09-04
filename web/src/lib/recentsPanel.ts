/**
 * Persisted geometry for the Recent-files strip at the bottom of the Files
 * panel (operator request 2026-09-04: make it collapsible and resizable).
 *
 * GLOBAL, not per project — unlike the recents LIST itself
 * (`styrman.recentFiles.<project>` in editorPane.ts), which is per project
 * because it names that project's files. How tall you like the strip, and
 * whether you want it at all, is a preference about the UI, and re-deciding
 * it for every project would be tedious.
 *
 * The parse/clamp half is pure so the "someone hand-edited localStorage"
 * cases are unit-testable; the storage half is thin wrappers that never throw
 * (a private window throws on ACCESS, not only on write).
 */
export const RECENTS_MIN_HEIGHT = 48;
export const RECENTS_MAX_HEIGHT = 320;
export const RECENTS_DEFAULT_HEIGHT = 96;

const HEIGHT_KEY = "styrman.recents.height";
const COLLAPSED_KEY = "styrman.recents.collapsed";

/**
 * Snap a stored or dragged height into range. Anything unparseable — a
 * hand-edited key, a value from a future version, a NaN out of `parseInt` —
 * falls back to the default rather than collapsing the panel to nothing.
 */
export function clampRecentsHeight(v: unknown): number {
  const n = typeof v === "string" ? Number.parseInt(v, 10) : v;
  if (typeof n !== "number" || !Number.isFinite(n)) return RECENTS_DEFAULT_HEIGHT;
  return Math.min(RECENTS_MAX_HEIGHT, Math.max(RECENTS_MIN_HEIGHT, Math.round(n)));
}

export function loadRecentsHeight(): number {
  try {
    const raw = localStorage.getItem(HEIGHT_KEY);
    return raw === null ? RECENTS_DEFAULT_HEIGHT : clampRecentsHeight(raw);
  } catch {
    return RECENTS_DEFAULT_HEIGHT;
  }
}

export function storeRecentsHeight(px: number): void {
  try {
    localStorage.setItem(HEIGHT_KEY, String(clampRecentsHeight(px)));
  } catch {
    /* private mode — the in-memory value still applies for this session */
  }
}

export function loadRecentsCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

export function storeRecentsCollapsed(collapsed: boolean): void {
  try {
    if (collapsed) localStorage.setItem(COLLAPSED_KEY, "1");
    else localStorage.removeItem(COLLAPSED_KEY);
  } catch {
    /* private mode */
  }
}
