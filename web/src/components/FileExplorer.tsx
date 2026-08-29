import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  api,
  tryUI,
  type FileContent,
  type FileEntry,
  type Scope,
} from "../lib/api";
import { useTheme } from "../lib/hooks";
import { toast } from "../lib/toast";
import { Button, ConfirmDialog, IconButton, PromptDialog } from "./ui";
import {
  ChevronRightIcon,
  DownloadIcon,
  FileIcon,
  FilePlusIcon,
  FolderIcon,
  FolderPlusIcon,
  PencilIcon,
  RefreshIcon,
  TrashIcon,
  UploadIcon,
  XIcon,
} from "./icons";

// CodeMirror is heavy — load it only when a file is actually opened.
const CodeEditor = lazy(() => import("./CodeEditor"));

const fmtSize = (n: number) =>
  n < 1024
    ? `${n} B`
    : n < 1024 * 1024
      ? `${(n / 1024).toFixed(0)} KB`
      : `${(n / 1048576).toFixed(1)} MB`;

type ExplorerDialog =
  | { kind: "newFile" }
  | { kind: "newFolder" }
  | { kind: "rename"; entry: FileEntry }
  | { kind: "delete"; entry: FileEntry };

/**
 * File explorer over a project's workspace or the shared home. The server
 * reads these paths directly (the home is mounted into it), so browsing
 * works even while the project's microVM is stopped.
 */
