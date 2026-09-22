import { useCallback, useEffect, useRef, useState } from "react";
import {
  clampRecentsHeight,
  loadRecentsCollapsed,
  loadRecentsHeight,
  storeRecentsCollapsed,
  storeRecentsHeight,
} from "../lib/recentsPanel";
import {
  canGoBack,
  canGoForward,
  currentPath,
  crumbTrail,
  goBack,
  goForward,
  initialHistory,
  parentOf,
  pushPath,
  type History,
} from "../lib/explorerHistory";
import { api, tryUI, type FileEntry, type Scope } from "../lib/api";
import {
  closeFileInPane,
  evictFromPanes,
  openFileInPane,
  useEditorPane,
  useFileSavedTick,
  useRecentFiles,
  type RecentFile,
} from "../lib/editorPane";
import { ConfirmDialog, IconButton, PromptDialog } from "./ui";
import {
  ArrowUpIcon,
  ChevronLeftIcon,
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
  // Back/forward history for THIS scope (see lib/explorerHistory). `path` is
  // derived from it — there is no separate path state to fall out of sync.
  const [history, setHistory] = useState<History>(() => initialHistory());
  const path = currentPath(history);
  const setPath = (p: string) => setHistory((h) => pushPath(h, p));
  const goUp = () => setPath(parentOf(path));
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [truncated, setTruncated] = useState(false);
  const [error, setError] = useState<string>();
  const [dragging, setDragging] = useState(false);
  // Recent-strip geometry: GLOBAL preference, read once on mount. Lazy
  // initialisers so localStorage is touched once, not on every render.
  const [recentsCollapsed, setRecentsCollapsed] = useState(loadRecentsCollapsed);
  const [recentsHeight, setRecentsHeight] = useState(loadRecentsHeight);
  /** In-flight resize drag: pointer Y and the height when it started. */
  const dragRef = useRef<{ y: number; h: number } | null>(null);
  /**
   * End a resize drag from EITHER pointerup or pointercancel. Persists once,
   * at the end: a write at pointermove rate would hammer localStorage for no
   * benefit, and `recentsHeight` here is current because React commits between
   * native pointer events.
   */
  const endResize = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragRef.current) return;
    dragRef.current = null;
    if (e.currentTarget.hasPointerCapture(e.pointerId))
      e.currentTarget.releasePointerCapture(e.pointerId);
    storeRecentsHeight(recentsHeight);
  };
  const [dialog, setDialog] = useState<ExplorerDialog | null>(null);

  // Switching project resets the view to that project's root.
  useEffect(() => {
    setScope({ kind: "project", project });
    setHistory(initialHistory());
    // setPath is intentionally absent: it is recreated every render and this
    // effect must run on a project change only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project]);

  /**
   * Open a file in this terminal's editor pane, switching the pane to it. The
   * pane fetches the content itself, so this does no I/O.
   */
  const openEntry = (path: string) => openFileInPane(project, term, scope, path);

  // What THIS terminal's editor pane is showing — the explorer highlights it
  // so "which file am I on" is answerable from the listing (operator request
  // 2026-09-01), plus the project's recently opened files for one-click
  // reopening.
  const { state: paneState } = useEditorPane(project, term);
  const openFile = paneState.file;
  const recents = useRecentFiles(project);
  const isOpen = (scopeKind: Scope["kind"], p: string) =>
    !!openFile && openFile.scope.kind === scopeKind && openFile.path === p;

  // Chip text: home files show their full path under ~/ (a bare "~/name"
  // implied a root-level file that does not exist); project files show the
  // basename, or parent/basename when another recent shares the basename —
  // identical chips reopened the wrong file (code review, 2026-09-02).
  const chipLabel = (r: RecentFile, all: RecentFile[]) => {
    if (r.scopeKind === "home") return `~/${r.path}`;
    const segs = r.path.split("/").filter(Boolean);
    const peers = all.filter((o) => o !== r && o.scopeKind === "project");
    // Walk up until the suffix is unique, rather than stopping after ONE
    // parent: `x/a/n.txt` and `y/a/n.txt` both rendered `a/n.txt`, two
    // identical chips (2026-09-02 review). Bounded by the path's own depth,
    // so the worst case is the full path — which is unique by definition.
    for (let take = 1; take <= segs.length; take++) {
      const label = segs.slice(segs.length - take).join("/");
      const collides = peers.some((o) => {
        const os = o.path.split("/").filter(Boolean);
        return os.slice(os.length - take).join("/") === label;
      });
      if (!collides) return label;
    }
    return r.path;
  };

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
        setTruncated(r.truncated === true);
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

  const { crumbs, elided } = crumbTrail(path);

  // A file/dir name typed by the user; rejects separators and dot-navigation.
  const nameError = (v: string): string | undefined => {
    if (!v || v === "." || v === "..") return "Enter a name";
    if (v.includes("/")) return "Names cannot contain /";
    return undefined;
  };
  // Creating over an existing entry silently TRUNCATED it (the save endpoint
  // opens O_CREATE+Truncate and there is no create-only endpoint) — and, with
  // no backup layer, that is unrecoverable (code review, 2026-09-02).
  const createError = (v: string): string | undefined =>
    nameError(v) ??
    (entries.some((e) => e.name === v)
      ? "An entry with that name already exists here"
      : undefined);

  const inDir = (name: string) => (path ? `${path}/${name}` : name);

  // The open editor must not survive its file being renamed/deleted — Save
  // on the stale path would recreate what the user just removed. This clears
  // every pane in the project, not just this terminal's: the same file can be
  // open in another terminal's pane.
  const closeIfOpen = (entryPath: string) =>
    closeFileInPane(project, scope.kind, entryPath);

  // Upload = replace on disk underneath any pane holding that file, so the
  // pane's pre-upload buffer must go (Save would overwrite the upload) while
  // the Recent chip stays — the file still exists. Evict EXACTLY what the
  // server reports it wrote — on success and on a mid-stream failure alike
  // (it reports the parts that landed before the error); never the dropped
  // names, which would discard unsaved edits for files it never touched
  // (code review, 2026-09-02).
  const upload = async (list: File[]) => {
    const r = await api.uploadFiles(scope, path, list);
    for (const name of r.saved)
      evictFromPanes(project, { scopeKind: scope.kind, path: inDir(name) });
    load();
    if (r.error) throw r.error;
  };

  const createFile = (name: string) =>
    tryUI(async () => {
      const p = inDir(name);
      // createFile, not saveFile: the dialog's existing-name check races the
      // last-loaded listing, so only the server's O_EXCL can guarantee this
      // does not truncate someone else's file (v1.17.2 review).
      await api.createFile(scope, p, "");
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
    <div
      className="flex h-full flex-col"
      // Bound on the PANEL, not on window: the terminal owns the keyboard
      // everywhere else, and a global Alt+Arrow would fight it (xterm sends
      // Alt+Arrow to the guest, where readline uses it for word motion).
      // tabIndex -1 so the container can hold focus when a click lands on
      // padding rather than on a control.
      tabIndex={-1}
      onKeyDown={(e) => {
        if (!e.altKey || e.ctrlKey || e.metaKey) return;
        if (e.key === "ArrowLeft") {
          e.preventDefault();
          setHistory(goBack);
        } else if (e.key === "ArrowRight") {
          e.preventDefault();
          setHistory(goForward);
        }
      }}
    >
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
              // A different tree: carrying history across would offer "back"
              // to a path that need not exist on this side.
              setHistory(initialHistory());
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
                  tryUI(() => upload(list));
                e.target.value = "";
              }}
            />
          </label>
          <IconButton label="Refresh" onClick={load}>
            <RefreshIcon />
          </IconButton>
        </div>
      </div>

      {/* Path bar. Was an 11px unpadded crumb row with no back/forward at all
          (operator, 2026-09-04). Now: real controls, text-xs, per-crumb hit
          targets, and middle elision instead of wrapping onto a second line
          and shoving the listing down in a 320px panel. */}
      <div className="flex items-center gap-0.5 border-b border-border px-1.5 py-1 text-xs">
        <IconButton label="Back" onClick={() => setHistory(goBack)} disabled={!canGoBack(history)}>
          <ChevronLeftIcon />
        </IconButton>
        <IconButton
          label="Forward"
          onClick={() => setHistory(goForward)}
          disabled={!canGoForward(history)}
        >
          <ChevronRightIcon />
        </IconButton>
        <IconButton label="Up one level" onClick={goUp} disabled={path === ""}>
          <ArrowUpIcon />
        </IconButton>
        <div className="ml-0.5 flex min-w-0 flex-1 items-center gap-0.5 overflow-hidden">
          <button
            className="shrink-0 truncate rounded px-1 py-0.5 text-text-dim hover:bg-bg-hover hover:text-text"
            onClick={() => setPath("")}
            title={scope.kind === "home" ? "Home" : `Project ID ${project}`}
            // Explicit: the visible label is the project's FRIENDLY NAME,
            // which the user can rename to anything, so it is useless as a
            // stable accessible name (and `title` does not supply one when the
            // button already has text).
            aria-label={scope.kind === "home" ? "Home root" : "Project root"}
          >
            {scope.kind === "home" ? "~" : (projectName ?? project)}
          </button>
          {elided && (
            <>
              <ChevronRightIcon className="h-3 w-3 shrink-0 opacity-60" />
              {/* Not a button: the dropped segments are still reachable with
                  Up, and a control that needs its own tooltip to explain
                  itself is worse than a plain marker. */}
              <span className="shrink-0 px-0.5 text-text-dim" title={path}>
                …
              </span>
            </>
          )}
          {crumbs.map((c) => (
            <span key={c.path} className="flex min-w-0 items-center gap-0.5">
              <ChevronRightIcon className="h-3 w-3 shrink-0 opacity-60" />
              <button
                className="truncate rounded px-1 py-0.5 text-text-dim hover:bg-bg-hover hover:text-text"
                onClick={() => setPath(c.path)}
                title={c.path}
              >
                {c.name}
              </button>
            </span>
          ))}
        </div>
      </div>

      <div
        data-testid="file-list"
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
            tryUI(() => upload(list));
        }}
      >
        {error && <p className="px-2 py-2 text-xs text-danger">{error}</p>}
        {!error && entries.length === 0 && (
          <p className="px-2 py-2 text-xs text-text-dim">Empty directory</p>
        )}
        {!error && truncated && (
          <p className="px-2 py-2 text-xs text-warn">
            This directory has more entries than the explorer lists. Some are
            not shown — use a terminal to see all of them.
          </p>
        )}
        {path !== "" && (
          <button
            className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-xs text-text-dim hover:bg-bg-hover"
            onClick={goUp}
          >
            ../
          </button>
        )}
        {entries.map((e) => (
          <div
            key={e.path}
            className={`group flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-xs ${
              // The open row had no hover: state, unlike every other row —
              // it read as disabled rather than as "this one is open"
              // (2026-09-01 editor-UX review, round 2).
              !e.dir && isOpen(scope.kind, e.path)
                ? "bg-accent-soft hover:bg-accent-soft/80"
                : "hover:bg-bg-hover"
            }`}
            onClick={() => (e.dir ? setPath(e.path) : openEntry(e.path))}
          >
            <span
              className={`shrink-0 ${
                !e.dir && isOpen(scope.kind, e.path)
                  ? "text-on-accent-soft"
                  : "text-text-dim"
              }`}
            >
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
              aria-current={
                !e.dir && isOpen(scope.kind, e.path) ? "true" : undefined
              }
              className={`min-w-0 flex-1 truncate rounded focus-visible:outline focus-visible:outline-1 focus-visible:outline-accent ${
                e.dir
                  ? "text-text"
                  : isOpen(scope.kind, e.path)
                    ? "font-medium text-on-accent-soft"
                    : "text-text-dim"
              }`}
            >
              {e.name}
              {e.symlink && <span className="text-accent"> ↗</span>}
            </span>
            {!e.dir && (
              <span
                className={`shrink-0 text-[10px] ${
                  isOpen(scope.kind, e.path)
                    ? "text-on-accent-soft"
                    : "text-text-dim"
                }`}
              >
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

      {/* BELOW the listing, not above (code review 2026-09-02): mounted above,
          the strip's first appearance shifted every row under a stationary
          pointer by almost a full row height — a double-click's second press
          landed on the wrong entry. That hazard is WORSE now the strip is a
          variable-height grid, which is also why its height comes from the
          stored value and not from its content: gaining a chip must never
          reflow the listing above it. */}
      {recents.length > 0 && (
        <div className="shrink-0 border-t border-border">
          {/* Drag handle sits ON the top border. Pointer events (not mouse)
              so a trackpad or a touch screen can resize too, with capture so
              the drag survives the pointer leaving the 4px strip. */}
          {!recentsCollapsed && (
            <div
              role="separator"
              aria-label="Resize recent files"
              aria-orientation="horizontal"
              // touch-none: without it a mobile browser can claim the initial
              // touch as a page-scroll gesture and cancel the pointer sequence
              // before the resize starts. select-none + preventDefault stop a
              // fast drag from selecting the text it passes over.
              className="h-1 cursor-row-resize touch-none select-none bg-transparent hover:bg-accent-soft"
              onPointerDown={(e) => {
                e.preventDefault();
                e.currentTarget.setPointerCapture(e.pointerId);
                dragRef.current = { y: e.clientY, h: recentsHeight };
              }}
              onPointerMove={(e) => {
                const d = dragRef.current;
                if (!d) return;
                // Dragging UP (smaller clientY) makes the strip taller.
                setRecentsHeight(clampRecentsHeight(d.h + (d.y - e.clientY)));
              }}
              onPointerUp={endResize}
              // A drag the OS cancels (a touch gesture taken over, a context
              // menu) fires pointercancel and never pointerup. Without this
              // the height the user just dragged to is applied on screen but
              // never persisted, so it silently reverts on the next reload.
              onPointerCancel={endResize}
            />
          )}
          <button
            onClick={() => {
              const next = !recentsCollapsed;
              setRecentsCollapsed(next);
              storeRecentsCollapsed(next);
            }}
            aria-expanded={!recentsCollapsed}
            aria-controls="recent-files"
            className="flex w-full items-center gap-1 px-2 py-1 text-[10px] uppercase tracking-wide text-text-dim hover:bg-bg-hover hover:text-text"
          >
            <ChevronRightIcon
              className={`h-3 w-3 transition-transform ${recentsCollapsed ? "" : "rotate-90"}`}
            />
            Recent ({recents.length})
          </button>
        </div>
      )}
      {recents.length > 0 && !recentsCollapsed && (
        <div
          data-testid="recent-files"
          id="recent-files"
          // The pixel height is the user's choice; the percentage is the floor
          // under the LISTING. The panel is h-full off the viewport, so on a
          // short one (small window, mobile landscape) a maxed-out strip would
          // otherwise squeeze the file list — the panel's primary content — to
          // a sliver while the strip stayed fully visible. max-height wins over
          // height, so this caps without touching what is stored.
          style={{ height: recentsHeight, maxHeight: "45%" }}
          className="flex shrink-0 flex-wrap content-start items-start gap-1 overflow-y-auto px-2 pb-1.5"
        >
          {recents.map((r) => {
            const open = isOpen(r.scopeKind, r.path);
            const full = (r.scopeKind === "home" ? "~/" : "") + r.path;
            return (
              <button
                key={`${r.scopeKind}/${r.path}`}
                onClick={() =>
                  openFileInPane(
                    project,
                    term,
                    r.scopeKind === "home"
                      ? { kind: "home", project }
                      : { kind: "project", project },
                    r.path,
                  )
                }
                aria-label={`Open recent file ${full}`}
                aria-current={open ? "true" : undefined}
                title={full}
                className={`max-w-40 shrink-0 truncate rounded-full px-2 py-0.5 font-mono text-[11px] ${
                  open
                    ? "bg-accent-soft font-medium text-on-accent-soft"
                    : "text-text-dim hover:bg-bg-hover hover:text-text"
                }`}
              >
                {chipLabel(r, recents)}
              </button>
            );
          })}
        </div>
      )}
      {dialog?.kind === "newFile" && (
        <PromptDialog
          title="New file"
          label="Created in the current directory."
          submitLabel="Create"
          validate={createError}
          onSubmit={createFile}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "newFolder" && (
        <PromptDialog
          title="New folder"
          label="Created in the current directory."
          submitLabel="Create"
          validate={createError}
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
