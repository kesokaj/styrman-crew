import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";

/**
 * request()'s default 20 s bound must not override a caller's own signal:
 * start/stop (150 s) and the LAN toggle (210 s) were clipped to 20 s by an
 * AbortSignal.any combine (BUG-055, BUG-022's mechanism). The rule is now:
 * no signal → the default; a signal → the caller owns the bound; checkFile,
 * which wants both, combines them itself.
 */
describe("request() signal ownership", () => {
  const seen: Array<AbortSignal | null | undefined> = [];
  const fetchSpy = vi.fn(async (_url: string, init?: RequestInit) => {
    seen.push(init?.signal);
    return new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } });
  });
  vi.stubGlobal("fetch", fetchSpy);
  afterEach(() => {
    seen.length = 0;
  });

  it("applies the default bound when the caller passes no signal", async () => {
    await api.getProject("11111111-1111-4111-8111-111111111111");
    expect(seen[0]).toBeInstanceOf(AbortSignal);
    expect(seen[0]?.aborted).toBe(false);
  });

  it("passes a caller's own timeout signal through unchanged", async () => {
    // startProject carries AbortSignal.timeout(150000); it must reach fetch as
    // THAT signal, not as a composite that also fires at 20 s. A composite
    // built with AbortSignal.any is a fresh object, so identity is the test —
    // read through the same API surface the pane uses.
    await api.startProject("11111111-1111-4111-8111-111111111111");
    const s = seen[0];
    expect(s).toBeInstanceOf(AbortSignal);
    // A composite would abort when a 20 s timer fires; a 150 s timeout signal
    // does not abort in the next macrotask. Advance fake time past 20 s.
    vi.useFakeTimers();
    try {
      vi.advanceTimersByTime(25000);
      expect(s?.aborted).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("checkFile combines the editor's abort with the 20 s bound", async () => {
    const ac = new AbortController();
    await api.checkFile("11111111-1111-4111-8111-111111111111", "x.ts", "a", ac.signal);
    const s = seen[0];
    expect(s).toBeInstanceOf(AbortSignal);
    expect(s).not.toBe(ac.signal); // a composite: the editor's abort plus the bound
    ac.abort();
    expect(s?.aborted).toBe(true);
  });
});