export function FileExplorer({
  project,
  projectName,
}: {
  project: string;
  /** Friendly name shown as the crumb root (falls back to the UUID). */
  projectName?: string;
}) {
  const [scope, setScope] = useState<Scope>({ kind: "project", project });
  const [path, setPath] = useState("");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [error, setError] = useState<string>();
  const [open, setOpen] = useState<FileContent | null>(null);
  const [dragging, setDragging] = useState(false);
  const [dialog, setDialog] = useState<ExplorerDialog | null>(null);

  // Switching project resets the view to that project's root.
  useEffect(() => {
    setScope({ kind: "project", project });
    setPath("");
  }, [project]);

  // Generation guard (audit finding): quick directory hops could let an
  // EARLIER listing resolve last and repaint the previous directory's entries
  // under the new breadcrumb. Each (scope,path) change invalidates in-flight
  // responses.
  const loadGen = useRef(0);
  const load = useCallback(() => {
    const gen = ++loadGen.current;
    api
      .listFiles(scope, path)
      .then((r) => {
        if (gen !== loadGen.current) return; // stale response
        setEntries(r.entries);
        setError(undefined);
      })
      .catch((e: Error) => {
        if (gen === loadGen.current) setError(e.message);
      });
  }, [scope, path]);

  useEffect(() => {
    load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, [load]);

  const crumbs = path ? path.split("/") : [];

  // A file/dir name typed by the user; rejects separators and dot-navigation.
  const nameError = (v: string): string | undefined => {
    if (!v || v === "." || v === "..") return "Enter a name";
    if (v.includes("/")) return "Names cannot contain /";
    return undefined;
  };

  const inDir = (name: string) => (path ? `${path}/${name}` : name);

  // The open editor must not survive its file being renamed/deleted — Save
  // on the stale path would recreate what the user just removed.
  const closeIfOpen = (entryPath: string) => {
    setOpen((o) =>
      o && (o.path === entryPath || o.path.startsWith(entryPath + "/"))
        ? null
        : o,
    );
  };

  const createFile = (name: string) =>
    tryUI(async () => {
      const p = inDir(name);
      await api.saveFile(scope, p, "");
      load();
      setOpen(await api.readFile(scope, p));
    });

  const createFolder = (name: string) =>
    tryUI(async () => {
      await api.makeDir(scope, inDir(name));
      load();
    });

  const renameEntry = (entry: FileEntry, name: string) => {
    if (name === entry.name) return;
    const dir = entry.path.includes("/")
      ? entry.path.slice(0, entry.path.lastIndexOf("/"))
      : "";
    tryUI(async () => {
      await api.renameFile(scope, entry.path, dir ? `${dir}/${name}` : name);
      closeIfOpen(entry.path);
      load();
    });
  };

  const deleteEntry = (entry: FileEntry) =>
    tryUI(async () => {
      await api.deleteFile(scope, entry.path);
      closeIfOpen(entry.path);
      load();
    });

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-1 border-b border-border px-2 py-1.5 text-xs">
        {(
          [
            ["project", "Project"],
            ["home", "Home"],
          ] as const
        ).map(([k, label]) => (
          <button
            key={k}
            onClick={() => {
              setScope(
                k === "home"
                  ? { kind: "home", project }
                  : { kind: "project", project },
              );
              setPath("");
            }}
            className={`rounded-full px-2.5 py-1 ${
              scope.kind === k
                ? "bg-accent-soft font-medium text-on-accent-soft"
                : "text-text-dim hover:bg-bg-hover"
            }`}
          >
            {label}
          </button>
        ))}
        <div className="ml-auto flex items-center">
          <IconButton
            label="New file"
            onClick={() => setDialog({ kind: "newFile" })}
          >
            <FilePlusIcon />
          </IconButton>
          <IconButton
            label="New folder"
            onClick={() => setDialog({ kind: "newFolder" })}
          >
            <FolderPlusIcon />
          </IconButton>
          <label
            className="inline-flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-lg text-text-dim transition-colors hover:bg-bg-hover hover:text-text"
            title="Upload files"
            aria-label="Upload files"
          >
            <UploadIcon />
            <input
              type="file"
              multiple
              className="hidden"
              onChange={(e) => {
                const list = Array.from(e.target.files ?? []);
                if (list.length)
                  tryUI(async () => {
                    await api.uploadFiles(scope, path, list);
                    load();
                  });
                e.target.value = "";
              }}
            />
          </label>
          <IconButton label="Refresh" onClick={load}>
            <RefreshIcon />
          </IconButton>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-0.5 px-2 py-1 text-[11px] text-text-dim">
        <button
          className="hover:text-text"
          onClick={() => setPath("")}
          title={scope.kind === "home" ? "Home" : `Project ID ${project}`}
        >
          {scope.kind === "home" ? "~" : (projectName ?? project)}
        </button>
        {crumbs.map((c, i) => (
          <span key={i} className="flex items-center gap-0.5">
            <ChevronRightIcon className="h-3 w-3 opacity-60" />
            <button
              className="hover:text-text"
              onClick={() => setPath(crumbs.slice(0, i + 1).join("/"))}
            >
              {c}
            </button>
          </span>
        ))}
      </div>

      <div
        className={`min-h-0 flex-1 overflow-y-auto px-1 pb-2 ${dragging ? "bg-accent-soft/30 outline-dashed outline-1 outline-accent" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          const list = Array.from(e.dataTransfer.files ?? []);
          if (list.length)
            tryUI(async () => {
              await api.uploadFiles(scope, path, list);
              load();
            });
        }}
      >
        {error && <p className="px-2 py-2 text-xs text-danger">{error}</p>}
        {!error && entries.length === 0 && (
          <p className="px-2 py-2 text-xs text-text-dim">Empty directory</p>
        )}
        {path !== "" && (
          <button
            className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-xs text-text-dim hover:bg-bg-hover"
            onClick={() => setPath(crumbs.slice(0, -1).join("/"))}
          >
            ../
          </button>
        )}
        {entries.map((e) => (
          <div
            key={e.path}
            className="group flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-xs hover:bg-bg-hover"
            onClick={() =>
              e.dir
                ? setPath(e.path)
                : tryUI(async () => setOpen(await api.readFile(scope, e.path)))
            }
          >
            <span className="shrink-0 text-text-dim">
              {e.dir ? (
                <FolderIcon className="h-3.5 w-3.5" />
              ) : (
                <FileIcon className="h-3.5 w-3.5" />
              )}
            </span>
            <span
              className={`min-w-0 flex-1 truncate ${e.dir ? "text-text" : "text-text-dim"}`}
            >
              {e.name}
              {e.symlink && <span className="text-accent"> ↗</span>}
            </span>
            {!e.dir && (
              <span className="shrink-0 text-[10px] text-text-dim">
                {fmtSize(e.size)}
              </span>
            )}
            <span className="invisible flex shrink-0 items-center group-hover:visible">
              {!e.dir && (
                <a
                  href={api.downloadUrl(scope, e.path)}
                  onClick={(ev) => ev.stopPropagation()}
                  className="inline-flex h-6 w-6 items-center justify-center rounded-md text-text-dim hover:bg-bg-active hover:text-accent"
                  title="Download"
                  aria-label={`Download ${e.name}`}
                >
                  <DownloadIcon className="h-3.5 w-3.5" />
                </a>
              )}
              <button
                onClick={(ev) => {
                  ev.stopPropagation();
                  setDialog({ kind: "rename", entry: e });
                }}
                className="inline-flex h-6 w-6 items-center justify-center rounded-md text-text-dim hover:bg-bg-active hover:text-text"
                title="Rename"
                aria-label={`Rename ${e.name}`}
              >
                <PencilIcon className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={(ev) => {
                  ev.stopPropagation();
                  setDialog({ kind: "delete", entry: e });
                }}
                className="inline-flex h-6 w-6 items-center justify-center rounded-md text-text-dim hover:bg-danger-soft hover:text-danger"
                title="Delete"
                aria-label={`Delete ${e.name}`}
              >
                <TrashIcon className="h-3.5 w-3.5" />
              </button>
            </span>
          </div>
        ))}
      </div>

      {dialog?.kind === "newFile" && (
        <PromptDialog
          title="New file"
          label="Created in the current directory."
          submitLabel="Create"
          validate={nameError}
          onSubmit={createFile}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "newFolder" && (
        <PromptDialog
          title="New folder"
          label="Created in the current directory."
          submitLabel="Create"
          validate={nameError}
          onSubmit={createFolder}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "rename" && (
        <PromptDialog
          title={`Rename “${dialog.entry.name}”`}
          initial={dialog.entry.name}
          submitLabel="Rename"
          validate={nameError}
          onSubmit={(name) => renameEntry(dialog.entry, name)}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "delete" && (
        <ConfirmDialog
          title={`Delete “${dialog.entry.name}”?`}
          body={
            dialog.entry.dir
              ? "The folder and everything inside it are deleted. This cannot be undone from here (checkpoints may still hold it)."
              : "This cannot be undone from here (checkpoints may still hold it)."
          }
          confirmLabel="Delete"
          danger
          onConfirm={() => deleteEntry(dialog.entry)}
          onClose={() => setDialog(null)}
        />
      )}
      {open && (
        <FileViewer
          scope={scope}
          file={open}
          onClose={() => setOpen(null)}
          onSaved={load}
        />
      )}
    </div>
  );
}

function FileViewer({
  scope,
  file,
  onClose,
  onSaved,
}: {
  scope: Scope;
  file: FileContent;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [text, setText] = useState(file.content);
  // The last SAVED content is the dirty baseline — so Save can leave the editor
  // open (it no longer closes) and the button correctly re-disables until the
  // next edit.
  const [savedContent, setSavedContent] = useState(file.content);
  const [saving, setSaving] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const { resolved } = useTheme();
  const modalRef = useRef<HTMLDivElement>(null);
  const dirty = text !== savedContent;

  // The window is resizable (CSS resize handle, bottom-right); remember the
  // size across opens. Wrapped in try/catch — storage can be unavailable.
  useEffect(() => {
    const el = modalRef.current;
    if (!el) return;
    try {
      const raw = localStorage.getItem("styrman-editor-size");
      if (raw) {
        const s = JSON.parse(raw);
        if (s.w) el.style.width = `${s.w}px`;
        if (s.h) el.style.height = `${s.h}px`;
      }
    } catch {
      /* no stored size */
    }
    const ro = new ResizeObserver(() => {
      try {
        localStorage.setItem(
          "styrman-editor-size",
          JSON.stringify({ w: el.offsetWidth, h: el.offsetHeight }),
        );
      } catch {
        /* storage disabled */
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const isMarkdown = /\.(md|markdown)$/i.test(file.path);
  const isJSON = /\.json$/i.test(file.path);
  // Markdown opens in rendered preview (the common intent is READING a doc);
  // one click flips to the editor. micromark escapes raw HTML by default, so
  // the rendered output is safe without a sanitizer.
  const [previewing, setPreviewing] = useState(isMarkdown);
  const [previewHtml, setPreviewHtml] = useState<string>();
  useEffect(() => {
    if (!previewing || !isMarkdown) return;
    let stale = false;
    import("micromark")
      .then(({ micromark }) => {
        if (stale) return;
        // micromark escapes raw HTML and sanitizes javascript: hrefs; the
        // target/rel rewrite just keeps README links from navigating the SPA.
        const html = micromark(text, { allowDangerousHtml: false }).replaceAll(
          "<a href=",
          '<a target="_blank" rel="noopener noreferrer" href=',
        );
        setPreviewHtml(html);
      })
      .catch(() => {
        // A failed chunk load must not leave "Rendering…" forever (audit
        // finding) — fall back to the plain editor view.
        if (!stale) setPreviewing(false);
      });
    return () => {
      stale = true;
    };
  }, [previewing, isMarkdown, text]);

  const formatJSON = () => {
    try {
      setText(JSON.stringify(JSON.parse(text), null, 2) + "\n");
    } catch (e) {
      toast("error", `Not valid JSON: ${(e as Error).message}`);
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

  // Don't silently drop edits when clicking the backdrop or ✕.
  const tryClose = () => {
    if (!dirty) onClose();
    else setConfirmDiscard(true);
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 md:p-8"
      onClick={tryClose}
    >
      <div
        ref={modalRef}
        className="flex max-h-[95vh] max-w-[97vw] flex-col overflow-hidden rounded-2xl bg-bg-raised shadow-2xl"
        style={{
          width: "min(56rem, 92vw)",
          height: "80vh",
          resize: "both",
          minWidth: "22rem",
          minHeight: "14rem",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs">
          <span className="truncate font-mono font-medium">{file.path}</span>
          <span className="text-text-dim">{fmtSize(file.size)}</span>
          {file.truncated && <span className="text-warn">truncated</span>}
          <div className="ml-auto flex items-center gap-1">
            {isMarkdown && !file.binary && (
              <div className="mr-1 inline-flex rounded-full bg-bg p-0.5">
                {(
                  [
                    [true, "Preview"],
                    [false, "Edit"],
                  ] as Array<[boolean, string]>
                ).map(([mode, label]) => (
                  <button
                    key={label}
                    onClick={() => setPreviewing(mode)}
                    className={`rounded-full px-2.5 py-0.5 ${
                      previewing === mode
                        ? "bg-accent-soft font-medium text-on-accent-soft"
                        : "text-text-dim hover:text-text"
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            )}
            {isJSON && !file.binary && !file.truncated && (
              <button
                onClick={formatJSON}
                className="inline-flex h-7 items-center rounded-lg px-2 text-text-dim hover:bg-bg-hover hover:text-accent"
                title="Pretty-print (2-space indent); Save to keep it"
              >
                Format
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
                onClick={save}
                disabled={!dirty || saving}
              >
                {saving ? "Saving…" : "Save"}
              </Button>
            )}
            <IconButton label="Close" onClick={tryClose}>
              <XIcon />
            </IconButton>
          </div>
        </div>
        {file.binary ? (
          <p className="p-4 text-xs text-text-dim">
            Binary file — use Download to fetch it.
          </p>
        ) : isMarkdown && previewing ? (
          <div
            className="prose-md min-h-0 flex-1 overflow-y-auto px-6 py-4"
            // Safe: micromark escapes raw HTML (allowDangerousHtml: false).
            dangerouslySetInnerHTML={{
              __html: previewHtml ?? "<p>Rendering…</p>",
            }}
          />
        ) : (
          <Suspense
            fallback={
              <p className="p-4 text-xs text-text-dim">Loading editor…</p>
            }
          >
            <CodeEditor
              value={text}
              filename={file.path}
              dark={resolved === "dark"}
              readOnly={file.truncated}
              onChange={setText}
              onSave={save}
            />
          </Suspense>
        )}
      </div>
      {confirmDiscard && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          body="The edits in this file are lost."
          confirmLabel="Discard"
          danger
          onConfirm={onClose}
          onClose={() => setConfirmDiscard(false)}
        />
      )}
    </div>
  );
}
