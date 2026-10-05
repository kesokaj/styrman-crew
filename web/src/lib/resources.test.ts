import { describe, expect, it } from "vitest";
import type { ProjectResources } from "./api";
import { bodyFrom, formFrom, unchanged } from "./resources";

const limits = { maxCpus: 16, maxMemoryGiB: 60, minDiskGB: 20, maxDiskGB: 500 };
const unlimited: ProjectResources = { cpus: 0, memoryGiB: 0, diskGB: 20, pending: false, limits };

describe("formFrom", () => {
  it("ticks Unlimited for 0 and prefills a number for when it is unticked", () => {
    const f = formFrom(unlimited);
    expect(f).toEqual({ cpusUnlimited: true, cpus: "4", memoryUnlimited: true, memoryGiB: "8", diskGB: "20" });
  });

  it("shows a set size as set", () => {
    const f = formFrom({ ...unlimited, cpus: 2, memoryGiB: 4, diskGB: 30 });
    expect(f).toEqual({ cpusUnlimited: false, cpus: "2", memoryUnlimited: false, memoryGiB: "4", diskGB: "30" });
  });

  it("never prefills above a small host's limits", () => {
    const f = formFrom({ ...unlimited, limits: { ...limits, maxCpus: 2, maxMemoryGiB: 3 } });
    expect(f.cpus).toBe("2");
    expect(f.memoryGiB).toBe("3");
  });
});

describe("bodyFrom", () => {
  it("sends 0 for Unlimited, whatever the box holds", () => {
    const r = bodyFrom({ ...formFrom(unlimited), cpus: "junk", memoryGiB: "junk" }, limits);
    expect(r).toEqual({ body: { cpus: 0, memoryGiB: 0, diskGB: 20 } });
  });

  it("sends a set size", () => {
    const f = { cpusUnlimited: false, cpus: "2", memoryUnlimited: false, memoryGiB: " 4 ", diskGB: "21" };
    expect(bodyFrom(f, limits)).toEqual({ body: { cpus: 2, memoryGiB: 4, diskGB: 21 } });
  });

  it.each([
    ["0", /CPU/],
    ["17", /CPU.*1 to 16/],
    ["1.5", /CPU/],
    ["-1", /CPU/],
    ["", /CPU/],
  ])("refuses CPU %j", (cpus, msg) => {
    const r = bodyFrom({ ...formFrom(unlimited), cpusUnlimited: false, cpus }, limits);
    expect("error" in r && r.error).toMatch(msg);
  });

  it("refuses memory above the host", () => {
    const r = bodyFrom({ ...formFrom(unlimited), memoryUnlimited: false, memoryGiB: "61" }, limits);
    expect("error" in r && r.error).toMatch(/Memory.*1 to 60/);
  });

  it("sends memory 0 on a host too small to set one", () => {
    const small = { ...limits, maxMemoryGiB: 0 };
    const r = bodyFrom({ ...formFrom(unlimited), memoryUnlimited: false, memoryGiB: "4" }, small);
    expect(r).toEqual({ body: { cpus: 0, memoryGiB: 0, diskGB: 20 } });
  });

  it.each(["19", "501", "20.5", "x"])("refuses disk %j (it can only grow)", (diskGB) => {
    const r = bodyFrom({ ...formFrom(unlimited), diskGB }, limits);
    expect("error" in r && r.error).toMatch(/20–500 GB.*only grow/);
  });
});

describe("unchanged", () => {
  it("compares the body with the saved values", () => {
    expect(unchanged({ cpus: 0, memoryGiB: 0, diskGB: 20 }, unlimited)).toBe(true);
    expect(unchanged({ cpus: 2, memoryGiB: 0, diskGB: 20 }, unlimited)).toBe(false);
    expect(unchanged({ cpus: 0, memoryGiB: 0, diskGB: 21 }, unlimited)).toBe(false);
  });
});
