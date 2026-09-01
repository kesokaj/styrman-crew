import { useCallback, useEffect, useRef, useState } from "react";
import { api, tryUI, type FileEntry, type Scope } from "../lib/api";
import {
  closeFileInPane,
  openFileInPane,
  useFileSavedTick,
} from "../lib/editorPane";
import { ConfirmDialog, IconButton, PromptDialog } from "./ui";
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
} from "./icons";

import { fmtSize } from "./FileEditor";

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
  term,
}: {
  project: string;
  /** Friendly name shown as the crumb root (falls back to the UUID). */
  projectName?: string;
  /**
   * The terminal this explorer is docked next to. Files open in THAT
   * terminal's editor pane (operator request 2026-09-01). Required: SidePanel
   * is the only host, and the modal viewer this replaced is gone.
   */
  term: string;
}) {
  const [scope, setScope] = useState<Scope>({ kind: "project", project });
  const [path, setPath] = useState("");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [error, setError] = useState<string>();
  const [dragging, setDragging] = useState(false);
  const [dialog, setDialog] = useState<ExplorerDialog | null>(null);

  // Switching project resets the view to that project's root.
  useEffect(() => {
    setScope({ kind: "project", project });
    setPath("");
  }, [project]);

  /**
   * Open a file in this terminal's editor pane, switching the pane to it. The
   * pane fetches the content itself, so this does no I/O.
   */
  const openEntry = (path: string) => openFileInPane(project, term, scope, path);

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

  // Re-list when the editor pane saves (it cannot call load() directly).
  const savedTick = useFileSavedTick();
  useEffect(() => {
    load();
    // Skip the poll while the tab is hidden — the guarantee usePolled gives that
    // this hand-rolled loop otherwise lost (don't hammer the guest exec API for
    // a panel nobody is looking at).
    const t = setInterval(() => {
      if (!document.hidden) load();
    }, 10000);
    return () => clearInterval(t);
  }, [load, savedTick]);

  const crumbs = path ? path.split("/") : [];

  // A file/dir name typed by the user; rejects separators and dot-navigation.
  const nameError = (v: string): string | undefined => {
    if (!v || v === "." || v === "..") return "Enter a name";
    if (v.includes("/")) return "Names cannot contain /";
    return undefined;
  };

  const inDir = (name: string) => (path ? `${path}/${name}` : name);

  // The open editor must not survive its file being renamed/deleted — Save
  // on the stale path would recreate what the user just removed. This clears
  // every pane in the project, not just this terminal's: the same file can be
  // open in another terminal's pane.
  const closeIfOpen = (entryPath: string) => closeFileInPane(project, entryPath);

  const createFile = (name: string) =>
    tryUI(async () => {
      const p = inDir(name);
      await api.saveFile(scope, p, "");
      load();
      openEntry(p); // same editor as a click — never a second surface
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
            onClick={() => (e.dir ? setPath(e.path) : openEntry(e.path))}
          >
            <span className="shrink-0 text-text-dim">
              {e.dir ? (
                <FolderIcon className="h-3.5 w-3.5" />
              ) : (
                <FileIcon className="h-3.5 w-3.5" />
              )}
            </span>
            {/* Keyboard target on the NAME, not the row: the row contains
                real buttons/links (axe nested-interactive, 2026-08-30). */}
            <span
              role="button"
              tabIndex={0}
              onKeyDown={(ev) => {
                if (ev.key === "Enter" || ev.key === " ") {
                  ev.preventDefault();
                  if (e.dir) setPath(e.path);
                  else void openEntry(e.path);
                }
              }}
              className={`min-w-0 flex-1 truncate rounded focus-visible:outline focus-visible:outline-1 focus-visible:outline-accent ${e.dir ? "text-text" : "text-text-dim"}`}
            >
              {e.name}
              {e.symlink && <span className="text-accent"> ↗</span>}
            </span>
            {!e.dir && (
              <span className="shrink-0 text-[10px] text-text-dim">
                {fmtSize(e.size)}
              </span>
            )}
            <span className="invisible flex shrink-0 items-center group-hover:visible group-focus-within:visible">
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
              ? "The folder and everything inside it are deleted. This cannot be undone from here — commit first if you want it recoverable."
              : "This cannot be undone from here — commit first if you want it recoverable."
          }
          confirmLabel="Delete"
          danger
          onConfirm={() => deleteEntry(dialog.entry)}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  );
}
