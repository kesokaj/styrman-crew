import { useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { api, tryUI, type Project } from "../lib/api";
import { usePolled, useTheme } from "../lib/hooks";
import { DeleteProjectDialog } from "./DeleteProjectDialog";
import { NewProjectDialog } from "./NewProjectDialog";
import { SetupBanner } from "./SetupBanner";
import { TerminalList } from "./TerminalList";

export function Sidebar() {
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as {
    project?: string;
    term?: string;
  };
  const { data, refresh } = usePolled(() => api.listProjects(), 5000);
  const { theme, toggle } = useTheme();
  const [showNew, setShowNew] = useState(false);
  const [deleting, setDeleting] = useState<Project | null>(null);
  const [menuFor, setMenuFor] = useState<string | null>(null);

  const projects = data?.projects ?? [];

  const openProject = (p: Project) =>
    tryUI(async () => {
      const { terminals } = await api.listTerminals(p.id);
      const first = terminals[0] ?? (await api.createTerminal(p.id));
      navigate({
        to: "/p/$project/$term",
        params: { project: p.id, term: first.id },
      });
    });

  const projectAction = (
    p: Project,
    action: "stop" | "start" | "delete" | "rename",
  ) =>
    tryUI(async () => {
      setMenuFor(null);
      if (action === "stop") await api.stopProject(p.id);
      if (action === "start") await api.startProject(p.id);
      if (action === "rename") {
        const name = prompt("New project name:", p.name);
        if (name && name !== p.name) await api.renameProject(p.id, name);
      }
      if (action === "delete") {
        setDeleting(p);
        return;
      }
      refresh();
    });

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-border bg-bg-raised">
      <div className="flex items-center justify-between px-3 py-3">
        <Link
          to="/"
          className="text-sm font-semibold tracking-wide hover:text-accent"
        >
          ⛵ styrman
        </Link>
        <button
          onClick={() => setShowNew(true)}
          className="rounded-md bg-accent-soft px-2.5 py-1 text-xs font-medium text-accent hover:brightness-110"
        >
          + New Project
        </button>
      </div>

      <SetupBanner />

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        <div className="px-1 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wider text-text-dim">
          Projects
        </div>
        {projects.length === 0 && (
          <p className="px-2 py-4 text-xs text-text-dim">
            No projects yet. Create one to get a terminal in its own microVM.
          </p>
        )}
        {projects.map((p) => (
          <div key={p.id} className="mb-0.5">
            <div
              className={`group flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 hover:bg-bg-hover ${
                params.project === p.id ? "bg-bg-hover" : ""
              }`}
              onClick={() => openProject(p)}
            >
              <span
                className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                  p.state === "running" ? "bg-ok" : "bg-border"
                }`}
                title={p.state}
              />
              <span className="min-w-0 flex-1 truncate">{p.name}</span>
              <button
                className="invisible rounded px-1 text-text-dim hover:text-text group-hover:visible"
                onClick={(e) => {
                  e.stopPropagation();
                  setMenuFor(menuFor === p.id ? null : p.id);
                }}
                aria-label={`Project ${p.name} actions`}
              >
                ⋯
              </button>
            </div>
            {menuFor === p.id && (
              <div className="mx-2 mb-1 flex gap-1 rounded-md border border-border bg-bg p-1 text-xs">
                {p.state === "running" ? (
                  <button
                    className="flex-1 rounded px-2 py-1 hover:bg-bg-hover"
                    onClick={() => projectAction(p, "stop")}
                  >
                    Stop
                  </button>
                ) : (
                  <button
                    className="flex-1 rounded px-2 py-1 hover:bg-bg-hover"
                    onClick={() => projectAction(p, "start")}
                  >
                    Start
                  </button>
                )}
                <button
                  className="flex-1 rounded px-2 py-1 hover:bg-bg-hover"
                  onClick={() => projectAction(p, "rename")}
                >
                  Rename
                </button>
                <button
                  className="flex-1 rounded px-2 py-1 text-danger hover:bg-bg-hover"
                  onClick={() => projectAction(p, "delete")}
                >
                  Delete
                </button>
              </div>
            )}
            {params.project === p.id && (
              <TerminalList project={p.id} activeTerm={params.term} />
            )}
          </div>
        ))}
      </nav>

      <div className="flex items-center justify-between border-t border-border px-3 py-2 text-xs text-text-dim">
        <Link to="/settings" className="hover:text-text">
          ⚙ Settings
        </Link>
        <button onClick={toggle} className="hover:text-text">
          {theme === "dark" ? "☀ Light" : "☾ Dark"}
        </button>
      </div>

      {deleting && (
        <DeleteProjectDialog
          projectId={deleting.id}
          projectName={deleting.name}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            const wasOpen = params.project === deleting.id;
            setDeleting(null);
            refresh();
            if (wasOpen) navigate({ to: "/" });
          }}
        />
      )}
      {showNew && (
        <NewProjectDialog
          onClose={() => setShowNew(false)}
          onCreated={(p, termId) => {
            setShowNew(false);
            refresh();
            navigate({
              to: "/p/$project/$term",
              params: { project: p, term: termId },
            });
          }}
        />
      )}
    </aside>
  );
}
