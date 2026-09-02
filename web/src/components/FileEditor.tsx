import {
  lazy,
  Suspense,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  api,
  tryUI,
  type CheckDiagnostic,
  type FileContent,
  type Scope,
} from "../lib/api";
import { useTheme } from "../lib/hooks";
import { toast } from "../lib/toast";
import { Button } from "./ui";

// CodeMirror is heavy — load it only when a file is actually opened.
const CodeEditor = lazy(() => import("./CodeEditor"));
import type { CodeEditorHandle } from "./CodeEditor";

// File extensions the server can format (its real CLI formatter runs in the
// project VM); JSON is also handled locally. Keep in sync with format.go.
const FORMATTABLE = new Set([
  "go",
  "tf",
  "tfvars",
  "hcl",
  "rs",
  "py",
  "sh",
  "bash",
  "js",
  "jsx",
  "ts",
  "tsx",
  "mjs",
  "cjs",
  "json",
  "css",
  "scss",
  "less",
  "html",
  "htm",
  "md",
  "markdown",
  "yaml",
  "yml",
  "vue",
  "graphql",
]);

const extOf = (p: string) => p.split(".").pop()?.toLowerCase() ?? "";

export const fmtSize = (n: number) =>
  n < 1024
    ? `${n} B`
    : n < 1024 * 1024
      ? `${(n / 1024).toFixed(1)} KB`
      : `${(n / 1024 / 1024).toFixed(1)} MB`;

// Soft wrap is a durable per-user preference, unlike which file is open.
const WRAP_KEY = "styrman-editor-wrap";

function loadWrap(): boolean {
  try {
    // Default ON: the operator asked for wrapping, and horizontal scrolling in
    // a narrow pane is the worse default.
    return localStorage.getItem(WRAP_KEY) !== "off";
  } catch {
    return true;
  }
}

/**
 * All editor behaviour for one open file — text, dirty tracking, save, format,
 * markdown preview, wrap. Split out from the component so the surface can be
 * mounted anywhere; EditorPane is the only mount point today (the modal file
 * viewer it replaced was deleted, 2026-09-01).
 */
