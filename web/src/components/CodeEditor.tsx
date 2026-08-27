import { useEffect, useRef } from "react";
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

// Explicit grammar list instead of @codemirror/language-data: the full
// catalogue is 120+ lazy chunks (~2.5 MB) committed and go:embed-ed for
// languages nobody opens here. Each grammar still lazy-loads on first use.
const legacy = (load: Promise<{ [k: string]: unknown }>, name: string) =>
  load.then(
    (m) =>
      new LanguageSupport(
        StreamLanguage.define(m[name] as Parameters<typeof StreamLanguage.define>[0]),
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
    name: "Dockerfile",
    filename: /^Dockerfile(\..*)?$/,
    load: () =>
      legacy(import("@codemirror/legacy-modes/mode/dockerfile"), "dockerFile"),
  }),
  LanguageDescription.of({
    name: "TOML",
    extensions: ["toml"],
    load: () => legacy(import("@codemirror/legacy-modes/mode/toml"), "toml"),
  }),
];

/**
 * CodeMirror 6 editor for the file viewer: syntax highlighting (language
 * picked by filename, grammar lazy-loaded), line numbers, search (Ctrl+F),
 * bracket matching, tab indent, Ctrl+S to save — deliberately nothing more.
 */
export default function CodeEditor({
  value,
  filename,
  dark,
  readOnly,
  onChange,
  onSave,
}: {
  value: string;
  filename: string;
  dark: boolean;
  readOnly: boolean;
  onChange: (text: string) => void;
  onSave: () => void;
}) {
  const hostRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const themeComp = useRef(new Compartment());
  // Refs so the mount-once editor always calls the latest callbacks.
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
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
        EditorView.theme({
          "&": {
            height: "100%",
            fontSize: "12px",
            backgroundColor: "transparent",
          },
          ".cm-scroller": {
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
          },
        }),
      ],
    });
    viewRef.current = view;
    const lang = LanguageDescription.matchFilename(LANGUAGES, filename);
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

  return <div ref={hostRef} className="min-h-0 flex-1 overflow-hidden" />;
}
