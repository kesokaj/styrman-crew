import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import {
  api,
  tryUI,
  type FileContent,
  type FileEntry,
  type Scope,
} from "../lib/api";
import { useTheme } from "../lib/hooks";
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
export function FileExplorer({ project }: { project: string }) {
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

  const load = useCallback(() => {
    api
      .listFiles(scope, path)
      .then((r) => {
        setEntries(r.entries);
        setError(undefined);
      })
      .catch((e: Error) => setError(e.message));
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
                k === "home" ? { kind: "home" } : { kind: "project", project },
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
        <button className="hover:text-text" onClick={() => setPath("")}>
          {scope.kind === "home" ? "~" : project}
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
  const [saving, setSaving] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const { resolved } = useTheme();
  const dirty = text !== file.content;

  const save = () => {
    if (saving || file.truncated) return; // truncated: saving would drop the tail
    return tryUI(async () => {
      setSaving(true);
      try {
        await api.saveFile(scope, file.path, text);
        onSaved();
        onClose();
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
        className="flex h-full w-full max-w-4xl flex-col rounded-2xl bg-bg-raised shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs">
          <span className="truncate font-mono font-medium">{file.path}</span>
          <span className="text-text-dim">{fmtSize(file.size)}</span>
          {file.truncated && <span className="text-warn">truncated</span>}
          <div className="ml-auto flex items-center gap-1">
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
