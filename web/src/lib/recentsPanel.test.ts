import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RECENTS_DEFAULT_HEIGHT,
  RECENTS_MAX_HEIGHT,
  RECENTS_MIN_HEIGHT,
  clampRecentsHeight,
  loadRecentsCollapsed,
  loadRecentsHeight,
  storeRecentsCollapsed,
  storeRecentsHeight,
} from "./recentsPanel";

function memoryStorage(seed?: Record<string, string>) {
  const m = new Map<string, string>(Object.entries(seed ?? {}));
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, String(v)),
    removeItem: (k: string) => void m.delete(k),
    _dump: () => Object.fromEntries(m),
  };
}

/** A private window: the accessor itself throws, not just the write. */
const hostileStorage = () => ({
  get getItem(): never {
    throw new DOMException("denied", "SecurityError");
  },
  get setItem(): never {
    throw new DOMException("denied", "SecurityError");
  },
  get removeItem(): never {
    throw new DOMException("denied", "SecurityError");
  },
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("clampRecentsHeight", () => {
  it("keeps a value inside the range", () => {
    expect(clampRecentsHeight(120)).toBe(120);
    expect(clampRecentsHeight(RECENTS_MIN_HEIGHT)).toBe(RECENTS_MIN_HEIGHT);
    expect(clampRecentsHeight(RECENTS_MAX_HEIGHT)).toBe(RECENTS_MAX_HEIGHT);
  });

  it("snaps out-of-range values to the nearest bound, never to zero", () => {
    // A drag past the top of the panel must not collapse the strip to
    // nothing, which would leave no handle to drag back.
    expect(clampRecentsHeight(0)).toBe(RECENTS_MIN_HEIGHT);
    expect(clampRecentsHeight(-500)).toBe(RECENTS_MIN_HEIGHT);
    expect(clampRecentsHeight(10_000)).toBe(RECENTS_MAX_HEIGHT);
  });

  it("parses a stored string, since localStorage holds only strings", () => {
    expect(clampRecentsHeight("120")).toBe(120);
    expect(clampRecentsHeight("999")).toBe(RECENTS_MAX_HEIGHT);
  });

  it("falls back to the default for anything unparseable", () => {
    for (const bad of ["", "abc", "NaN", null, undefined, {}, [], NaN, Infinity])
      expect(clampRecentsHeight(bad)).toBe(RECENTS_DEFAULT_HEIGHT);
  });

  it("rounds a fractional drag delta", () => {
    expect(clampRecentsHeight(120.6)).toBe(121);
  });
});

describe("recents height storage", () => {
  it("round-trips through localStorage", () => {
    const s = memoryStorage();
    vi.stubGlobal("localStorage", s);
    storeRecentsHeight(140);
    expect(loadRecentsHeight()).toBe(140);
  });

  it("clamps on the way OUT as well as in — a hand-edited key cannot break the panel", () => {
    vi.stubGlobal("localStorage", memoryStorage({ "styrman.recents.height": "99999" }));
    expect(loadRecentsHeight()).toBe(RECENTS_MAX_HEIGHT);
    vi.stubGlobal("localStorage", memoryStorage({ "styrman.recents.height": "junk" }));
    expect(loadRecentsHeight()).toBe(RECENTS_DEFAULT_HEIGHT);
  });

  it("clamps on the way in, so an out-of-range value is never persisted", () => {
    const s = memoryStorage();
    vi.stubGlobal("localStorage", s);
    storeRecentsHeight(10_000);
    expect(s._dump()["styrman.recents.height"]).toBe(String(RECENTS_MAX_HEIGHT));
  });

  it("uses the default when nothing is stored", () => {
    vi.stubGlobal("localStorage", memoryStorage());
    expect(loadRecentsHeight()).toBe(RECENTS_DEFAULT_HEIGHT);
  });

  it("survives a private window on both read and write", () => {
    vi.stubGlobal("localStorage", hostileStorage());
    expect(loadRecentsHeight()).toBe(RECENTS_DEFAULT_HEIGHT);
    expect(() => storeRecentsHeight(200)).not.toThrow();
  });
});

describe("recents collapsed storage", () => {
  it("defaults to expanded", () => {
    vi.stubGlobal("localStorage", memoryStorage());
    expect(loadRecentsCollapsed()).toBe(false);
  });

  it("round-trips, and REMOVES the key when expanded again", () => {
    const s = memoryStorage();
    vi.stubGlobal("localStorage", s);
    storeRecentsCollapsed(true);
    expect(loadRecentsCollapsed()).toBe(true);
    storeRecentsCollapsed(false);
    expect(loadRecentsCollapsed()).toBe(false);
    // Absent, not "0": the default is expanded, so expanded stores nothing.
    expect(s._dump()["styrman.recents.collapsed"]).toBeUndefined();
  });

  it("treats any value other than \"1\" as expanded", () => {
    for (const v of ["0", "true", "yes", ""]) {
      vi.stubGlobal("localStorage", memoryStorage({ "styrman.recents.collapsed": v }));
      expect(loadRecentsCollapsed()).toBe(false);
    }
  });

  it("survives a private window on both read and write", () => {
    vi.stubGlobal("localStorage", hostileStorage());
    expect(loadRecentsCollapsed()).toBe(false);
    expect(() => storeRecentsCollapsed(true)).not.toThrow();
  });
});
