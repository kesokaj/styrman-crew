import { beforeEach, describe, expect, it, vi } from "vitest";

/**
 * clips.ts documents three invariants nothing tested (audit 2026-09-13): a
 * re-copied text moves to the front instead of duplicating, entries older
 * than the TTL are dropped, and getClips returns the SAME array when nothing
 * changed (useSyncExternalStore loops on a fresh reference).
 */
describe("clips", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-09-13T12:00:00Z"));
    const mem = new Map<string, string>();
    vi.stubGlobal("window", {
      sessionStorage: {
        getItem: (k: string) => mem.get(k) ?? null,
        setItem: (k: string, v: string) => void mem.set(k, v),
        removeItem: (k: string) => void mem.delete(k),
      },
    });
  });

  it("dedups on re-copy and keeps newest first", async () => {
    const { addClip, getClips } = await import("./clips");
    addClip("a");
    addClip("b");
    addClip("a ");
    expect(getClips().map((c) => c.text)).toEqual(["a", "b"]);
  });

  it("drops entries past the TTL and returns a stable reference otherwise", async () => {
    const { addClip, getClips } = await import("./clips");
    addClip("old");
    const first = getClips();
    expect(getClips()).toBe(first);
    vi.advanceTimersByTime(30 * 60 * 1000);
    addClip("new");
    expect(getClips().map((c) => c.text)).toEqual(["new", "old"]);
    vi.advanceTimersByTime(31 * 60 * 1000); // "old" is now past one hour
    const after = getClips();
    expect(after.map((c) => c.text)).toEqual(["new"]);
    expect(getClips()).toBe(after);
  });

  it("ignores blank text and clears on demand", async () => {
    const { addClip, clearClips, getClips } = await import("./clips");
    addClip("   ");
    expect(getClips()).toEqual([]);
    addClip("x");
    clearClips();
    expect(getClips()).toEqual([]);
  });
});
