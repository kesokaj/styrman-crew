import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";
import { FONT_STACKS, type FontFamilyPref } from "../lib/fonts";
import { EditorView, basicSetup } from "codemirror";
import { Compartment, EditorState, StateEffect } from "@codemirror/state";
import { keymap } from "@codemirror/view";
import { indentWithTab } from "@codemirror/commands";
import {
  LanguageDescription,
  LanguageSupport,
  StreamLanguage,
} from "@codemirror/language";
import { oneDark } from "@codemirror/theme-one-dark";
import type { CheckDiagnostic } from "../lib/api";
import { linter, lintGutter, type Diagnostic } from "@codemirror/lint";

// Explicit grammar list instead of @codemirror/language-data: the full
// catalogue is 120+ lazy chunks (~2.5 MB) committed and go:embed-ed for
// languages nobody opens here. Each grammar still lazy-loads on first use.
const legacy = (load: Promise<{ [k: string]: unknown }>, name: string) =>
  load.then(
    (m) =>
      new LanguageSupport(
        StreamLanguage.define(
          m[name] as Parameters<typeof StreamLanguage.define>[0],
        ),
      ),
  );
const LANGUAGES = [
  LanguageDescription.of({
    name: "TypeScript",
    extensions: ["ts", "tsx", "js", "jsx", "mjs", "cjs"],
    load: () =>
      import("@codemirror/lang-javascript").then((m) =>
        m.javascript({ jsx: true, typescript: true }),
      ),
  }),
  LanguageDescription.of({
    name: "Go",
    extensions: ["go"],
    load: () => import("@codemirror/lang-go").then((m) => m.go()),
  }),
  LanguageDescription.of({
    name: "Java",
    extensions: ["java"],
    load: () => import("@codemirror/lang-java").then((m) => m.java()),
  }),
  LanguageDescription.of({
    name: "C/C++",
    extensions: ["c", "h", "cc", "cpp", "cxx", "hpp", "hh", "hxx"],
    load: () => import("@codemirror/lang-cpp").then((m) => m.cpp()),
  }),
  LanguageDescription.of({
    name: "Rust",
    extensions: ["rs"],
    load: () => import("@codemirror/lang-rust").then((m) => m.rust()),
  }),
  LanguageDescription.of({
    name: "PHP",
    extensions: ["php"],
    load: () => import("@codemirror/lang-php").then((m) => m.php()),
  }),
  LanguageDescription.of({
    name: "SQL",
    extensions: ["sql"],
    load: () => import("@codemirror/lang-sql").then((m) => m.sql()),
  }),
  LanguageDescription.of({
    name: "XML",
    extensions: ["xml", "svg", "xsd", "xsl", "plist"],
    load: () => import("@codemirror/lang-xml").then((m) => m.xml()),
  }),
  LanguageDescription.of({
    name: "Terraform",
    // HCL covers Terraform (.tf/.tfvars) and other HashiCorp configs (.hcl).
    extensions: ["tf", "tfvars", "hcl"],
    load: () => import("codemirror-lang-hcl").then((m) => m.hcl()),
  }),
  LanguageDescription.of({
    name: "Python",
    extensions: ["py"],
    load: () => import("@codemirror/lang-python").then((m) => m.python()),
  }),
  LanguageDescription.of({
    name: "JSON",
    extensions: ["json"],
    load: () => import("@codemirror/lang-json").then((m) => m.json()),
  }),
  LanguageDescription.of({
    name: "YAML",
    extensions: ["yaml", "yml"],
    load: () => import("@codemirror/lang-yaml").then((m) => m.yaml()),
  }),
  LanguageDescription.of({
    name: "Markdown",
    extensions: ["md", "markdown"],
    load: () => import("@codemirror/lang-markdown").then((m) => m.markdown()),
  }),
  LanguageDescription.of({
    name: "HTML",
    extensions: ["html", "htm"],
    load: () => import("@codemirror/lang-html").then((m) => m.html()),
  }),
  LanguageDescription.of({
    name: "CSS",
    extensions: ["css"],
    load: () => import("@codemirror/lang-css").then((m) => m.css()),
  }),
  LanguageDescription.of({
    name: "Shell",
    extensions: ["sh", "bash", "zsh"],
    filename: /^\.?(bashrc|profile|bash_\w+|zshrc)$/,
    load: () => legacy(import("@codemirror/legacy-modes/mode/shell"), "shell"),
  }),
  LanguageDescription.of({
    // Dockerfile, Dockerfile.dev, Containerfile, and the lowercase spellings.
    // `.dockerfile` as an extension is covered by the extensions list.
    name: "Dockerfile",
    extensions: ["dockerfile", "containerfile"],
    filename: /^(dockerfile|containerfile)(\..*)?$/i,
    load: () =>
      legacy(import("@codemirror/legacy-modes/mode/dockerfile"), "dockerFile"),
  }),
  LanguageDescription.of({
    name: "TOML",
    extensions: ["toml"],
    load: () => legacy(import("@codemirror/legacy-modes/mode/toml"), "toml"),
  }),
];

