import { describe, expect, it } from "vitest";
import { basename } from "./editorPane";

// NOTE ON SCOPE: the pane store's per-terminal behaviour — draft parked on
// unmount, restored only for the file it belongs to, dropped when that file is
// closed or evicted — is NOT unit-tested here. Its state lives in a
// module-level Map that is deliberately not exported, and reaching it would
// mean either widening the API for tests or pulling in a React renderer. It is
// covered by the browser e2e tier instead. Asserting `true === true` around
// those calls would have looked like coverage and been worth nothing.
//
// basename IS pure and exported, so it gets real tests.
describe("basename", () => {
  it("takes the last path segment", () => {
    expect(basename("a/b/c.txt")).toBe("c.txt");
    expect(basename("c.txt")).toBe("c.txt");
    expect(basename("/abs/path/f.go")).toBe("f.go");
  });

  it("handles the edges without throwing", () => {
    expect(basename("")).toBe("");
    expect(basename("a/b/")).toBe("");
    expect(basename("/")).toBe("");
  });

  it("does not treat backslashes as separators (POSIX guest paths)", () => {
    expect(basename("a\\b.txt")).toBe("a\\b.txt");
  });
});
