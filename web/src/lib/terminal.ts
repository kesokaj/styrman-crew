// The styrman terminal: one place that assembles a fully-equipped xterm.js
// (the same engine VS Code embeds) instead of scattering terminal features
// through components. Everything terminal-generic lives here; TerminalPane
// only wires the WebSocket protocol and React lifecycle around it.
//
// Equipment:
// - DOM renderer (deliberately not WebGL — see createTerminal)
// - clickable URLs (web-links addon)
// - OSC 52 clipboard (TUIs and tmux can write the system clipboard)
// - search API (Ctrl+Shift+F overlay in TerminalPane)
// - unicode 11 widths (emoji don't break box-drawing)
// - Ctrl/Cmd+C copy-when-selected (else SIGINT), Ctrl/Cmd+V paste, plus
//   copy-on-select (clipboard writes the instant a selection is highlighted)

import { FONT_STACKS, type FontFamilyPref } from "./fonts";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { ClipboardAddon } from "@xterm/addon-clipboard";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { addClip } from "./clips";

// Paired ANSI palettes (operator Q2): one per app theme, M3-tinted so the
// pane matches the chrome. background mirrors --term-bg in index.css —
// change them together. A single palette can't hold contrast on both
// grounds; every color here is picked against its own background.
const DARK_THEME = {
  background: "#101216", // --term-bg dark
  foreground: "#e2e4e9",
  cursor: "#a8c7fa",
  cursorAccent: "#101216",
  selectionBackground: "#2e4468",
  black: "#3c4043",
  red: "#f28b82",
  green: "#81c995",
  yellow: "#fdd663",
  blue: "#8ab4f8",
  magenta: "#d7aefb",
  cyan: "#78d9ec",
  white: "#bdc1c6",
  brightBlack: "#7d818a",
  brightRed: "#f6aea9",
  brightGreen: "#a8dab5",
  brightYellow: "#fde293",
  brightBlue: "#aecbfa",
  brightMagenta: "#e9d2fd",
  brightCyan: "#a1e4f2",
  brightWhite: "#f1f3f4",
};

const LIGHT_THEME = {
  background: "#f6f8fb", // --term-bg light
  foreground: "#1b1d22",
  cursor: "#185abc",
  cursorAccent: "#f6f8fb",
  selectionBackground: "#cfe0f8",
  black: "#20242b",
  red: "#c5221f",
  green: "#137333",
  yellow: "#915f00",
  blue: "#185abc",
  magenta: "#7c2bbf",
  cyan: "#036e77",
  white: "#8f96a3",
  brightBlack: "#5c6370",
  brightRed: "#a50e0e",
  brightGreen: "#0d652d",
  brightYellow: "#7a5000",
  brightBlue: "#174ea6",
  brightMagenta: "#681da8",
  brightCyan: "#005a60",
  brightWhite: "#b8bec9",
};

/** The xterm theme for the app's resolved color scheme. */
export function terminalTheme(dark: boolean) {
  return dark ? DARK_THEME : LIGHT_THEME;
}

export interface StyrmanTerminal {
  term: Terminal;
  fit: FitAddon;
  search: SearchAddon;
  /** Add a keydown handler; return false to swallow the event. xterm allows
   *  only ONE custom key handler, so this module owns it and chains. */
  addKeyHandler(h: (ev: KeyboardEvent) => boolean): void;
  dispose(): void;
}

export interface TerminalOptions {
  /** Called whenever text lands on the clipboard (copy-on-select or
   *  Ctrl+C), so the UI can confirm it — vital feedback in a web app. */
  onCopy?: (text: string) => void;
  /** Start with the dark palette (default true). Swap later via
   *  `term.options.theme = terminalTheme(dark)`. */
  dark?: boolean;
  /** Initial font size in px. Optional — omitting it keeps the historical
   *  13px, so no call site is forced to care. Change it later by assigning
   *  `term.options.fontSize`; see TerminalPane for the re-measure the change
   *  requires. */
  fontSize?: number;
  /** Initial font family. A PREFERENCE, not a raw stack: resolution to a CSS
   *  string happens in here, so no caller can hand this an arbitrary string
   *  (security review 2026-09-04). */
  fontFamily?: FontFamilyPref;
}

