import { useRef, useState } from "react";
import { createFileRoute, useBlocker } from "@tanstack/react-router";
import { api, tryUI } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { SidePanel } from "../components/SidePanel";
import { TerminalPane } from "../components/TerminalPane";
import { TerminalStatusBar } from "../components/TerminalStatusBar";
import { EditorPane } from "../components/EditorPane";
import { ConfirmDialog } from "../components/ui";
import { basename, useEditorPane } from "../lib/editorPane";

export const Route = createFileRoute("/p/$project/$term")({
  component: TerminalPage,
});

function TerminalPage() {
  const { project, term } = Route.useParams();
  const { data: proj } = usePolled(() => api.getProject(project), 20000, [
    project,
  ]);
  const { data, refresh } = usePolled(() => api.listTerminals(project), 10000, [
    project,
  ]);
  const record = data?.terminals.find((t) => t.id === term);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");
  // Which view this terminal's main pane shows. Per-terminal, so one terminal
  // can hold an editor while another shows a shell (operator request).
  const { state: pane, setView } = useEditorPane(project, term);
  // Leaving this route (another terminal, a project, Settings) unmounts the
  // per-terminal editor and its unsaved buffer — only the store's dirty flag
  // survives, which then reads as a lie. Block with a discard prompt, and arm
  // the browser's beforeunload, while edits exist (code review, 2026-09-02).
  const blocker = useBlocker({
    shouldBlockFn: () => pane.dirty,
    enableBeforeUnload: () => pane.dirty,
    withResolver: true,
  });
  // Mount the editor lazily on first use, then keep it mounted alongside the
  // terminal so switching views never reloads the file or the session. (The
  // ref latches across terminal switches too, since this route does not
  // remount — harmless: it only decides whether the editor subtree exists.)
  const editorUsed = useRef(false);
  if (pane.view === "editor" || pane.file) editorUsed.current = true;
  // Bumped on every switch back to the terminal so it reclaims keyboard focus
  // (it is hidden, not unmounted, so nothing else would).
  const [focusToken, setFocusToken] = useState(0);
  const showTerminal = () => {
    setView("terminal");
    setFocusToken((n) => n + 1);
  };

  const savingRef = useRef(false);
  const saveTitle = async () => {
    if (savingRef.current) return; // Enter triggers blur → avoid double PATCH
    savingRef.current = true;
    setEditing(false);
    if (title.trim() && title.trim() !== record?.title) {
      // tryUI, not a silent catch: a failed rename otherwise reverts the
      // header with no explanation (audit finding).
      await tryUI(async () => {
        await api.updateTerminal(project, term, { title: title.trim() });
      });
      refresh();
    }
    savingRef.current = false;
  };

  return (
    <>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-bg-raised px-3 text-xs">
          <span className="text-text-dim">{proj?.name ?? "…"}</span>
          <span className="text-text-dim">/</span>
          {editing ? (
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onBlur={saveTitle}
              onKeyDown={(e) => e.key === "Enter" && saveTitle()}
              className="rounded border border-border bg-bg px-1.5 py-0.5 outline-none focus:border-accent"
            />
          ) : (
            <button
              className="font-medium hover:underline"
              title="Rename terminal"
              onClick={() => {
                setTitle(record?.title ?? "");
                setEditing(true);
              }}
            >
              {record?.title ?? `Terminal ${term}`}
            </button>
          )}
          {record?.live && (
            <span
              className="h-1.5 w-1.5 rounded-full bg-ok"
              title="live session"
            />
          )}
          {/* Terminal ⇄ Editor. The terminal keeps running either way — it is
              hidden with CSS below, never unmounted, so its WebSocket and tmux
              session survive the switch. */}
          <div className="ml-3 inline-flex rounded-full bg-bg p-0.5">
            {(
              [
                ["terminal", "Terminal"],
                ["editor", "Editor"],
              ] as Array<["terminal" | "editor", string]>
            ).map(([mode, label]) => (
              <button
                key={mode}
                onClick={() => (mode === "terminal" ? showTerminal() : setView(mode))}
                aria-pressed={pane.view === mode}
                // Pinned: the filename badge below is a CHILD of this button,
                // and without an explicit label it would join the accessible
                // name ("Editor foo.txt") — breaking every exact-name locator
                // and screen-reader muscle memory (review finding).
                aria-label={label}
                // The label pin hides the badge/dot below from assistive tech
                // (accname stops at the label); describe them instead.
                aria-describedby={
                  mode === "editor" && pane.file ? "editor-toggle-desc" : undefined
                }
                className={`rounded-full px-2.5 py-0.5 ${
                  pane.view === mode
                    ? "bg-accent-soft font-medium text-on-accent-soft"
                    : "text-text-dim hover:text-text"
                }`}
                title={
                  mode === "editor"
                    ? "Show the editor in this pane (the terminal keeps running)"
                    : "Show the terminal"
                }
              >
                {label}
                {/* Which file the (hidden) editor holds — otherwise invisible
                    from the terminal view (operator request 2026-09-01). */}
                {mode === "editor" && pane.file && (
                  <>
                    <span className="ml-1 hidden max-w-28 truncate align-middle font-mono text-[10px] sm:inline-block">
                      {basename(pane.file.path)}
                    </span>
                    <span id="editor-toggle-desc" className="sr-only">
                      {basename(pane.file.path)}
                      {pane.dirty ? ", unsaved changes" : ""}
                    </span>
                  </>
                )}
                {/* Unsaved edits are otherwise invisible from the terminal
                    view — the editor and its Save button are hidden. */}
                {mode === "editor" && pane.dirty && (
                  <span
                    className="ml-1 inline-block h-1.5 w-1.5 rounded-full bg-warn align-middle"
                    title="Unsaved changes"
                  />
                )}
              </button>
            ))}
          </div>
          <span
            className="ml-auto hidden max-w-64 truncate font-mono text-[10px] text-text-dim sm:inline"
            title={`Project ID ${project}`}
          >
            {project}
          </span>
        </header>
        {/* Both views live in the same pane. The terminal is hidden with
            `hidden` rather than unmounted: TerminalPane closes its WebSocket on
            unmount, so unmounting would tear down the session on every toggle. */}
        <div className="min-h-0 flex-1">
          <div className={`h-full ${pane.view === "editor" ? "hidden" : ""}`}>
            <TerminalPane
              key={`${project}/${term}`}
              target={{ kind: "term", project, id: term }}
              onExited={refresh}
              focusToken={focusToken}
            />
          </div>
          {editorUsed.current && (
            <div className={`h-full ${pane.view === "editor" ? "" : "hidden"}`}>
              <EditorPane
                // Keyed per terminal, like TerminalPane. Without it the pane
                // is REUSED across a terminal switch: `wanted` flips to the
                // new terminal's file, the loaded buffer is cleared, and the
                // previous terminal's unsaved edits are wiped with no prompt —
                // while setDirty(false) lands on the NEW terminal's key,
                // leaving the old one latched dirty (review finding).
                key={`${project}/${term}`}
                project={project}
                term={term}
                onShowTerminal={showTerminal}
              />
            </div>
          )}
        </div>
        <TerminalStatusBar project={project} />
      </div>
      {/* key={project}: reset the whole panel subtree on a project switch so no
          child (e.g. the port-share editor) can carry one project's state — and
          write it — onto another (M28). */}
      <SidePanel key={project} project={project} term={term} projectName={proj?.name} />
      {blocker.status === "blocked" && (
        <ConfirmDialog
          title="Discard unsaved changes?"
          body={`"${pane.file?.path ?? "The open file"}" has unsaved changes. Leaving this terminal will discard them.`}
          confirmLabel="Discard and leave"
          danger
          onConfirm={blocker.proceed}
          onCancel={blocker.reset}
          // onClose also runs after a confirm — resolve the blocker exactly
          // once via onConfirm/onCancel and keep this a no-op.
          onClose={() => {}}
        />
      )}
    </>
  );
}
