import { describe, expect, it } from "vitest";
import { lanChip, lanLine, preferOverride } from "./lan";

describe("lanChip", () => {
  it("flags a LAN project", () => {
    const c = lanChip({ lanAccess: true });
    expect(c.text).toBe("LAN");
    expect(c.className).toContain("warn");
    expect(c.title).toMatch(/local network/i);
  });

  // The safe default gets no decoration — and this is the assertion that
  // matters, because the badge renders from optional fields: a regression
  // that showed "LAN" on every project would be a lie about isolation, and
  // tsc cannot catch it.
  it("renders nothing for a sandboxed project", () => {
    expect(lanChip({ lanAccess: false }).text).toBe("");
  });

  it("renders nothing when the field is absent", () => {
    expect(lanChip({} as { lanAccess?: boolean } as Parameters<typeof lanChip>[0]).text).toBe("");
  });
});

describe("lanLine", () => {
  it("says what LAN mode grants, including its limits", () => {
    const s = lanLine(true);
    expect(s).toMatch(/reaches whatever this server reaches/i);
    expect(s).toMatch(/cannot reach other projects/i);
    expect(s).toMatch(/open a connection into it/i);
  });

  it("says sandbox is internet-only", () => {
    const s = lanLine(false);
    expect(s).toMatch(/internet and nothing else/i);
    expect(s).toMatch(/not the local network/i);
  });
});

describe("preferOverride", () => {
  it("returns the poll when there is no override", () => {
    expect(preferOverride(undefined, { lanAccess: true })).toEqual({ lanAccess: true });
  });

  it("returns the override until the poll agrees", () => {
    const override = { lanAccess: true };
    expect(preferOverride(override, { lanAccess: false })).toBe(override);
    const caughtUp = { lanAccess: true };
    expect(preferOverride(override, caughtUp)).toBe(caughtUp);
  });

  // BUG-020: the old version compared a monotonic timestamp, so an
  // out-of-band change could never displace a local one and the switch
  // latched on the last PUT forever. With one boolean the override yields the
  // moment the server agrees, and there is no clock to latch on.
  it("yields to an out-of-band change once the server agrees", () => {
    const override = { lanAccess: true };
    const outOfBand = { lanAccess: true, source: "someone else" };
    expect(preferOverride(override, outOfBand)).toBe(outOfBand);
  });
});
