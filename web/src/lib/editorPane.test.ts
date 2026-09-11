import { describe, expect, it } from "vitest";
import {
  __stateForTest,
  basename,
  clearBrowserRequest,
  discardDraft,
  openFileInPane,
  openInBrowser,
} from "./editorPane";

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

// Discard means discard: the parked buffer and the dirty flag go, the open
// file stays (the pane just shows it as saved on disk next time).
describe("discardDraft", () => {
  it("clears the draft and the dirty flag but keeps the file", async () => {
    const mod = await import("./editorPane");
    const project = "p-discard";
    const term = "t1";
    openFileInPane(project, term, { kind: "project", project }, "a.go");
    // useEditorPane is a hook; drive the store through the same setters the
    // pane uses by reaching its internal state via a second open + inspect.
    const before = mod.__stateForTest?.(project, term);
    expect(before?.file?.path).toBe("a.go");
    discardDraft(project, term);
    const after = mod.__stateForTest?.(project, term);
    expect(after?.dirty).toBe(false);
    expect(after?.draft).toBeNull();
    expect(after?.file?.path).toBe("a.go");
  });
});

describe("openInBrowser", () => {
  it("switches the view and carries a fresh request object each time", () => {
    openInBrowser("p", "t", "http://localhost:3000/");
    const first = __stateForTest("p", "t");
    expect(first.view).toBe("browser");
    expect(first.browser).toEqual({ url: "http://localhost:3000/" });
    // The SAME url again is a new request: the store's identity guard must
    // see a change, or a second "Open in Browser" is a silent no-op.
    openInBrowser("p", "t", "http://localhost:3000/");
    const second = __stateForTest("p", "t");
    expect(second.browser).not.toBe(first.browser);
    expect(second.browser).toEqual(first.browser);
  });

  it("keeps the open file and its draft while switching", () => {
    openFileInPane("p2", "t", { kind: "project", project: "p2" }, "a.txt");
    openInBrowser("p2", "t", "http://localhost:8080/");
    const st = __stateForTest("p2", "t");
    expect(st.view).toBe("browser");
    expect(st.file?.path).toBe("a.txt");
  });

  it("clearBrowserRequest drops the request and nothing else", () => {
    openInBrowser("p3", "t", "http://localhost:1/");
    clearBrowserRequest("p3", "t");
    const st = __stateForTest("p3", "t");
    expect(st.browser).toBeNull();
    expect(st.view).toBe("browser");
    // Idempotent: a second clear is a no-op (same state object).
    clearBrowserRequest("p3", "t");
    expect(__stateForTest("p3", "t")).toBe(st);
  });
});
