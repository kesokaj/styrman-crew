import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import {
  api,
  tryUI,
  type FileContent,
  type FileEntry,
  type Scope,
} from "../lib/api";
import { useTheme } from "../lib/hooks";

// CodeMirror is heavy — load it only when a file is actually opened.
const CodeEditor = lazy(() => import("./CodeEditor"));

const fmtSize = (n: number) =>
  n < 1024
    ? `${n} B`
    : n < 1024 * 1024
      ? `${(n / 1024).toFixed(0)} KB`
      : `${(n / 1048576).toFixed(1)} MB`;

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
  const validName = (name: string | null): name is string => {
    const n = name?.trim() ?? "";
    return n !== "" && n !== "." && n !== ".." && !n.includes("/");
  };

  // The open editor must not survive its file being renamed/deleted — Save
  // on the stale path would recreate what the user just removed.
  const closeIfOpen = (entryPath: string) => {
    setOpen((o) =>
      o && (o.path === entryPath || o.path.startsWith(entryPath + "/"))
        ? null
        : o,
    );
  };

  const newFolder = () => {
    const name = prompt("New folder name (in the current directory):");
    if (!validName(name)) return;
    tryUI(async () => {
      await api.makeDir(scope, path ? `${path}/${name.trim()}` : name.trim());
      load();
    });
  };

  const renameEntry = (e: FileEntry) => {
    const name = prompt("Rename to:", e.name);
    if (!validName(name) || name.trim() === e.name) return;
    const dir = e.path.includes("/")
      ? e.path.slice(0, e.path.lastIndexOf("/"))
      : "";
    tryUI(async () => {
      await api.renameFile(
        scope,
        e.path,
        dir ? `${dir}/${name.trim()}` : name.trim(),
      );
      closeIfOpen(e.path);
      load();
    });
  };

  const deleteEntry = (e: FileEntry) => {
    const what = e.dir ? `"${e.name}" and everything inside it` : `"${e.name}"`;
    if (!confirm(`Delete ${what}?`)) return;
    tryUI(async () => {
      await api.deleteFile(scope, e.path);
      closeIfOpen(e.path);
      load();
    });
  };

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
            className={`rounded px-2 py-0.5 ${
              scope.kind === k
                ? "bg-accent-soft text-accent"
                : "text-text-dim hover:bg-bg-hover"
            }`}
          >
            {label}
          </button>
        ))}
        <button
          onClick={() => {
            const name = prompt("New file name (in the current directory):");
            if (!name || name.includes("/")) return;
            const newPath = path ? `${path}/${name}` : name;
            tryUI(async () => {
              await api.saveFile(scope, newPath, "");
              load();
              setOpen(await api.readFile(scope, newPath));
            });
          }}
          className="ml-auto px-1 text-text-dim hover:text-accent"
          title="New file"
        >
          ＋
        </button>
        <button
          onClick={newFolder}
          className="px-1 text-text-dim hover:text-accent"
          title="New folder"
        >
          ⊞
        </button>
        <label className="cursor-pointer px-1 text-text-dim hover:text-accent" title="Upload files">
          ↥
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
        <button
          onClick={load}
          className="px-1 text-text-dim hover:text-text"
          title="Refresh"
        >
          ⟳
        </button>
      </div>

      <div className="flex flex-wrap items-center gap-0.5 px-2 py-1 text-[11px] text-text-dim">
        <button className="hover:text-text" onClick={() => setPath("")}>
          {scope.kind === "home" ? "~" : project}
        </button>
        {crumbs.map((c, i) => (
          <span key={i} className="flex items-center gap-0.5">
            <span>/</span>
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
            className="flex w-full items-center gap-2 rounded px-2 py-1 text-xs text-text-dim hover:bg-bg-hover"
            onClick={() => setPath(crumbs.slice(0, -1).join("/"))}
          >
            ../
          </button>
        )}
        {entries.map((e) => (
          <div
            key={e.path}
            className="group flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-xs hover:bg-bg-hover"
            onClick={() =>
              e.dir
                ? setPath(e.path)
                : tryUI(async () => setOpen(await api.readFile(scope, e.path)))
            }
          >
            <span className="w-3 shrink-0 text-text-dim">
              {e.dir ? "▸" : "·"}
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
            {!e.dir && (
              <a
                href={api.downloadUrl(scope, e.path)}
                onClick={(ev) => ev.stopPropagation()}
                className="invisible shrink-0 text-text-dim hover:text-accent group-hover:visible"
                title="Download"
              >
                ↓
              </a>
            )}
            <button
              onClick={(ev) => {
                ev.stopPropagation();
                renameEntry(e);
              }}
              className="invisible shrink-0 text-text-dim hover:text-accent group-hover:visible"
              title="Rename"
            >
              ✎
            </button>
            <button
              onClick={(ev) => {
                ev.stopPropagation();
                deleteEntry(e);
              }}
              className="invisible shrink-0 text-text-dim hover:text-danger group-hover:visible"
              title="Delete"
            >
              ⨯
            </button>
          </div>
        ))}
      </div>

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
  const { theme } = useTheme();
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
    if (!dirty || confirm("Discard unsaved changes?")) onClose();
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-8"
      onClick={tryClose}
    >
      <div
        className="flex h-full w-full max-w-4xl flex-col rounded-lg border border-border bg-bg-raised shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs">
          <span className="truncate font-medium">{file.path}</span>
          <span className="text-text-dim">{fmtSize(file.size)}</span>
          {file.truncated && <span className="text-warn">truncated</span>}
          <div className="ml-auto flex items-center gap-2">
            <a
              href={api.downloadUrl(scope, file.path)}
              className="text-text-dim hover:text-accent"
            >
              Download
            </a>
            {!file.binary && !file.truncated && (
              <button
                onClick={save}
                disabled={!dirty || saving}
                className="rounded bg-accent px-2 py-1 font-medium text-white disabled:opacity-40"
              >
                {saving ? "Saving…" : "Save"}
              </button>
            )}
            <button onClick={tryClose} className="text-text-dim hover:text-text">
              ✕
            </button>
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
              dark={theme === "dark"}
              readOnly={file.truncated}
              onChange={setText}
              onSave={save}
            />
          </Suspense>
        )}
      </div>
    </div>
  );
}
