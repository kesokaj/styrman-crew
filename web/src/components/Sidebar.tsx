import { useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { api, tryUI, type Project } from "../lib/api";
import {
  useMediaQuery,
  usePolled,
  useTheme,
  type ThemePref,
} from "../lib/hooks";
import { DeleteProjectDialog } from "./DeleteProjectDialog";
import { NewProjectDialog } from "./NewProjectDialog";
import { NotificationBell } from "./NotificationBell";
import { SetupBanner } from "./SetupBanner";
import { TerminalList } from "./TerminalList";
import { PromptDialog } from "./ui";
import {
  CrewMark,
  GearIcon,
  MonitorIcon,
  MoonIcon,
  MoreIcon,
  PlusIcon,
  SunIcon,
} from "./icons";

const themeOptions: Array<{
  pref: ThemePref;
  label: string;
  Icon: typeof SunIcon;
}> = [
  { pref: "light", label: "Light theme", Icon: SunIcon },
  { pref: "dark", label: "Dark theme", Icon: MoonIcon },
  { pref: "system", label: "Follow system theme", Icon: MonitorIcon },
];

export function Sidebar({
  open = false,
  onNavigate,
}: {
  /** Mobile drawer state; ignored on md+ where the rail is permanent. */
  open?: boolean;
  onNavigate?: () => void;
}) {
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as {
    project?: string;
    term?: string;
  };
  const { data, refresh } = usePolled(() => api.listProjects(), 5000);
  const { pref, setPref } = useTheme();
  // Off-screen ≠ gone: without inert, keyboard/screen-reader users tab into
  // the translated-away drawer on mobile.
  const isDesktop = useMediaQuery("(min-width: 768px)");
  const [showNew, setShowNew] = useState(false);
  const [deleting, setDeleting] = useState<Project | null>(null);
  const [renaming, setRenaming] = useState<Project | null>(null);
  const [menuFor, setMenuFor] = useState<string | null>(null);

  const projects = data?.projects ?? [];

  const openProject = (p: Project) =>
    tryUI(async () => {
      const { terminals } = await api.listTerminals(p.id);
      const first = terminals[0] ?? (await api.createTerminal(p.id));
      onNavigate?.();
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
        setRenaming(p);
        return;
      }
      if (action === "delete") {
        setDeleting(p);
        return;
      }
      refresh();
    });

  return (
    <aside
      // max-md transform variants ONLY: an always-on translate would make
      // this the containing block for every position:fixed descendant.
      className={`fixed inset-y-0 left-0 z-[45] flex h-full w-64 shrink-0 flex-col bg-bg-raised max-md:transition-transform md:static md:border-r md:border-border ${
        open
          ? "max-md:translate-x-0 max-md:shadow-2xl"
          : "max-md:-translate-x-full"
      }`}
      inert={!open && !isDesktop}
    >
      <div className="flex items-center gap-2 px-3 py-3">
        <Link
          to="/"
          onClick={onNavigate}
          className="flex items-center gap-2 text-sm font-semibold hover:opacity-80"
        >
          <CrewMark className="h-5 w-5" />
          crew
        </Link>
        <div className="ml-auto hidden md:block">
          <NotificationBell />
        </div>
        <button
          onClick={() => setShowNew(true)}
          className="inline-flex items-center gap-1 rounded-full bg-accent-soft px-3 py-1.5 text-xs font-medium text-on-accent-soft hover:brightness-105"
        >
          <PlusIcon className="h-3.5 w-3.5" />
          New project
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
              className={`group flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-bg-hover ${
                params.project === p.id ? "bg-bg-hover" : ""
              } ${p.state === "deleting" ? "pointer-events-none opacity-50" : ""}`}
              onClick={() => p.state !== "deleting" && openProject(p)}
            >
              <span
                className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                  p.state === "running"
                    ? "bg-ok"
                    : p.state === "deleting"
                      ? "animate-pulse bg-danger"
                      : "bg-border"
                }`}
                title={p.state}
              />
              <span className="min-w-0 flex-1 truncate">{p.name}</span>
              {p.state === "deleting" && (
                <span className="shrink-0 text-[10px] text-text-dim">
                  deleting…
                </span>
              )}
              {p.deleteError && p.state !== "deleting" && (
                <span
                  className="shrink-0 text-[10px] text-danger"
                  title={p.deleteError}
                >
                  delete failed
                </span>
              )}
              <button
                className="invisible inline-flex h-6 w-6 items-center justify-center rounded-md text-text-dim hover:bg-bg-active hover:text-text group-hover:visible"
                onClick={(e) => {
                  e.stopPropagation();
                  setMenuFor(menuFor === p.id ? null : p.id);
                }}
                aria-label={`Project ${p.name} actions`}
              >
                <MoreIcon />
              </button>
            </div>
            {menuFor === p.id && p.state !== "deleting" && (
              <div className="mx-2 mb-1 flex gap-1 rounded-lg border border-border bg-bg p-1 text-xs">
                {p.state === "running" ? (
                  <button
                    className="flex-1 rounded-md px-2 py-1 hover:bg-bg-hover"
                    onClick={() => projectAction(p, "stop")}
                  >
                    Stop
                  </button>
                ) : (
                  <button
                    className="flex-1 rounded-md px-2 py-1 hover:bg-bg-hover"
                    onClick={() => projectAction(p, "start")}
                  >
                    Start
                  </button>
                )}
                <button
                  className="flex-1 rounded-md px-2 py-1 hover:bg-bg-hover"
                  onClick={() => projectAction(p, "rename")}
                >
                  Rename
                </button>
                <button
                  className="flex-1 rounded-md px-2 py-1 text-danger hover:bg-danger-soft"
                  onClick={() => projectAction(p, "delete")}
                >
                  Delete
                </button>
              </div>
            )}
            {params.project === p.id && (
              <TerminalList
                project={p.id}
                activeTerm={params.term}
                onNavigate={onNavigate}
              />
            )}
          </div>
        ))}
      </nav>

      <div className="flex items-center justify-between border-t border-border px-3 py-2 text-xs text-text-dim">
        <Link
          to="/settings"
          onClick={onNavigate}
          className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 hover:bg-bg-hover hover:text-text"
        >
          <GearIcon className="h-3.5 w-3.5" />
          Settings
        </Link>
        <div
          className="flex items-center rounded-full bg-bg p-0.5"
          role="radiogroup"
          aria-label="Theme"
        >
          {themeOptions.map(({ pref: p, label, Icon }) => (
            <button
              key={p}
              role="radio"
              aria-checked={pref === p}
              aria-label={label}
              title={label}
              onClick={() => setPref(p)}
              className={`inline-flex h-6 w-6 items-center justify-center rounded-full ${
                pref === p
                  ? "bg-accent-soft text-on-accent-soft"
                  : "hover:text-text"
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
            </button>
          ))}
        </div>
      </div>

      {renaming && (
        <PromptDialog
          title={`Rename “${renaming.name}”`}
          initial={renaming.name}
          submitLabel="Rename"
          validate={(v) => (v ? undefined : "Name required")}
          onSubmit={(name) =>
            tryUI(async () => {
              if (name !== renaming.name)
                await api.renameProject(renaming.id, name);
              refresh();
            })
          }
          onClose={() => setRenaming(null)}
        />
      )}
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
            onNavigate?.();
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
