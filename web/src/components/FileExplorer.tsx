import { useCallback, useEffect, useState } from "react";
import {
  api,
  tryUI,
  type FileContent,
  type FileEntry,
  type Scope,
} from "../lib/api";

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
          onClick={load}
          className="ml-auto px-1 text-text-dim hover:text-text"
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

      <div className="min-h-0 flex-1 overflow-y-auto px-1 pb-2">
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
  const dirty = text !== file.content;

  const save = () =>
    tryUI(async () => {
      setSaving(true);
      try {
        await api.saveFile(scope, file.path, text);
        onSaved();
        onClose();
      } finally {
        setSaving(false);
      }
    });

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-8"
      onClick={onClose}
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
            {!file.binary && (
              <button
                onClick={save}
                disabled={!dirty || saving}
                className="rounded bg-accent px-2 py-1 font-medium text-white disabled:opacity-40"
              >
                {saving ? "Saving…" : "Save"}
              </button>
            )}
            <button onClick={onClose} className="text-text-dim hover:text-text">
              ✕
            </button>
          </div>
        </div>
        {file.binary ? (
          <p className="p-4 text-xs text-text-dim">
            Binary file — use Download to fetch it.
          </p>
        ) : (
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            spellCheck={false}
            className="min-h-0 flex-1 resize-none bg-transparent p-3 font-mono text-xs outline-none"
          />
        )}
      </div>
    </div>
  );
}
