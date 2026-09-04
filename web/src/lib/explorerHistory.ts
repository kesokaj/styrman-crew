/**
 * Back/forward history for the file explorer (operator request 2026-09-04:
 * "going back or forwards is a bit small" — there was in fact no back or
 * forward at all, only breadcrumbs and a `../` row).
 *
 * A plain reducer rather than component state so the edge cases — the
 * truncated forward tail, the cap, the no-op push — are unit-testable without
 * a React renderer. The explorer keeps ONE of these per scope and throws it
 * away when the scope changes: project and home are different trees, and
 * offering "back" to a path that does not exist in the tree you are now
 * looking at is worse than offering nothing.
 */
export type History = {
  /** Visited paths, oldest first. Always non-empty. */
  stack: string[];
  /** Index of the entry currently on screen. */
  index: number;
};

/**
 * Enough to cover any realistic browsing session, and bounded so a long-lived
 * panel cannot grow it without limit. When it overflows, the OLDEST entry is
 * dropped — the far end of the history is the part nobody reaches.
 */
export const HISTORY_CAP = 50;

export const initialHistory = (path = ""): History => ({
  stack: [path],
  index: 0,
});

export const currentPath = (h: History): string => h.stack[h.index] ?? "";

export const canGoBack = (h: History): boolean => h.index > 0;
export const canGoForward = (h: History): boolean =>
  h.index < h.stack.length - 1;

/**
 * Navigate to `path`, dropping any forward entries — the branch you did not
 * take is gone the moment you take another, exactly as a browser does it.
 *
 * Returns the SAME object when the path is already current, so a redundant
 * navigation cannot trigger a re-render or a refetch.
 */
export function pushPath(h: History, path: string): History {
  if (currentPath(h) === path) return h;
  const stack = [...h.stack.slice(0, h.index + 1), path];
  if (stack.length > HISTORY_CAP) {
    const overflow = stack.length - HISTORY_CAP;
    return { stack: stack.slice(overflow), index: HISTORY_CAP - 1 };
  }
  return { stack, index: stack.length - 1 };
}

export const goBack = (h: History): History =>
  canGoBack(h) ? { ...h, index: h.index - 1 } : h;

export const goForward = (h: History): History =>
  canGoForward(h) ? { ...h, index: h.index + 1 } : h;

/** The containing directory, or "" at the root. */
export const parentOf = (path: string): string =>
  path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";

/**
 * Crumbs to render, middle-elided when the path is deeper than `keep`.
 *
 * The panel is ~320px wide, so a deep path used to wrap onto a second line
 * and push the listing down. `elided` marks where segments were dropped; the
 * caller renders one non-interactive gap marker there, and the segments it
 * does show stay clickable with their real ABSOLUTE path — not an offset into
 * the shortened list.
 */
export type Crumb = { name: string; path: string };

export function crumbTrail(
  path: string,
  keep = 3,
): { crumbs: Crumb[]; elided: boolean } {
  const segs = path.split("/").filter(Boolean);
  const all: Crumb[] = segs.map((name, i) => ({
    name,
    path: segs.slice(0, i + 1).join("/"),
  }));
  if (all.length <= keep) return { crumbs: all, elided: false };
  return { crumbs: all.slice(all.length - keep), elided: true };
}
