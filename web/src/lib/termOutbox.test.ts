import { describe, expect, it } from "vitest";
import { createOutbox } from "./termOutbox";

// The terminal pane's queue of keys typed while its socket is down
// (termOutbox.ts). The clock is injected: what matters is how OLD the queue
// is when the socket comes back, and a machine that slept in between makes
// that hours (security review 2026-10-02).
function rig(opts: { maxEntries?: number; maxAgeMs?: number } = {}) {
  let t = 1_000_000;
  const box = createOutbox({ ...opts, now: () => t });
  return { box, advance: (ms: number) => (t += ms) };
}

describe("createOutbox", () => {
  it("returns what was typed, in order, and empties", () => {
    const { box } = rig();
    box.push("l");
    box.push("s");
    box.push("\r");
    expect(box.drain()).toEqual(["l", "s", "\r"]);
    expect(box.drain()).toEqual([]);
    expect(box.size).toBe(0);
  });

  it("keeps keys typed a few seconds before the reconnect", () => {
    const { box, advance } = rig();
    box.push("a");
    advance(4_000);
    box.push("b");
    advance(15_000); // the longest single backoff step
    expect(box.drain()).toEqual(["a", "b"]);
  });

  it("drops the whole queue once its oldest key is older than the limit", () => {
    // A machine that slept: the wall clock moved on, the timers did not.
    const { box, advance } = rig();
    box.push("r");
    box.push("m");
    advance(3 * 60 * 60 * 1000);
    expect(box.drain()).toEqual([]);
    expect(box.size).toBe(0);
  });

  it("never sends half a line: one stale key drops the fresh ones too", () => {
    const { box, advance } = rig({ maxAgeMs: 30_000 });
    box.push("rm -rf ");
    advance(29_000);
    box.push("build");
    advance(2_000); // the first chunk is 31 s old, the second 2 s
    expect(box.drain()).toEqual([]);
  });

  it("the limit is exclusive at exactly maxAgeMs", () => {
    const { box, advance } = rig({ maxAgeMs: 30_000 });
    box.push("x");
    advance(30_000);
    expect(box.drain()).toEqual(["x"]);
    box.push("y");
    advance(30_001);
    expect(box.drain()).toEqual([]);
  });

  it("stops taking keys at the cap instead of growing", () => {
    const { box } = rig({ maxEntries: 3 });
    for (const k of ["1", "2", "3", "4", "5"]) box.push(k);
    expect(box.size).toBe(3);
    expect(box.drain()).toEqual(["1", "2", "3"]);
  });

  it("clear empties without sending", () => {
    const { box } = rig();
    box.push("x");
    box.clear();
    expect(box.size).toBe(0);
    expect(box.drain()).toEqual([]);
  });

  it("a queue started after a stale one was dropped is fresh again", () => {
    const { box, advance } = rig({ maxAgeMs: 30_000 });
    box.push("old");
    advance(60_000);
    expect(box.drain()).toEqual([]);
    box.push("new");
    advance(1_000);
    expect(box.drain()).toEqual(["new"]);
  });
});
