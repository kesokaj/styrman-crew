/**
 * The monospace faces the UI can be set to, and the ONLY strings that are
 * allowed to reach a CSS font-family declaration.
 *
 * This lives in its own module, free of React and of xterm, so that both the
 * terminal (`lib/terminal.ts`) and the editor (`components/CodeEditor.tsx`)
 * can take a `FontFamilyPref` — a three-value union — rather than an
 * arbitrary string, and resolve it themselves.
 *
 * That indirection is the point (security review 2026-09-04): CodeMirror's
 * `EditorView.theme` hands its values to `style-mod`, which concatenates them
 * into a CSS rule with NO escaping, so a value containing `;` or `}` would
 * emit extra declarations or close the rule and open a new selector. Typing
 * the prop as a union means no caller can supply such a string in the first
 * place, instead of the guard resting on every call site remembering to look
 * the value up first. The component doing secrets display (the Settings
 * credential viewer) is one of those call sites.
 */
export type FontFamilyPref = "google-sans-code" | "jetbrains-mono" | "system";

/**
 * The first and third entries are VERBATIM the stacks these two surfaces used
 * before either was configurable — the terminal's six-part chain and the
 * editor's bare system stack. Do not "tidy" them: dropping a fallback changes
 * what renders for someone whose machine has that face, and the appearance
 * defaults promise that nothing changes until the user touches a control.
 *
 * `'JetBrains Mono'` (non-variable) inside the first stack is deliberate and
 * predates the bundled face; it resolves only for a user who has it installed
 * locally. The bundled face is a different family name entirely
 * (`'JetBrains Mono Variable'`), so the two never collide.
 */
export const FONT_STACKS: Record<FontFamilyPref, string> = {
  "google-sans-code":
    "'Google Sans Code Variable', 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
  "jetbrains-mono":
    "'JetBrains Mono Variable', ui-monospace, SFMono-Regular, Menlo, monospace",
  system: "ui-monospace, SFMono-Regular, Menlo, monospace",
};

/** Human labels for the family picker, in display order. */
export const FONT_FAMILY_LABELS: Array<[FontFamilyPref, string]> = [
  ["google-sans-code", "Google Sans Code"],
  ["jetbrains-mono", "JetBrains Mono"],
  ["system", "System monospace"],
];

export const FONT_SIZE_MIN = 8;
export const FONT_SIZE_MAX = 32;
