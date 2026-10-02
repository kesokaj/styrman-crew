import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createLiveness, isPong, PING_FRAME } from "./termLiveness";

// The terminal pane's dead-socket check (ADR-079, BUG-085), driven with fake
// timers: what is sent, and when the socket is declared dead.
function rig(timeoutMs?: number) {
  const pings: number[] = [];
  let dead = 0;
  const l = createLiveness({
    sendPing: () => {
      pings.push(Date.now());
    },
    onDead: () => {
      dead++;
    },
    timeoutMs,
  });
  return { l, pings, deadCount: () => dead };
}

describe("createLiveness", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("pings once on open and sets no deadline", () => {
    const r = rig();
    r.l.opened();
    expect(r.pings).toHaveLength(1);
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(0);
  });

  it("never declares a connection dead that has not answered a ping", () => {
    // A server without the pong reply (a rollback under an open page): the
    // probe must not turn every wake into a reconnect.
    const r = rig();
    r.l.opened();
    r.l.probe();
    r.l.probe();
    expect(r.pings).toHaveLength(1); // the open ping only
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(0);
  });

  it("declares the socket dead once, when a probe goes unanswered", () => {
    const r = rig();
    r.l.opened();
    r.l.pong();
    r.l.probe();
    expect(r.pings).toHaveLength(2);
    vi.advanceTimersByTime(4_999);
    expect(r.deadCount()).toBe(0);
    vi.advanceTimersByTime(1);
    expect(r.deadCount()).toBe(1);
    // Dead is final for this connection: no second verdict, no more pings.
    r.l.probe();
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(1);
    expect(r.pings).toHaveLength(2);
  });

  it("a pong inside the deadline keeps the socket alive", () => {
    const r = rig();
    r.l.opened();
    r.l.pong();
    r.l.probe();
    vi.advanceTimersByTime(3_000);
    r.l.pong();
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(0);
  });

  it("any frame inside the deadline keeps the socket alive", () => {
    // Terminal output arriving is proof enough; the pong may be queued behind it.
    const r = rig();
    r.l.opened();
    r.l.pong();
    r.l.probe();
    vi.advanceTimersByTime(3_000);
    r.l.received();
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(0);
  });

  it("does not stack probes while one is pending", () => {
    const r = rig();
    r.l.opened();
    r.l.pong();
    r.l.probe();
    r.l.probe();
    r.l.probe();
    expect(r.pings).toHaveLength(2); // open + one probe
    // The deadline is the FIRST probe's: later calls must not push it out.
    vi.advanceTimersByTime(5_000);
    expect(r.deadCount()).toBe(1);
  });

  it("probes again after an answered probe", () => {
    const r = rig();
    r.l.opened();
    r.l.pong();
    r.l.probe();
    r.l.pong();
    r.l.probe();
    expect(r.pings).toHaveLength(3);
    vi.advanceTimersByTime(5_000);
    expect(r.deadCount()).toBe(1);
  });

  it("stop cancels a pending deadline and silences every later call", () => {
    const r = rig();
    r.l.opened();
    r.l.pong();
    r.l.probe();
    r.l.stop();
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(0);
    r.l.probe();
    r.l.opened();
    expect(r.pings).toHaveLength(2);
    vi.advanceTimersByTime(60_000);
    expect(r.deadCount()).toBe(0);
  });

  it("honours a custom timeout", () => {
    const r = rig(250);
    r.l.opened();
    r.l.pong();
    r.l.probe();
    vi.advanceTimersByTime(249);
    expect(r.deadCount()).toBe(0);
    vi.advanceTimersByTime(1);
    expect(r.deadCount()).toBe(1);
  });
});

describe("control frames", () => {
  it("the ping frame is the control message the bridge answers", () => {
    expect(JSON.parse(PING_FRAME)).toEqual({ type: "ping" });
  });

  it("recognises a pong and nothing else", () => {
    expect(isPong('{"type":"pong"}')).toBe(true);
    expect(isPong('{"type":"ping"}')).toBe(false);
    expect(isPong('{"type":"resize","cols":80,"rows":24}')).toBe(false);
    expect(isPong("pong")).toBe(false);
    expect(isPong("")).toBe(false);
    expect(isPong("null")).toBe(false);
    expect(isPong('["pong"]')).toBe(false);
  });
});
