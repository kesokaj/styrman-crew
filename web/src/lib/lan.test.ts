import { describe, expect, it } from "vitest";
import { formatClock, lanChip, lanLine, preferOverride } from "./lan";

describe("lanChip", () => {
  it("renders nothing while off or unavailable", () => {
    expect(lanChip({}).text).toBe("");
    expect(lanChip({ lanState: "off" }).text).toBe("");
    expect(lanChip({ lanAccess: true, lanState: "unavailable", lanMessage: "not installed" }).text).toBe("");
  });
  it("flags on in warn colours with the prefix in the title", () => {
    const c = lanChip({ lanAccess: true, lanState: "on" }, ["192.168.86.0/24"]);
    expect(c.text).toBe("LAN");
    expect(c.className).toContain("warn");
    expect(c.title).toContain("192.168.86.0/24");
    expect(c.pending).toBe(false);
  });
  it("pulses while pending and says which way it is going", () => {
    expect(lanChip({ lanAccess: true, lanState: "pending" }).pending).toBe(true);
    expect(lanChip({ lanAccess: true, lanState: "pending" }).title).toContain("applying");
    expect(lanChip({ lanAccess: false, lanState: "pending" }).title).toContain("closing");
  });
  it("carries root's message on error", () => {
    const c = lanChip({ lanAccess: true, lanState: "error", lanMessage: "host firewall reported: insert failed" });
    expect(c.className).toContain("danger");
    expect(c.title).toBe("host firewall reported: insert failed");
  });
  it("falls back to a generic place when the prefix is unknown (non-admin)", () => {
    expect(lanChip({ lanAccess: true, lanState: "on" }).title).toContain("your local network");
  });
});

describe("lanLine", () => {
  it("names what stays blocked while on", () => {
    const l = lanLine("on", true, "", ["192.168.86.0/24"], "2026-09-08T10:00:00Z");
    expect(l).toContain("192.168.86.0/24");
    expect(l).toContain("Still blocked");
    expect(l).toContain("Applied ");
  });
  it("distinguishes the two pending directions and the no-bridge case", () => {
    expect(lanLine("pending", true, "applying on the host firewall")).toContain("Applying");
    expect(lanLine("pending", true, "applies when the project next starts")).toContain("next starts");
    expect(lanLine("pending", false, "closing on the host firewall")).toContain("drop immediately");
  });
  it("relays root's error and the unavailability reason", () => {
    expect(lanLine("error", true, "cannot confirm closed")).toContain("cannot confirm closed");
    expect(lanLine("unavailable", true, "host firewall script is outdated (have 2, need 3)")).toContain("have 2, need 3");
    expect(lanLine("unavailable", true, "")).toContain("not installed");
  });
  it("describes the default while off", () => {
    expect(lanLine(undefined, false, "")).toContain("Internet only");
    expect(lanLine("off", false, "")).toContain("cannot reach");
  });
});

describe("formatClock", () => {
  it("returns the input unchanged when it is not a date", () => {
    expect(formatClock("garbage")).toBe("garbage");
  });
  it("formats a real stamp as a time", () => {
    expect(formatClock("2026-09-08T10:00:00Z")).toMatch(/\d/);
  });
});

describe("preferOverride", () => {
  const at = (s: string) => ({ lanChangedAt: s });

  it("shows the reply until the poll catches up", () => {
    const o = at("2026-09-08T10:00:00Z");
    const stalePoll = at("2026-09-08T09:59:00Z");
    expect(preferOverride(o, stalePoll)).toBe(o);
  });

  it("hands over once the poll reports the same change", () => {
    const o = at("2026-09-08T10:00:00Z");
    const same = at("2026-09-08T10:00:00Z");
    expect(preferOverride(o, same)).toBe(same);
  });

  // The bug: an out-of-band change moves the poll PAST the override, and an
  // equality test pinned the component to the stale reply forever.
  it("hands over when the record changed out of band", () => {
    const o = at("2026-09-08T10:00:00Z");
    const newer = at("2026-09-08T10:05:00Z");
    expect(preferOverride(o, newer)).toBe(newer);
  });

  it("falls back to equality when a timestamp is unusable", () => {
    const o = { lanChangedAt: undefined };
    const d = { lanChangedAt: undefined };
    expect(preferOverride(o, d)).toBe(d);
    const junk = { lanChangedAt: "not a date" };
    expect(preferOverride(junk, at("2026-09-08T10:00:00Z"))).toBe(junk);
  });

  it("copes with either side missing", () => {
    const d = at("2026-09-08T10:00:00Z");
    expect(preferOverride(undefined, d)).toBe(d);
    expect(preferOverride(d, undefined)).toBe(d);
  });
});