export function useFileEditorState(
  scope: Scope,
  file: FileContent,
  onSaved: () => void,
) {
  const [text, setText] = useState(file.content);
  // The last SAVED content is the dirty baseline — so Save can leave the editor
  // open and the button correctly re-disables until the next edit.
  const [savedContent, setSavedContent] = useState(file.content);
  const [saving, setSaving] = useState(false);
  const [formatting, setFormatting] = useState(false);
  const [wrap, setWrapState] = useState(loadWrap);
  const editorRef = useRef<CodeEditorHandle>(null);
  const dirty = text !== savedContent;
  // ONE document identity — scope (kind AND project) + path — used both for
  // the render-phase reset below and to remount CodeEditor (it takes its
  // initial document from `value` only on mount). Including the project means
  // this file's correctness no longer rests on EditorPane's per-terminal key
  // living in another file (security review).
  const docIdentity = `${scope.kind}:${scope.project}:${file.path}`;

  const setWrap = (on: boolean) => {
    setWrapState(on);
    try {
      localStorage.setItem(WRAP_KEY, on ? "on" : "off");
    } catch {
      /* storage disabled — the toggle still works for this session */
    }
  };

  const isMarkdown = /\.(md|markdown)$/i.test(file.path);
  const isJSON = /\.json$/i.test(file.path);
  // Markdown opens in rendered preview (the common intent is READING a doc);
  // one click flips to the editor. micromark escapes raw HTML by default, so
  // the rendered output is safe without a sanitizer.
  const [previewing, setPreviewing] = useState(isMarkdown);
  const [previewHtml, setPreviewHtml] = useState<string>();

  // Reopening a different file in the SAME mounted pane must reset the text
  // and the dirty baseline. This is done DURING RENDER, not in an effect
  // (React's documented "adjusting state when props change" pattern), because
  // an effect runs too late: CodeEditor remounts on a filename change and
  // takes its initial document from `value` in that same render, so an
  // effect-based reset left the new file showing the PREVIOUS file's content
  // (caught by the e2e Dockerfile check, 2026-09-01).
  // Keyed on SCOPE + path, not path alone: the same relative path exists in
  // both the project and the home scope, and the Recent strip makes that
  // cross-scope open one click — keyed on path alone the other scope's buffer
  // survived and Ctrl+S wrote it over the wrong file (code review, 2026-09-02).
  // (Declared above the reset so the reset can clear it — the note is
  // per-file too, like preview mode.)
  const [checkNote, setCheckNote] = useState<string>();
  const [shownKey, setShownKey] = useState(docIdentity);
  if (shownKey !== docIdentity) {
    setShownKey(docIdentity);
    setCheckNote(undefined);
    setText(file.content);
    setSavedContent(file.content);
    // Preview mode is per-file too. The modal got this free by unmounting
    // between files; the pane stays mounted, so without this a .md opened
    // after a .go showed raw source instead of the rendered preview, and
    // previewHtml lingered from the previous file (review finding).
    setPreviewing(/\.(md|markdown)$/i.test(file.path));
    setPreviewHtml(undefined);
  }

  useEffect(() => {
    if (!previewing || !isMarkdown) return;
    let stale = false;
    import("micromark")
      .then(({ micromark }) => {
        if (stale) return;
        const html = micromark(text, { allowDangerousHtml: false }).replaceAll(
          "<a href=",
          '<a target="_blank" rel="noopener noreferrer" href=',
        );
        setPreviewHtml(html);
      })
      .catch(() => {
        // A failed chunk load must not leave "Rendering…" forever — fall back
        // to the plain editor view.
        if (!stale) setPreviewing(false);
      });
    return () => {
      stale = true;
    };
  }, [previewing, isMarkdown, text]);

  const canFormat =
    !file.binary && !file.truncated && FORMATTABLE.has(extOf(file.path));

  // Syntax check, run by CodeMirror's linter on idle. Failures are swallowed
  // to an empty result on purpose: the project VM may be stopped, or the file
  // type may have no checker, and neither is worth a toast every second. The
  // ONE thing it must never do is claim a clean file when it does not know —
  // an empty gutter reads as "no problems found", so a failed check returns
  // nothing rather than a fabricated pass, and the tool's own errors come back
  // as real diagnostics from the server.
  const runCheck = async (text: string): Promise<CheckDiagnostic[]> => {
    if (file.binary || file.truncated) return [];
    try {
      const r = await api.checkFile(scope.project, file.path, text);
      if (!r.supported) {
        setCheckNote(undefined);
        return [];
      }
      setCheckNote(
        r.unavailable ? `${r.tool ?? "checker"} not installed in this project` : undefined,
      );
      return r.diagnostics;
    } catch (e) {
      // NEVER present a failed check as a clean file: an empty gutter reads as
      // "no problems found". Say the check could not run instead (review
      // finding — this catch used to clear the note and return [], directly
      // contradicting the comment above it).
      const msg = (e as Error).message ?? "";
      setCheckNote(
        /start the project/i.test(msg)
          ? "syntax check needs the project running"
          : "syntax check unavailable",
      );
      return [];
    }
  };

  // JSON formats locally (instant, offline); every other type runs the real CLI
  // formatter (gofmt, prettier, terraform fmt, …) in the project VM. Both push
  // the result through the editor's imperative setContent so the VIEW updates
  // (and onChange flows it back to `text` → dirty).
  const doFormat = async () => {
    if (isJSON) {
      try {
        editorRef.current?.setContent(
          JSON.stringify(JSON.parse(text), null, 2) + "\n",
        );
      } catch (e) {
        toast("error", `Not valid JSON: ${(e as Error).message}`);
      }
      return;
    }
    setFormatting(true);
    try {
      const { content } = await api.formatFile(scope.project, file.path, text);
      editorRef.current?.setContent(content);
    } catch (e) {
      toast("error", (e as Error).message);
    } finally {
      setFormatting(false);
    }
  };

  const save = () => {
    if (saving || file.truncated) return; // truncated: saving would drop the tail
    return tryUI(async () => {
      setSaving(true);
      try {
        await api.saveFile(scope, file.path, text);
        setSavedContent(text); // new baseline; the editor stays open
        onSaved(); // refresh the listing (size/mtime)
        toast("success", "Saved");
      } finally {
        setSaving(false);
      }
    });
  };

  return {
    text,
    docIdentity,
    setText,
    dirty,
    saving,
    save,
    formatting,
    canFormat,
    doFormat,
    isJSON,
    isMarkdown,
    previewing,
    setPreviewing,
    previewHtml,
    editorRef,
    wrap,
    setWrap,
    runCheck,
    checkNote,
  };
}

