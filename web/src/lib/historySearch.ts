import type { HistoryMatch } from "./api";

// The history panel's search state, kept out of the component so it can be
// tested without a DOM (plan D13). A search starts on Enter; a newer search,
// clearing the box, the clear button and leaving the panel all cancel the one
// in flight, so it stops holding one of the server's search slots (v1.42.3)
// and its late answer can never refill a list the user cleared.

export interface HistorySearchDeps {
  search: (q: string, signal: AbortSignal) => Promise<HistoryMatch[]>;
  onMatches: (m: HistoryMatch[] | null) => void;
  onSearching: (searching: boolean) => void;
  onError: (message: string) => void;
}

export interface HistorySearch {
  /** Search for q; an empty q is cancel(). */
  run(q: string): Promise<void>;
  /** Abort the search in flight, drop its answer, clear the results. */
  cancel(): void;
}

export function createHistorySearch(deps: HistorySearchDeps): HistorySearch {
  let gen = 0; // two quick Enters can resolve out of order: only the newest wins
  let inFlight: AbortController | null = null;

  const cancel = () => {
    gen++;
    inFlight?.abort();
    inFlight = null;
    deps.onMatches(null);
    deps.onSearching(false);
  };

  const run = async (q: string) => {
    const trimmed = q.trim();
    if (!trimmed) {
      cancel();
      return;
    }
    const mine = ++gen;
    inFlight?.abort(); // a new search supersedes the previous one
    const ac = new AbortController();
    inFlight = ac;
    deps.onSearching(true);
    try {
      const matches = await deps.search(trimmed, ac.signal);
      if (mine === gen) deps.onMatches(matches);
    } catch (e) {
      if (mine === gen && !ac.signal.aborted) deps.onError((e as Error).message);
    } finally {
      if (mine === gen) {
        deps.onSearching(false);
        inFlight = null;
      }
    }
  };

  return { run, cancel };
}
