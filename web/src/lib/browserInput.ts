/**
 * Pure mappings from DOM input events to CDP `Input.*` parameters, plus the
 * URL bar's normalisation, for the Browser view (ADR-068). Kept free of the
 * DOM so the coordinate scaling and key mapping are unit-tested.
 */

/** The screencast frame's size in CSS pixels of the remote page. */
export type FrameGeom = { deviceWidth: number; deviceHeight: number };
/** Where the canvas sits on this page (a DOMRect, or the parts we use). */
export type RectLike = { left: number; top: number; width: number; height: number };

type ModifierKeys = { altKey: boolean; ctrlKey: boolean; metaKey: boolean; shiftKey: boolean };

/** CDP's modifier bitmask: 1 Alt, 2 Ctrl, 4 Meta, 8 Shift. */
export function modifiersOf(e: ModifierKeys): number {
  return (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0);
}

/**
 * Canvas-relative client coordinates → remote page coordinates. The canvas
 * is drawn at the frame's size scaled to fit its box, so the ratio is the
 * frame's width over the box's width (same for height); the result is
 * rounded and clamped into the frame.
 */
export function pointOf(
  clientX: number,
  clientY: number,
  rect: RectLike,
  frame: FrameGeom,
): { x: number; y: number } {
  const sx = rect.width > 0 ? frame.deviceWidth / rect.width : 1;
  const sy = rect.height > 0 ? frame.deviceHeight / rect.height : 1;
  const clamp = (v: number, max: number) => Math.min(Math.max(Math.round(v), 0), Math.max(max - 1, 0));
  return {
    x: clamp((clientX - rect.left) * sx, frame.deviceWidth),
    y: clamp((clientY - rect.top) * sy, frame.deviceHeight),
  };
}

export type MouseButton = "none" | "left" | "middle" | "right" | "back" | "forward";

const BUTTONS: MouseButton[] = ["left", "middle", "right", "back", "forward"];

export type MouseEventLike = ModifierKeys & {
  type: "mousedown" | "mouseup" | "mousemove";
  button: number;
  buttons?: number;
  clientX: number;
  clientY: number;
  detail: number;
};

export type InputMouseParams = {
  type: "mousePressed" | "mouseReleased" | "mouseMoved" | "mouseWheel";
  x: number;
  y: number;
  button: MouseButton;
  buttons?: number;
  clickCount?: number;
  modifiers: number;
  deltaX?: number;
  deltaY?: number;
};

/** `Input.dispatchMouseEvent` params for a mouse button or move event. */
export function mouseParams(e: MouseEventLike, rect: RectLike, frame: FrameGeom): InputMouseParams {
  const { x, y } = pointOf(e.clientX, e.clientY, rect, frame);
  const type =
    e.type === "mousedown" ? "mousePressed" : e.type === "mouseup" ? "mouseReleased" : "mouseMoved";
  // On a move the DOM's `button` is meaningless (always 0); CDP wants "none"
  // unless a button is held, which `buttons` tells us.
  const held = e.buttons ?? 0;
  const button: MouseButton =
    type === "mouseMoved"
      ? held & 1
        ? "left"
        : held & 4
          ? "middle"
          : held & 2
            ? "right"
            : "none"
      : (BUTTONS[e.button] ?? "none");
  const p: InputMouseParams = { type, x, y, button, modifiers: modifiersOf(e) };
  if (type !== "mouseMoved") p.clickCount = Math.max(1, Math.min(e.detail || 1, 3));
  if (e.buttons !== undefined) p.buttons = e.buttons;
  return p;
}

export type WheelEventLike = ModifierKeys & {
  deltaX: number;
  deltaY: number;
  deltaMode?: number;
  clientX: number;
  clientY: number;
};

/** `Input.dispatchMouseEvent` params of type mouseWheel. Line/page deltas are scaled to pixels. */
export function wheelParams(e: WheelEventLike, rect: RectLike, frame: FrameGeom): InputMouseParams {
  const { x, y } = pointOf(e.clientX, e.clientY, rect, frame);
  const unit = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? rect.height || 800 : 1;
  return {
    type: "mouseWheel",
    x,
    y,
    button: "none",
    modifiers: modifiersOf(e),
    deltaX: Math.round(e.deltaX * unit),
    deltaY: Math.round(e.deltaY * unit),
  };
}

export type KeyEventLike = ModifierKeys & {
  type: "keydown" | "keyup";
  key: string;
  code: string;
  keyCode: number;
  location?: number;
};

export type InputKeyParams = {
  type: "keyDown" | "keyUp";
  key: string;
  code: string;
  windowsVirtualKeyCode: number;
  nativeVirtualKeyCode: number;
  modifiers: number;
  text?: string;
  unmodifiedText?: string;
  location?: number;
  isKeypad?: boolean;
};

/**
 * `Input.dispatchKeyEvent` params. A keyDown of a printable key carries the
 * character as `text`, which is what makes Chromium insert it; Enter carries
 * "\r" for the same reason; everything else (arrows, Backspace, modifiers
 * alone) is a bare key event. keyUp never carries text.
 */
export function keyParams(e: KeyEventLike): InputKeyParams {
  const mods = modifiersOf(e);
  const p: InputKeyParams = {
    type: e.type === "keydown" ? "keyDown" : "keyUp",
    key: e.key,
    code: e.code,
    windowsVirtualKeyCode: e.keyCode,
    nativeVirtualKeyCode: e.keyCode,
    modifiers: mods,
  };
  if (e.location) {
    p.location = e.location;
    if (e.location === 3) p.isKeypad = true;
  }
  if (p.type === "keyDown") {
    // Ctrl/Meta chords (Ctrl+A, Cmd+C) must not insert their letter.
    const chord = e.ctrlKey || e.metaKey;
    if (e.key === "Enter") {
      p.text = "\r";
      p.unmodifiedText = "\r";
    } else if (!chord && [...e.key].length === 1) {
      p.text = e.key;
      p.unmodifiedText = e.key;
    }
  }
  return p;
}

/**
 * What the URL bar's input means, or null for nothing. Shortcuts for the
 * common in-guest case: `:3000` and `localhost:3000/x` become http URLs; a
 * bare host gets `http://`; anything with a scheme is taken as typed.
 */
export function normalizeUrl(input: string): string | null {
  const s = input.trim();
  if (s === "") return null;
  if (/^:\d{1,5}(\/.*)?$/.test(s)) return `http://localhost${s}`;
  // A scheme is `xxx://` (hierarchical) or one of the few schemes a page can
  // sensibly be — NOT `localhost:8080`, whose "scheme" is a host and port.
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(s) || /^(about|data|blob|file|view-source):/i.test(s)) return s;
  return `http://${s}`;
}