/** This editor's font before it was configurable, and still the fallback when
 *  a call site supplies no preference (the Settings credential viewer). Note
 *  it is 12px on the BARE SYSTEM STACK — deliberately not the terminal's
 *  13px/Google Sans Code — so an upgrade changes nobody's editor. */
export const DEFAULT_EDITOR_FONT_SIZE = 12;
export const DEFAULT_EDITOR_FONT_FAMILY: FontFamilyPref = "system";

/** The compartmentalised slice of the editor theme: exactly the two font
 *  rules, nothing else, so reconfiguring cannot disturb layout.
 *
 *  `family` is the UNION, resolved here rather than taken as a string:
 *  style-mod concatenates a theme value into a CSS rule with no escaping
 *  (`prop + ": " + value + ";"`), so a `;` or `}` in it would emit further
 *  declarations or close the rule outright. Only the three constants in
 *  `lib/fonts.ts` can reach it (security review 2026-09-04). */
const editorFontTheme = (size: number, family: FontFamilyPref) =>
  EditorView.theme({
    "&": { fontSize: `${size}px` },
    ".cm-scroller": { fontFamily: FONT_STACKS[family] },
  });

/** Imperative handle: replace the whole document (used by Format). */
export interface CodeEditorHandle {
  setContent: (text: string) => void;
}

/**
 * CodeMirror 6 editor for the file viewer: syntax highlighting (language
 * picked by filename, grammar lazy-loaded), line numbers, search (Ctrl+F),
 * bracket matching, tab indent, Ctrl+S to save — deliberately nothing more.
 */
const CodeEditor = forwardRef<
  CodeEditorHandle,
  {
    value: string;
    filename: string;
    dark: boolean;
    readOnly: boolean;
    /** Soft-wrap long lines instead of scrolling horizontally. */
    wrap: boolean;
    /**
     * Font size in px and CSS font stack for the editor surface.
     *
     * OPTIONAL, unlike `dark`/`wrap`, and defaulting to the values this
     * editor used before either was configurable. `settings.tsx`'s read-only
     * credential viewer is a second call site: it shows secrets, not code, so
     * it has no business following the user's *editor* font preference, and
     * optional props let it stay exactly as it was.
     */
    fontSize?: number;
    fontFamily?: FontFamilyPref;
    /**
     * Runs the language's real checker in the project VM and returns its
     * diagnostics. Omitted (e.g. the read-only Settings viewer) disables the
     * lint gutter entirely.
     */
    check?: (text: string) => Promise<CheckDiagnostic[]>;
    onChange: (text: string) => void;
    onSave: () => void;
  }
