/**
 * The diff pane exists so a change can be reviewed BEFORE it is committed. A
 * line that renders differently from how it applies defeats that, so the
 * characters that can reorder or hide text are made VISIBLE rather than
 * dropped (a silently removed byte is its own kind of lie about the file):
 * C0/C1 controls, the bidi overrides/isolates/marks (Trojan Source, security
 * review 2026-09-04), and the zero-width family — ZWSP, ZWNJ, ZWJ, WORD
 * JOINER and the BOM used as one — which hides a homoglyph or splits an
 * identifier without any bidi at all (audit 2026-09-13). Tab is kept: it is
 * real, meaningful diff content.
 */
// eslint-disable-next-line no-control-regex
const INVISIBLE = /[\u0000-\u0008\u000B-\u001F\u007F-\u009F\u00AD\u061C\u200B-\u200F\u202A-\u202E\u2060\u2066-\u2069\uFEFF]/g;

export const reveal = (line: string) =>
  line.replace(INVISIBLE, (c) => `<U+${c.codePointAt(0)!.toString(16).toUpperCase().padStart(4, "0")}>`);
