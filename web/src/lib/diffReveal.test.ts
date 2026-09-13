import { describe, expect, it } from "vitest";
import { reveal } from "./diffReveal";

// The diff pane exists so a change can be reviewed BEFORE it is committed.
// A line that renders differently from how it applies defeats that entirely,
// so the invisible characters that can reorder or hide text are made visible
// rather than silently passed through (security review 2026-09-04).

describe("reveal", () => {
  it("leaves ordinary diff content untouched", () => {
    expect(reveal("+const x = 1;")).toBe("+const x = 1;");
    expect(reveal("-  removed")).toBe("-  removed");
    expect(reveal("@@ -1,2 +1,3 @@")).toBe("@@ -1,2 +1,3 @@");
    expect(reveal("")).toBe("");
  });

  it("keeps TAB — it is real, meaningful diff content", () => {
    expect(reveal("+\tindented")).toBe("+\tindented");
  });

  it("reveals the bidi overrides behind Trojan Source", () => {
    // RLO can render a line's tail reversed, so what a reviewer reads is not
    // what git applies.
    expect(reveal("+ if (admin) {\u202e")).toBe("+ if (admin) {<U+202E>");
    for (const c of ["\u202a", "\u202b", "\u202c", "\u202d", "\u202e"])
      expect(reveal(`x${c}y`)).toContain("<U+20");
  });

  it("reveals the bidi ISOLATES too, not just the overrides", () => {
    for (const c of ["\u2066", "\u2067", "\u2068", "\u2069"])
      expect(reveal(`x${c}y`)).not.toBe(`x${c}y`);
  });

  it("reveals the zero-width family and the soft hyphen (audit 2026-09-13)", () => {
    for (const c of ["\u200b", "\u200c", "\u200d", "\u2060", "\ufeff", "\u00ad"])
      expect(reveal(`ab${c}cd`)).toMatch(/^ab<U\+[0-9A-F]{4}>cd$/);
  });

  it("reveals invisible directional marks", () => {
    expect(reveal("a\u200eb")).toBe("a<U+200E>b");
    expect(reveal("a\u200fb")).toBe("a<U+200F>b");
    expect(reveal("a\u061cb")).toBe("a<U+061C>b");
  });

  it("reveals C0 and C1 control bytes", () => {
    expect(reveal("a\u0000b")).toBe("a<U+0000>b");
    expect(reveal("a\u0007b")).toBe("a<U+0007>b");
    expect(reveal("a\u001bb")).toBe("a<U+001B>b"); // ESC, i.e. an ANSI escape
    expect(reveal("a\u007fb")).toBe("a<U+007F>b");
    expect(reveal("a\u009bb")).toBe("a<U+009B>b");
  });

  it("reveals rather than DROPS — a removed byte is its own lie", () => {
    const out = reveal("a\u202eb");
    expect(out).not.toBe("ab");
    expect(out.length).toBeGreaterThan("ab".length);
  });

  it("reveals every occurrence, not only the first", () => {
    expect(reveal("\u202ea\u202eb")).toBe("<U+202E>a<U+202E>b");
  });
});
