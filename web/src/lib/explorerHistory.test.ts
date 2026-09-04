import { describe, expect, it } from "vitest";
import {
  HISTORY_CAP,
  canGoBack,
  canGoForward,
  crumbTrail,
  currentPath,
  goBack,
  goForward,
  initialHistory,
  parentOf,
  pushPath,
} from "./explorerHistory";

/** Walk a fresh history through a list of paths. */
const visit = (...paths: string[]) =>
  paths.reduce((h, p) => pushPath(h, p), initialHistory());

describe("explorer history", () => {
  it("starts at the root with nowhere to go", () => {
    const h = initialHistory();
    expect(currentPath(h)).toBe("");
    expect(canGoBack(h)).toBe(false);
    expect(canGoForward(h)).toBe(false);
  });

  it("walks forward and back over visited paths", () => {
    const h = visit("src", "src/lib");
    expect(currentPath(h)).toBe("src/lib");
    expect(canGoBack(h)).toBe(true);
    expect(canGoForward(h)).toBe(false);

    const back1 = goBack(h);
    expect(currentPath(back1)).toBe("src");
    expect(canGoForward(back1)).toBe(true);

    const back2 = goBack(back1);
    expect(currentPath(back2)).toBe("");
    expect(canGoBack(back2)).toBe(false);

    expect(currentPath(goForward(goForward(back2)))).toBe("src/lib");
  });

  it("drops the forward tail when you navigate somewhere new", () => {
    // …/src/lib, back to /src, then off to /web: "src/lib" must be gone, the
    // way a browser discards the branch you did not take.
    const h = pushPath(goBack(visit("src", "src/lib")), "web");
    expect(currentPath(h)).toBe("web");
    expect(canGoForward(h)).toBe(false);
    expect(h.stack).toEqual(["", "src", "web"]);
  });

  it("returns the SAME object when the path is already current", () => {
    // Identity matters: a redundant push must not re-render or refetch.
    const h = visit("src");
    expect(pushPath(h, "src")).toBe(h);
  });

  it("returns the same object at the ends of the range", () => {
    const root = initialHistory();
    expect(goBack(root)).toBe(root);
    expect(goForward(root)).toBe(root);
  });

  it("caps the stack by dropping the OLDEST entries, keeping the current one", () => {
    const many = Array.from({ length: HISTORY_CAP + 10 }, (_, i) => `d${i}`);
    const h = visit(...many);
    expect(h.stack.length).toBe(HISTORY_CAP);
    expect(h.index).toBe(HISTORY_CAP - 1);
    // Still showing the newest path, and the oldest are the ones gone.
    expect(currentPath(h)).toBe(`d${many.length - 1}`);
    expect(h.stack).not.toContain("");
    expect(h.stack).not.toContain("d0");
    // And back still works right after an overflow.
    expect(currentPath(goBack(h))).toBe(`d${many.length - 2}`);
  });

  it("treats the root as a normal history entry", () => {
    const h = visit("src", "");
    expect(currentPath(h)).toBe("");
    expect(canGoBack(h)).toBe(true);
    expect(currentPath(goBack(h))).toBe("src");
  });
});

describe("parentOf", () => {
  it("walks one level up, bottoming out at the root", () => {
    expect(parentOf("a/b/c")).toBe("a/b");
    expect(parentOf("a")).toBe("");
    expect(parentOf("")).toBe("");
  });
});

describe("crumbTrail", () => {
  it("is empty at the root", () => {
    expect(crumbTrail("")).toEqual({ crumbs: [], elided: false });
  });

  it("gives each segment its own cumulative path", () => {
    const { crumbs, elided } = crumbTrail("a/b/c");
    expect(elided).toBe(false);
    expect(crumbs.map((c) => [c.name, c.path])).toEqual([
      ["a", "a"],
      ["b", "a/b"],
      ["c", "a/b/c"],
    ]);
  });

  it("elides the MIDDLE, keeping the deepest segments", () => {
    const { crumbs, elided } = crumbTrail("a/b/c/d/e");
    expect(elided).toBe(true);
    expect(crumbs.map((c) => c.name)).toEqual(["c", "d", "e"]);
    // The kept crumbs still point at their real full paths, not at offsets
    // into the shortened list — clicking "c" must go to a/b/c.
    expect(crumbs.map((c) => c.path)).toEqual(["a/b/c", "a/b/c/d", "a/b/c/d/e"]);
  });

  it("does not elide exactly at the threshold", () => {
    expect(crumbTrail("a/b/c").elided).toBe(false);
    expect(crumbTrail("a/b/c/d").elided).toBe(true);
  });

  it("ignores empty segments from stray slashes", () => {
    expect(crumbTrail("a//b").crumbs.map((c) => c.name)).toEqual(["a", "b"]);
  });
});
