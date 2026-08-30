import { useEffect, useState } from "react";
import { api, type Project } from "./api";

/** First live terminal of a project, creating one if none exist — the shared
 *  "open a project" step (deduped from Sidebar + dashboard, v1.14.0). */
export async function firstTerminalId(project: string): Promise<string> {
  const { terminals } = await api.listTerminals(project);
  return terminals[0]?.id ?? (await api.createTerminal(project)).id;
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
