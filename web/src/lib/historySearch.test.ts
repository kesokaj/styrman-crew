import { describe, expect, it } from "vitest";
import type { HistoryMatch } from "./api";
import { createHistorySearch } from "./historySearch";

const m = (line: string): HistoryMatch => ({ transcript: "t1", line }) as HistoryMatch;

// A controllable search: each call parks until the test resolves it.
function harness() {
  const calls: { q: string; signal: AbortSignal; resolve: (r: HistoryMatch[]) => void; reject: (e: Error) => void }[] = [];
  const state = { matches: null as HistoryMatch[] | null, searching: false, errors: [] as string[] };
  const s = createHistorySearch({
    search: (q, signal) =>
      new Promise((resolve, reject) => calls.push({ q, signal, resolve, reject })),
    onMatches: (x) => (state.matches = x),
    onSearching: (b) => (state.searching = b),
    onError: (e) => state.errors.push(e),
  });
  return { s, calls, state };
}

describe("history search", () => {
  it("shows the newest search's matches", async () => {
    const { s, calls, state } = harness();
    const p = s.run(" foo ");
    expect(calls[0]!.q).toBe("foo");
    expect(state.searching).toBe(true);
    calls[0]!.resolve([m("a")]);
    await p;
    expect(state.matches).toEqual([m("a")]);
    expect(state.searching).toBe(false);
  });

  it("clearing during a search aborts it and keeps the list empty when the old answer arrives", async () => {
    const { s, calls, state } = harness();
    const p = s.run("foo");
    s.cancel();
    expect(calls[0]!.signal.aborted).toBe(true);
    expect(state.searching).toBe(false);
    calls[0]!.resolve([m("late")]);
    await p;
    expect(state.matches).toBeNull();
    expect(state.searching).toBe(false);
  });

  it("an emptied box cancels like the clear button", async () => {
    const { s, calls, state } = harness();
    void s.run("foo");
    await s.run("   ");
    expect(calls).toHaveLength(1);
    expect(calls[0]!.signal.aborted).toBe(true);
    expect(state.matches).toBeNull();
  });

  it("a newer search aborts the older and only the newer answers", async () => {
    const { s, calls, state } = harness();
    const p1 = s.run("one");
    const p2 = s.run("two");
    expect(calls[0]!.signal.aborted).toBe(true);
    calls[1]!.resolve([m("two")]);
    calls[0]!.resolve([m("one")]);
    await Promise.all([p1, p2]);
    expect(state.matches).toEqual([m("two")]);
  });

  it("reports an error of the current search, never of an aborted one", async () => {
    const { s, calls, state } = harness();
    const p1 = s.run("one");
    s.cancel();
    calls[0]!.reject(new Error("aborted"));
    await p1;
    expect(state.errors).toEqual([]);
    const p2 = s.run("two");
    calls[1]!.reject(new Error("boom"));
    await p2;
    expect(state.errors).toEqual(["boom"]);
    expect(state.searching).toBe(false);
  });
});