>(function CodeEditor(
  {
    value,
    filename,
    dark,
    readOnly,
    wrap,
    fontSize = DEFAULT_EDITOR_FONT_SIZE,
    fontFamily = DEFAULT_EDITOR_FONT_FAMILY,
    check,
    onChange,
    onSave,
  },
  ref,
) {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const themeComp = useRef(new Compartment());
  // Separate compartment so toggling wrap reconfigures in place — remounting
  // would lose cursor, scroll position, and undo history.
  const wrapComp = useRef(new Compartment());
  // Third compartment, same reason as wrap: a font change from Settings must
  // reconfigure an OPEN buffer in place. Remounting would lose cursor, scroll
  // position and undo history — and remount-on-filename-change (see the mount
  // effect) does not fire when only the preference changed, so it could not
  // serve as the mechanism even if that loss were acceptable.
  const fontComp = useRef(new Compartment());
  // Keep the latest check fn reachable from the mount-once linter extension.
  const checkRef = useRef(check);
  checkRef.current = check;
  // Refs so the mount-once editor always calls the latest callbacks.
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);

  // Replace the document from outside (Format). The updateListener fires with
  // docChanged, so onChange flows the new text back to the parent's state.
  useImperativeHandle(ref, () => ({
    setContent: (text: string) => {
      const v = viewRef.current;
      if (v) {
        v.dispatch({
          changes: { from: 0, to: v.state.doc.length, insert: text },
        });
      }
    },
  }), []);
  useEffect(() => {
    onChangeRef.current = onChange;
    onSaveRef.current = onSave;
  });

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const view = new EditorView({
      doc: value,
      parent: host,
      extensions: [
        basicSetup,
        keymap.of([
          {
            key: "Mod-s",
            run: () => {
              onSaveRef.current();
              return true;
            },
          },
          indentWithTab,
        ]),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChangeRef.current(u.state.doc.toString());
        }),
        EditorState.readOnly.of(readOnly),
        themeComp.current.of(dark ? oneDark : []),
        wrapComp.current.of(wrap ? EditorView.lineWrapping : []),
        // Syntax checking runs a real CLI checker inside the project VM, so it
        // is debounced hard (1s idle) rather than run per keystroke; CodeMirror
        // also cancels a pending run when the doc changes again.
        // Only for a file type the server can check (CHECKABLE_EXTENSIONS in
        // lib/api.ts — `check` is undefined otherwise): an empty gutter on an
        // uncheckable file reads as "no problems found".
        ...(check ? [lintGutter()] : []),
        ...(check ? [linter(
          async (view): Promise<Diagnostic[]> => {
            const fn = checkRef.current;
            if (!fn) return [];
            const doc = view.state.doc;
            const ds = await fn(doc.toString());
            return ds.map((d) => {
              // Clamp: a checker can report a line past the end (e.g. "expected
              // EOF"), and an out-of-range position throws inside CodeMirror.
              const lineNo = Math.min(Math.max(d.line, 1), doc.lines);
              const line = doc.line(lineNo);
              const from = Math.min(
                line.from + Math.max((d.col ?? 1) - 1, 0),
                line.to,
              );
              return {
                from,
                to: line.to,
                severity: d.severity,
                message: d.message,
              };
            });
          },
          { delay: 1000 },
        )] : []),
        // Layout stays static; ONLY the two font rules are compartmentalised,
        // so the reconfigure carries the smallest possible slice.
        EditorView.theme({
          "&": { height: "100%", backgroundColor: "transparent" },
        }),
        fontComp.current.of(editorFontTheme(fontSize, fontFamily)),
      ],
    });
    viewRef.current = view;
    // Match on the BASENAME, not the path we were handed. CodeMirror's
    // matchFilename tests each `filename` regex against the whole string, so
    // "projects/x/Dockerfile" never matched /^Dockerfile…/ — Dockerfiles and
    // shell dotfiles (.bashrc &c.) silently lost highlighting, while
    // extension-matched grammars kept working because matchFilename falls
    // back to splitting on the last dot.
    const base = filename.split("/").pop() ?? filename;
    const lang = LanguageDescription.matchFilename(LANGUAGES, base);
    lang?.load().then((support) => {
      view.dispatch({ effects: StateEffect.appendConfig.of(support) });
    });
    view.focus();
    return () => {
      viewRef.current = null;
      view.destroy();
    };
    // Mount once per file; content changes flow out via onChange only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filename, readOnly]);

  // Theme toggles while a file is open swap the highlight style in place.
  useEffect(() => {
    viewRef.current?.dispatch({
      effects: themeComp.current.reconfigure(dark ? oneDark : []),
    });
  }, [dark]);

  // Same for wrap — reconfigure, never remount.
  useEffect(() => {
    viewRef.current?.dispatch({
      effects: wrapComp.current.reconfigure(
        wrap ? EditorView.lineWrapping : [],
      ),
    });
  }, [wrap]);

  // Same for the font — reconfigure, never remount.
  useEffect(() => {
    viewRef.current?.dispatch({
      effects: fontComp.current.reconfigure(
        editorFontTheme(fontSize, fontFamily),
      ),
    });
  }, [fontSize, fontFamily]);

  return <div ref={hostRef} className="min-h-0 flex-1 overflow-hidden" />;
});

export default CodeEditor;
