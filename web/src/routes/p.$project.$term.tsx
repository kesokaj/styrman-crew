import { useRef, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { api, tryUI } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { SidePanel } from "../components/SidePanel";
import { TerminalPane } from "../components/TerminalPane";
import { TerminalStatusBar } from "../components/TerminalStatusBar";

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
          <span
            className="ml-auto hidden max-w-64 truncate font-mono text-[10px] text-text-dim sm:inline"
            title={`Project ID ${project}`}
          >
            {project}
          </span>
        </header>
        <div className="min-h-0 flex-1">
          <TerminalPane
            key={`${project}/${term}`}
            target={{ kind: "term", project, id: term }}
            onExited={refresh}
          />
        </div>
        <TerminalStatusBar project={project} />
      </div>
      <SidePanel project={project} term={term} projectName={proj?.name} />
    </>
  );
}
