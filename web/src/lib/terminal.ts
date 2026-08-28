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
// - URL extraction from the buffer (extractUrls) and the raw output stream
//   (sniffChunk) — built for the removed v1.2 link bar; currently unused at
//   runtime (nothing calls sniffChunk) but kept as a documented capability.

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
  /** Extract http(s) URLs from the last `rows` buffer lines, newest last.
   *  Soft-wrapped lines are joined first, so long URLs survive wrapping. */
  extractUrls(rows?: number): string[];
  /** Add a keydown handler; return false to swallow the event. xterm allows
   *  only ONE custom key handler, so this module owns it and chains. */
  addKeyHandler(h: (ev: KeyboardEvent) => boolean): void;
  /** Feed a raw output chunk so hard-wrapped URLs can be sniffed from the
   *  stream (call from the WebSocket onmessage, before term.write). */
  sniffChunk(data: Uint8Array): void;
  dispose(): void;
}

export interface TerminalOptions {
  /** Called whenever text lands on the clipboard (copy-on-select or
   *  Ctrl+C), so the UI can confirm it — vital feedback in a web app. */
  onCopy?: (text: string) => void;
  /** Start with the dark palette (default true). Swap later via
   *  `term.options.theme = terminalTheme(dark)`. */
  dark?: boolean;
}

export function createTerminal(
  host: HTMLElement,
  opts: TerminalOptions = {},
): StyrmanTerminal {
  const term = new Terminal({
    fontSize: 13,
    fontFamily:
      "'Google Sans Code Variable', 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
    cursorBlink: true,
    scrollback: 10000,
    allowProposedApi: true, // unicode11 + search decorations
    theme: terminalTheme(opts.dark ?? true),
  });

  const fit = new FitAddon();
  const search = new SearchAddon();
  term.loadAddon(fit);
  term.loadAddon(search);
  term.loadAddon(new ClipboardAddon());
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

  // Clipboard keys. ev.preventDefault() is required — xterm discards this
  // handler's return before its own preventDefault, so without it the
  // browser's native paste fires too (double paste). Falls back to the
  // native path when the async clipboard API is unavailable. ev.code keeps
  // this keyboard-layout-stable.
  keyHandlers.push((ev) => {
    if (ev.type !== "keydown") return true;
    if (!(ev.ctrlKey || ev.metaKey)) return true;
    if (ev.code === "KeyC") {
      if (!term.hasSelection()) return true; // SIGINT / browser default
      if (typeof navigator.clipboard?.writeText !== "function") return true;
      ev.preventDefault();
      copyText(term.getSelection());
      term.clearSelection();
      return false;
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

  // Stream sniffer: TUIs sometimes HARD-wrap a URL on screen (each fragment
  // its own line — buffer joining can't help), but the URL usually arrives
  // contiguous in the output stream. Feed raw chunks here; escape sequences
  // become separators (a cursor move mid-URL genuinely breaks contiguity),
  // and a rolling tail handles URLs split across chunk boundaries.
  const ansiRe =
    // eslint-disable-next-line no-control-regex
    /\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-B0-9]|\x1b[=>]|[\x00-\x08\x0b\x0c\x0e-\x1f]/g;
  // Styled hard-wrap continuation: TUIs (agy) print long URLs as separate
  // styled lines, and tmux re-renders that as "…frag ESC(B ESC[m ESC[K CRLF
  // indent ESC[34m ESC[4m frag…" (style/charset/erase codes around the
  // newline — captured from a real client stream). A newline bracketed by
  // escape runs on BOTH sides is a styled-wrap marker; collapsing it rejoins
  // the URL. The lookahead refuses to glue two distinct URLs, and prose
  // damaged by the join doesn't matter — only URLs are extracted.
  const wrapSeq = "(?:\\x1b\\[[0-9;]*[mK]|\\x1b\\([A-B0-9])";
  const styledWrapRe = new RegExp(
    `(?:${wrapSeq})+[\\r\\n]+[ \\t]*(?:${wrapSeq})+(?!https?://)`,
    "g",
  );
  const decoder = new TextDecoder();
  let rawTail = "";
  const streamUrls: string[] = [];
  const sniffChunk = (data: Uint8Array) => {
    // Keep the RAW tail (escapes intact) so a wrap marker split across two
    // chunks is still seen whole on the next scan.
    rawTail = (rawTail + decoder.decode(data, { stream: true })).slice(-8192);
    const text = rawTail.replace(styledWrapRe, "").replace(ansiRe, " ");
    for (const m of text.matchAll(/https?:\/\/[^\s"'`<>()[\]{}|\\^]+/g)) {
      const url = m[0].replace(/[.,;:!?]+$/, "");
      if (url.length > 10) {
        const i = streamUrls.indexOf(url);
        if (i !== -1) streamUrls.splice(i, 1);
        streamUrls.push(url);
        if (streamUrls.length > 8) streamUrls.shift();
      }
    }
  };

  const extractUrls = (rows = 400): string[] => {
    const buf = term.buffer.active;
    const end = buf.length;
    const start = Math.max(0, end - rows);
    // Join soft-wrapped rows into logical lines (isWrapped marks rows that
    // continue the previous one) — this is what makes wrapped URLs whole.
    const logical: string[] = [];
    for (let i = start; i < end; i++) {
      const line = buf.getLine(i);
      if (!line) continue;
      const text = line.translateToString(true);
      if (line.isWrapped && logical.length > 0) {
        logical[logical.length - 1] += text;
      } else {
        logical.push(text);
      }
    }
    const found: string[] = [];
    const re = /https?:\/\/[^\s"'`<>()[\]{}|\\^]+/g;
    for (const line of logical) {
      for (const m of line.matchAll(re)) {
        // Trim trailing punctuation that's prose, not URL.
        const url = m[0].replace(/[.,;:!?]+$/, "");
        if (url.length > 10 && !found.includes(url)) found.push(url);
      }
    }
    // Merge in stream-sniffed URLs (they survive hard-wrapping), then drop
    // truncated variants: a URL that is a strict prefix of another, or that
    // shares its first 80 chars with a LONGER one, is a partial render of
    // the same link (TUIs re-render at several widths) — keep the longest.
    for (const u of streamUrls) {
      if (!found.includes(u)) found.push(u);
    }
    return found.filter(
      (u) =>
        !found.some(
          (v) =>
            v !== u &&
            (v.startsWith(u) ||
              (u.length > 80 &&
                v.length > u.length &&
                v.slice(0, 80) === u.slice(0, 80))),
        ),
    );
  };

  return {
    term,
    fit,
    search,
    extractUrls,
    addKeyHandler,
    sniffChunk,
    dispose: () => {
      clearTimeout(selTimer); // don't fire copy-on-select after teardown (N2)
      term.dispose();
    },
  };
}