/** The terminal's historical font, still the default when no preference is
 *  supplied. There is now exactly ONE copy of the stack itself, in
 *  `lib/fonts.ts`, so nothing can drift apart from it. */
export const DEFAULT_TERM_FONT_SIZE = 13;
export const DEFAULT_TERM_FONT_FAMILY: FontFamilyPref = "google-sans-code";

export function createTerminal(
  host: HTMLElement,
  opts: TerminalOptions = {},
): StyrmanTerminal {
  const term = new Terminal({
    fontSize: opts.fontSize ?? DEFAULT_TERM_FONT_SIZE,
    fontFamily: FONT_STACKS[opts.fontFamily ?? DEFAULT_TERM_FONT_FAMILY],
    cursorBlink: true,
    scrollback: 10000,
    allowProposedApi: true, // unicode11 + search decorations
    theme: terminalTheme(opts.dark ?? true),
  });

  const fit = new FitAddon();
  const search = new SearchAddon();
  term.loadAddon(fit);
  term.loadAddon(search);
  // OSC 52 clipboard, WRITE-ONLY. The default provider also implements read,
  // so a `\x1b]52;c;?` query from the (untrusted) guest would exfiltrate the
  // user's system clipboard — passwords, tokens copied from the credential
  // viewer — into the PTY. Deny read (return empty); allow write only for the
  // system selection so TUIs (tmux/vim yank) can still copy (H8).
  term.loadAddon(
    new ClipboardAddon(undefined, {
      readText: () => "",
      writeText: (selection: string, text: string) =>
        selection === "c" &&
        typeof navigator.clipboard?.writeText === "function"
          ? navigator.clipboard.writeText(text)
          : Promise.resolve(),
    }),
  );
  term.loadAddon(new Unicode11Addon());
  term.unicode.activeVersion = "11";
  term.loadAddon(
    new WebLinksAddon((event, uri) => {
      event.preventDefault();
      window.open(uri, "_blank", "noopener,noreferrer");
    }),
  );

  term.open(host);
  // Deliberately the DOM renderer, not WebGL: text stays real DOM
  // (selectable, assertable, screen-reader-visible) and our terminals never
  // hit the throughput where the GPU renderer matters.

  const copyText = (text: string) => {
    if (!text || typeof navigator.clipboard?.writeText !== "function") return;
    navigator.clipboard
      .writeText(text)
      .then(() => {
        addClip(text); // clipboard history (side panel)
        opts.onCopy?.(text);
      })
      .catch(() => {});
  };

  // Copy-on-select: capture the selection to the clipboard the moment the
  // mouse releases. Waiting for an explicit Ctrl+C is a losing race — TUIs
  // (claude, agy) redraw constantly and wipe the selection first. This is
  // how native terminals (PuTTY, most Linux terminals) behave.
  // Hook xterm's own selection events, not DOM mouseup: the text is
  // captured the instant it is highlighted, and the last non-empty
  // selection is copied once it settles. Even if a redraw wipes the
  // highlight mid-drag, the captured text still lands on the clipboard.
  let lastSelection = "";
  let selTimer: ReturnType<typeof setTimeout> | undefined;
  term.onSelectionChange(() => {
    const s = term.getSelection();
    if (!s.trim()) return; // clicks clear selection — never copy emptiness
    lastSelection = s;
    clearTimeout(selTimer);
    selTimer = setTimeout(() => copyText(lastSelection), 250);
  });

  // xterm supports exactly one custom key handler — this module owns it and
  // chains extra handlers registered via addKeyHandler (first false wins).
  const keyHandlers: Array<(ev: KeyboardEvent) => boolean> = [];
  term.attachCustomKeyEventHandler((ev) => {
    for (const h of keyHandlers) {
      if (h(ev) === false) return false;
    }
    return true;
  });

  // Typing @ $ { } [ ] \ € etc. on Nordic and other non-US layouts uses a
  // "compose" modifier that yields a special character rather than a Meta/ESC
  // sequence. xterm otherwise mistakes it for Meta and sends ESC+key, which
  // readline renders as "(arg: 2)" and never yields the "@". The modifier
  // differs by platform:
  //   - Windows/Linux: AltGr (Right Alt) — reported as the AltGraph modifier
  //     state, or in Chrome as ctrlKey+altKey together. Left-Alt-only is real
  //     Meta (readline Alt+b) and stays untouched.
  //   - mac: Option (Left or Right Alt) composes, because we keep
  //     macOptionIsMeta off so Option types symbols (Swedish @ = Option+2).
  // When the OS has already composed a printable single character into ev.key,
  // type exactly that and stop xterm. The length-1 guard skips Alt/Option+Arrow
  // word-nav, F-keys and dead keys, so those keep their Meta behavior.
  const isMac =
    typeof navigator !== "undefined" &&
    /Mac|iPhone|iPad|iPod/i.test(
      navigator.platform || navigator.userAgent || "",
    );
  const altGrOf = (ev: KeyboardEvent) =>
    (typeof ev.getModifierState === "function" &&
      ev.getModifierState("AltGraph")) ||
    (ev.ctrlKey && ev.altKey) ||
    (isMac && ev.altKey && !ev.ctrlKey);

  // Dead keys on Nordic/Swedish (ISO) layouts: ~ ` ^ ´ ¨ aren't typed directly —
  // you press the diacritic key, then Space for the bare char (or a vowel to
  // accent it). xterm routes these through the browser IME, which is buggy for
  // these layouts (xterm.js #2151/#2479, both open), so ~ ` ^ come out mangled.
  // We intercept: capture the pending diacritic on the dead keydown (bypassing
  // xterm's composition) and resolve it ourselves on the next key.
  const MODIFIER_KEYS = new Set([
    "Shift",
    "Control",
    "Alt",
    "AltGraph",
    "Meta",
    "CapsLock",
    "NumLock",
    "ScrollLock",
  ]);
  // Precomposed letters keyed by the resolved spacing diacritic.
  const DEAD_COMPOSE: Record<string, Record<string, string>> = {
    "~": { a: "ã", n: "ñ", o: "õ", A: "Ã", N: "Ñ", O: "Õ" },
    "^": {
      a: "â",
      e: "ê",
      i: "î",
      o: "ô",
      u: "û",
      A: "Â",
      E: "Ê",
      I: "Î",
      O: "Ô",
      U: "Û",
    },
    "`": {
      a: "à",
      e: "è",
      i: "ì",
      o: "ò",
      u: "ù",
      A: "À",
      E: "È",
      I: "Ì",
      O: "Ò",
      U: "Ù",
    },
    "´": {
      a: "á",
      e: "é",
      i: "í",
      o: "ó",
      u: "ú",
      y: "ý",
      A: "Á",
      E: "É",
      I: "Í",
      O: "Ó",
      U: "Ú",
      Y: "Ý",
    },
    "¨": {
      a: "ä",
      e: "ë",
      i: "ï",
      o: "ö",
      u: "ü",
      y: "ÿ",
      A: "Ä",
      E: "Ë",
      I: "Ï",
      O: "Ö",
      U: "Ü",
    },
  };
  // Which spacing diacritic a dead keydown means, by physical key + modifiers.
  // ev.code is layout-independent (US-QWERTY position); on a Nordic ISO board
  // the ¨/^/~ key sits where US ] is (BracketRight) and ´/` where US = is (Equal).
  const deadDiacritic = (ev: KeyboardEvent): string | null => {
    const altGr = altGrOf(ev);
    switch (ev.code) {
      case "BracketRight":
        if (altGr) return "~";
        if (ev.shiftKey) return "^";
        return "¨"; // ¨
      case "Equal":
        if (ev.shiftKey) return "`";
        if (!altGr) return "´"; // ´
        return null;
      default:
        return null;
    }
  };
  let pendingDead: string | null = null;
  keyHandlers.push((ev) => {
    if (ev.type !== "keydown") return true;
    // Resolve a pending dead key with this keystroke.
    if (pendingDead !== null) {
      if (MODIFIER_KEYS.has(ev.key)) return true; // wait for the base key
      const dead = pendingDead;
      pendingDead = null;
      if (ev.code === "Space") {
        ev.preventDefault();
        term.input(dead);
        return false;
      }
      const composed = DEAD_COMPOSE[dead]?.[ev.key];
      if (composed) {
        ev.preventDefault();
        term.input(composed);
        return false;
      }
      // Not composable — emit the bare diacritic, then let this key proceed
      // (it may itself begin a new dead sequence, handled just below).
      term.input(dead);
    }
    if (ev.key === "Dead") {
      const dead = deadDiacritic(ev);
      if (dead !== null) {
        ev.preventDefault();
        pendingDead = dead;
        return false;
      }
      // Unknown dead key for this layout — let xterm try (no regression).
    }
    return true;
  });

  // Typing @ $ { } [ ] \ € etc. on Nordic and other non-US layouts uses a
  // "compose" modifier that yields a special character rather than a Meta/ESC
  // sequence. xterm otherwise mistakes it for Meta and sends ESC+key, which
  // readline renders as "(arg: 2)" and never yields the "@". The modifier
  // differs by platform:
  //   - Windows/Linux: AltGr (Right Alt) — reported as the AltGraph modifier
  //     state, or in Chrome as ctrlKey+altKey together. Left-Alt-only is real
  //     Meta (readline Alt+b) and stays untouched.
  //   - mac: Option (Left or Right Alt) composes, because we keep
  //     macOptionIsMeta off so Option types symbols (Swedish @ = Option+2).
  // When the OS has already composed a printable single character into ev.key,
  // type exactly that and stop xterm. The length-1 guard skips Alt/Option+Arrow
  // word-nav, F-keys and dead keys, so those keep their Meta behavior.
  keyHandlers.push((ev) => {
    if (ev.type !== "keydown" || ev.metaKey) return true;
    if (
      altGrOf(ev) &&
      ev.key &&
      ev.key.length === 1 &&
      (ev.key.codePointAt(0) ?? 0) >= 0x20
    ) {
      ev.preventDefault();
      term.input(ev.key);
      return false;
    }
    return true;
  });

  // Clipboard + interrupt keys. ev.preventDefault() is required — xterm discards
  // this handler's return before its own preventDefault, so without it the
  // browser's native copy/paste fires too. Falls back to the native path when
  // the async clipboard API is unavailable. ev.code keeps this layout-stable.
  keyHandlers.push((ev) => {
    if (ev.type !== "keydown") return true;
    if (!(ev.ctrlKey || ev.metaKey)) return true;
    if (ev.code === "KeyC") {
      // Ctrl+Shift+C copies the selection (terminals reserve plain Ctrl+C for
      // SIGINT); Cmd+C (metaKey) copies too. Copy-on-select already mirrors
      // selections to the clipboard, so this is the explicit path.
      if ((ev.ctrlKey && ev.shiftKey) || (ev.metaKey && !ev.ctrlKey)) {
        if (!term.hasSelection()) return true;
        if (typeof navigator.clipboard?.writeText !== "function") return true;
        ev.preventDefault();
        copyText(term.getSelection());
        term.clearSelection();
        return false;
      }
      // Plain Ctrl+C MUST interrupt (SIGINT). A lingering copy-on-select
      // highlight let the BROWSER copy and swallow the keydown, so ^C never
      // reached the shell (couldn't kill e.g. ping). Send ETX (\x03) ourselves
      // and preventDefault so nothing — browser or xterm default — intercepts.
      if (ev.ctrlKey && !ev.altKey) {
        ev.preventDefault();
        term.input("\x03");
        if (term.hasSelection()) term.clearSelection();
        return false;
      }
      return true;
    }
    if (ev.code === "KeyV") {
      if (typeof navigator.clipboard?.readText !== "function") return true;
      ev.preventDefault();
      navigator.clipboard
        .readText()
        .then((t) => t && term.paste(t))
        .catch(() => {});
      return false;
    }
    return true;
  });

  const addKeyHandler = (h: (ev: KeyboardEvent) => boolean) => {
    keyHandlers.push(h);
  };

  return {
    term,
    fit,
    search,
    addKeyHandler,
    dispose: () => {
      clearTimeout(selTimer); // don't fire copy-on-select after teardown (N2)
      term.dispose();
    },
  };
}