export type FileEditorState = ReturnType<typeof useFileEditorState>;

/** The toolbar row: path, size, preview toggle, wrap, Format, Download, Save. */
export function FileEditorToolbar({
  scope,
  file,
  st,
  children,
}: {
  scope: Scope;
  file: FileContent;
  st: FileEditorState;
  /** Trailing controls owned by the wrapper (close ✕, view switch, …). */
  children?: ReactNode;
}) {
  return (
    <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs">
      <span data-testid="editor-path" className="truncate font-mono font-medium">
        {file.path}
      </span>
      <span className="shrink-0 text-text-dim">{fmtSize(file.size)}</span>
      {file.truncated && <span className="shrink-0 text-warn">truncated</span>}
      {st.checkNote && (
        <span className="min-w-0 truncate text-text-dim" title={st.checkNote}>
          {st.checkNote}
        </span>
      )}
      <div className="ml-auto flex shrink-0 items-center gap-1">
        {st.isMarkdown && !file.binary && (
          <div className="mr-1 inline-flex rounded-full bg-bg p-0.5">
            {(
              [
                [true, "Preview"],
                [false, "Edit"],
              ] as Array<[boolean, string]>
            ).map(([mode, label]) => (
              <button
                key={label}
                onClick={() => st.setPreviewing(mode)}
                className={`rounded-full px-2.5 py-0.5 ${
                  st.previewing === mode
                    ? "bg-accent-soft font-medium text-on-accent-soft"
                    : "text-text-dim hover:text-text"
                }`}
              >
                {label}
              </button>
            ))}
          </div>
        )}
        {!file.binary && !(st.isMarkdown && st.previewing) && (
          <button
            onClick={() => st.setWrap(!st.wrap)}
            aria-pressed={st.wrap}
            className={`inline-flex h-7 items-center rounded-lg px-2 hover:bg-bg-hover ${
              st.wrap ? "text-accent" : "text-text-dim hover:text-accent"
            }`}
            title={
              st.wrap
                ? "Word wrap on — click to scroll long lines instead"
                : "Word wrap off — click to wrap long lines"
            }
          >
            Wrap
          </button>
        )}
        {st.canFormat && (
          <button
            onClick={st.doFormat}
            disabled={st.formatting}
            className="inline-flex h-7 items-center rounded-lg px-2 text-text-dim hover:bg-bg-hover hover:text-accent disabled:opacity-40"
            title={
              st.isJSON
                ? "Pretty-print JSON (local); Save to keep it"
                : "Format with the language's formatter, run in the project VM; Save to keep it"
            }
          >
            {st.formatting ? "Formatting…" : "Format"}
          </button>
        )}
        <a
          href={api.downloadUrl(scope, file.path)}
          className="inline-flex h-7 items-center rounded-lg px-2 text-text-dim hover:bg-bg-hover hover:text-accent"
        >
          Download
        </a>
        {!file.binary && !file.truncated && (
          <Button
            variant="filled"
            onClick={st.save}
            disabled={!st.dirty || st.saving}
          >
            {st.saving ? "Saving…" : "Save"}
          </Button>
        )}
        {children}
      </div>
    </div>
  );
}

/** The body: binary notice, rendered markdown, or the CodeMirror editor. */
export function FileEditorBody({
  file,
  st,
}: {
  file: FileContent;
  st: FileEditorState;
}) {
  const { resolved } = useTheme();
  if (file.binary) {
    return (
      <p className="p-4 text-xs text-text-dim">
        Binary file — use Download to fetch it.
      </p>
    );
  }
  if (st.isMarkdown && st.previewing) {
    return (
      <div
        className="prose-md min-h-0 flex-1 overflow-y-auto px-6 py-4"
        // Safe: micromark escapes raw HTML (allowDangerousHtml: false).
        dangerouslySetInnerHTML={{
          __html: st.previewHtml ?? "<p>Rendering…</p>",
        }}
      />
    );
  }
  return (
    <Suspense
      fallback={<p className="p-4 text-xs text-text-dim">Loading editor…</p>}
    >
      <CodeEditor
        key={st.docIdentity}
        ref={st.editorRef}
        value={st.text}
        filename={file.path}
        dark={resolved === "dark"}
        readOnly={file.truncated}
        wrap={st.wrap}
        check={st.runCheck}
        onChange={st.setText}
        onSave={st.save}
      />
    </Suspense>
  );
}
