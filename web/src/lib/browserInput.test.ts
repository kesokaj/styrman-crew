import { describe, expect, it } from "vitest";
import {
  isChord,
  keyParams,
  modifiersOf,
  mouseParams,
  normalizeUrl,
  pointOf,
  wheelParams,
} from "./browserInput";

const noMods = { altKey: false, ctrlKey: false, metaKey: false, shiftKey: false };
const rect = { left: 100, top: 50, width: 640, height: 400 };
const frame = { deviceWidth: 1280, deviceHeight: 800 };

describe("pointOf", () => {
  it("scales canvas coordinates into the frame and clamps", () => {
    expect(pointOf(100, 50, rect, frame)).toEqual({ x: 0, y: 0 });
    expect(pointOf(420, 250, rect, frame)).toEqual({ x: 640, y: 400 });
    // Past the edge: clamped inside the frame, never negative.
    expect(pointOf(2000, 2000, rect, frame)).toEqual({ x: 1279, y: 799 });
    expect(pointOf(-5, -5, rect, frame)).toEqual({ x: 0, y: 0 });
  });

  it("survives a zero-size box", () => {
    expect(pointOf(10, 10, { left: 0, top: 0, width: 0, height: 0 }, frame)).toEqual({ x: 10, y: 10 });
  });
});

describe("modifiersOf", () => {
  it("builds CDP's bitmask", () => {
    expect(modifiersOf(noMods)).toBe(0);
    expect(modifiersOf({ ...noMods, altKey: true })).toBe(1);
    expect(modifiersOf({ ...noMods, ctrlKey: true, shiftKey: true })).toBe(10);
    expect(modifiersOf({ ...noMods, metaKey: true })).toBe(4);
  });
});

describe("mouseParams", () => {
  it("maps a click with its button and click count", () => {
    const p = mouseParams(
      { ...noMods, type: "mousedown", button: 0, buttons: 1, clientX: 420, clientY: 250, detail: 2 },
      rect,
      frame,
    );
    expect(p).toEqual({
      type: "mousePressed", x: 640, y: 400, button: "left", buttons: 1, clickCount: 2, modifiers: 0,
    });
    expect(mouseParams({ ...noMods, type: "mouseup", button: 2, clientX: 100, clientY: 50, detail: 1 }, rect, frame))
      .toMatchObject({ type: "mouseReleased", button: "right", clickCount: 1 });
  });

  it("reports the held button on a move, or none", () => {
    const drag = mouseParams(
      { ...noMods, type: "mousemove", button: 0, buttons: 1, clientX: 200, clientY: 100, detail: 0 },
      rect,
      frame,
    );
    expect(drag).toMatchObject({ type: "mouseMoved", button: "left" });
    expect(drag.clickCount).toBeUndefined();
    const hover = mouseParams(
      { ...noMods, type: "mousemove", button: 0, buttons: 0, clientX: 200, clientY: 100, detail: 0 },
      rect,
      frame,
    );
    expect(hover.button).toBe("none");
  });

  it("caps the click count at a triple click", () => {
    expect(mouseParams({ ...noMods, type: "mousedown", button: 0, clientX: 100, clientY: 50, detail: 9 }, rect, frame).clickCount).toBe(3);
  });
});

describe("wheelParams", () => {
  it("passes pixel deltas and scales line deltas", () => {
    expect(wheelParams({ ...noMods, deltaX: 0, deltaY: 120, clientX: 100, clientY: 50 }, rect, frame))
      .toEqual({ type: "mouseWheel", x: 0, y: 0, button: "none", modifiers: 0, deltaX: 0, deltaY: 120 });
    expect(wheelParams({ ...noMods, deltaX: 0, deltaY: 3, deltaMode: 1, clientX: 100, clientY: 50 }, rect, frame).deltaY).toBe(48);
  });
});

describe("keyParams", () => {
  it("carries text for printable keys and Enter on keyDown only", () => {
    expect(keyParams({ ...noMods, type: "keydown", key: "a", code: "KeyA", keyCode: 65 }))
      .toMatchObject({ type: "keyDown", key: "a", code: "KeyA", windowsVirtualKeyCode: 65, text: "a", unmodifiedText: "a" });
    expect(keyParams({ ...noMods, type: "keyup", key: "a", code: "KeyA", keyCode: 65 }).text).toBeUndefined();
    expect(keyParams({ ...noMods, type: "keydown", key: "Enter", code: "Enter", keyCode: 13 }).text).toBe("\r");
    expect(keyParams({ ...noMods, type: "keydown", key: "Backspace", code: "Backspace", keyCode: 8 }).text).toBeUndefined();
    // A non-BMP character is one key, not two code units.
    expect(keyParams({ ...noMods, type: "keydown", key: "😀", code: "", keyCode: 0 }).text).toBe("😀");
  });

  it("does not insert the letter of a Ctrl/Meta chord", () => {
    const p = keyParams({ ...noMods, ctrlKey: true, type: "keydown", key: "a", code: "KeyA", keyCode: 65 });
    expect(p.modifiers).toBe(2);
    expect(p.text).toBeUndefined();
  });

  it("inserts an AltGr (Ctrl+Alt) character — that is typing, not a chord", () => {
    const p = keyParams({ ...noMods, ctrlKey: true, altKey: true, type: "keydown", key: "@", code: "Digit2", keyCode: 50 });
    expect(p.modifiers).toBe(3);
    expect(p.text).toBe("@");
    expect(isChord({ ...noMods, ctrlKey: true, altKey: true })).toBe(false);
    expect(isChord({ ...noMods, ctrlKey: true })).toBe(true);
    expect(isChord({ ...noMods, metaKey: true })).toBe(true);
    expect(isChord({ ...noMods, altKey: true })).toBe(false);
  });

  it("marks keypad keys", () => {
    expect(keyParams({ ...noMods, type: "keydown", key: "1", code: "Numpad1", keyCode: 97, location: 3 }))
      .toMatchObject({ location: 3, isKeypad: true, text: "1" });
  });
});

describe("normalizeUrl", () => {
  it("expands the in-guest shortcuts and adds a scheme", () => {
    expect(normalizeUrl("")).toBeNull();
    expect(normalizeUrl("   ")).toBeNull();
    expect(normalizeUrl(":3000")).toBe("http://localhost:3000");
    expect(normalizeUrl(":3000/api")).toBe("http://localhost:3000/api");
    expect(normalizeUrl("localhost:8080/x")).toBe("http://localhost:8080/x");
    expect(normalizeUrl("example.com")).toBe("http://example.com");
    expect(normalizeUrl(" https://example.com/a?b=c ")).toBe("https://example.com/a?b=c");
    expect(normalizeUrl("about:blank")).toBe("about:blank");
  });

  it("refuses every scheme but http, https and about:blank", () => {
    for (const bad of [
      "javascript:alert(1)",
      "javascript://%0aalert(1)",
      "data:text/html,<b>x</b>",
      "file:///etc/passwd",
      "view-source:http://localhost:3000/",
      "blob:http://x/y",
      "ftp://example.com/",
      "about:settings",
    ]) {
      expect(normalizeUrl(bad), bad).toBeNull();
    }
    // Host:port shapes are still hosts.
    expect(normalizeUrl("127.0.0.1:9000/x")).toBe("http://127.0.0.1:9000/x");
  });
});
