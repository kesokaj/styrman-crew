import { useEffect, useState } from "react";
import { api, type Project } from "./api";

/** The terminal to land on when opening a project, creating one if none exist
 *  — the shared "open a project" step (deduped from Sidebar + dashboard,
 *  v1.14.0).
 *
 *  Picks the MOST RECENTLY OPENED terminal, explicitly. This used to fall out
 *  of the server's list order, but that is now sorted by title so the sidebar
 *  stops reshuffling (2026-09-01); taking `terminals[0]` after that change
 *  would land the user on whichever random "adjective noun" title sorts first
 *  (review finding). Resuming where you left off is the behaviour worth
 *  keeping, so it is now stated here rather than implied by a sort order. */
export async function firstTerminalId(project: string): Promise<string> {
  const { terminals } = await api.listTerminals(project);
  if (terminals.length === 0) return (await api.createTerminal(project)).id;
  return terminals.reduce((best, t) =>
    Date.parse(t.lastOpened) > Date.parse(best.lastOpened) ? t : best,
  ).id;
}

/**
 * ONE app-wide listProjects poll (poll de-amplification, operator item,
 * v1.14.0): Sidebar, dashboard, and Settings previously each ran their own
 * interval against the same endpoint. All subscribers now share a single 5s
 * tick that pauses in hidden tabs and stops with the last subscriber; any
 * mutation calls refreshProjects() and every surface updates at once.
 */
type ProjectsState = { data?: { projects: Project[] }; error?: string };

let state: ProjectsState = {};
const subs = new Set<(s: ProjectsState) => void>();
let timer: ReturnType<typeof setInterval> | undefined;
let inFlight = false;
// A refresh requested while a poll is mid-flight must not be swallowed —
// that in-flight response predates the mutation (audit 2026-08-30).
let pendingRefresh = false;

async function tick() {
  if (typeof document !== "undefined" && document.hidden) return;
  if (inFlight) {
    pendingRefresh = true;
    return;
  }
  inFlight = true;
  try {
    state = { data: await api.listProjects() };
  } catch (e) {
    // Keep the last good data; surface the error alongside it.
    state = { ...state, error: (e as Error).message };
  } finally {
    inFlight = false;
  }
  subs.forEach((fn) => fn(state));
  if (pendingRefresh) {
    pendingRefresh = false;
    void tick();
  }
}

// Catch up immediately when a hidden tab becomes visible (ticks are
// suppressed while hidden), matching usePolled's behavior.
function onVisible() {
  if (!document.hidden) void tick();
}

/** Force an immediate refresh (after create/delete/stop/start/rename). */
function refreshProjects() {
  void tick();
}

export function useProjects(): ProjectsState & { refresh: () => void } {
  const [s, setS] = useState<ProjectsState>(state);
  useEffect(() => {
    subs.add(setS);
    setS(state);
    if (!timer) {
      void tick();
      timer = setInterval(tick, 5000);
      document.addEventListener("visibilitychange", onVisible);
    }
    return () => {
      subs.delete(setS);
      if (subs.size === 0 && timer) {
        clearInterval(timer);
        timer = undefined;
        document.removeEventListener("visibilitychange", onVisible);
      }
    };
  }, []);
  return { ...s, refresh: refreshProjects };
}
