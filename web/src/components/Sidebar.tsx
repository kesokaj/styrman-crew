import { useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { api, projectNameError, tryUI, type Project } from "../lib/api";
import { firstTerminalId, useProjects } from "../lib/projects";
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
  ChevronRightIcon,
  CrewMark,
  GearIcon,
  MonitorIcon,
  MoonIcon,
  MoreIcon,
  PlusIcon,
  SunIcon,
} from "./icons";

// Per-project expand state persists across navigation and reloads. We store
// the COLLAPSED set (not expanded): projects stay expanded by default — a new
// project shows its terminals without any action, and collapse is explicit.
const COLLAPSED_KEY = "styrman-collapsed-projects";
function loadCollapsed(): Set<string> {
  try {
    const raw = localStorage.getItem(COLLAPSED_KEY);
    return raw ? new Set<string>(JSON.parse(raw)) : new Set();
  } catch {
    return new Set();
  }
}

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
  const { data, refresh, error } = useProjects();
  // ONE batched git poll for every badge (server iterates running projects).
  const { data: gitData } = usePolled(() => api.gitSummary(), 30000);
  const gitOf = (id: string) => gitData?.items.find((g) => g.projectId === id);
  const { pref, setPref } = useTheme();
  // Off-screen ≠ gone: without inert, keyboard/screen-reader users tab into
  // the translated-away drawer on mobile.
  const isDesktop = useMediaQuery("(min-width: 768px)");
  const [showNew, setShowNew] = useState(false);
  const [deleting, setDeleting] = useState<Project | null>(null);
  const [renaming, setRenaming] = useState<Project | null>(null);
  const [menuFor, setMenuFor] = useState<string | null>(null);
  // Optimistic stop/start feedback per project id, cleared when the request
  // settles — the dot pulses amber and the row says what's happening.
  const [pending, setPending] = useState<
    Record<string, "stopping" | "starting">
  >({});
  const [collapsed, setCollapsed] = useState<Set<string>>(loadCollapsed);

  const projects = data?.projects ?? [];

  const persistCollapsed = (next: Set<string>) => {
    setCollapsed(next);
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...next]));
    } catch {
      // private mode / disabled storage — expand state is a convenience, not
      // state we must keep; the UI still works for this session.
    }
  };
  const setExpanded = (id: string, expanded: boolean) => {
    const next = new Set(collapsed);
    if (expanded) next.delete(id);
    else next.add(id);
    persistCollapsed(next);
  };
  const allExpanded = projects.every((p) => !collapsed.has(p.id));
  const toggleAll = () =>
    persistCollapsed(
      allExpanded ? new Set(projects.map((p) => p.id)) : new Set(),
    );

  const openProject = (p: Project) =>
    tryUI(async () => {
      setExpanded(p.id, true); // opening a project reveals its terminals
      const term = await firstTerminalId(p.id);
      onNavigate?.();
      navigate({ to: "/p/$project/$term", params: { project: p.id, term } });
    });

  const projectAction = (
    p: Project,
    action: "stop" | "start" | "delete" | "rename",
  ) =>
    tryUI(async () => {
      setMenuFor(null);
      if (action === "rename") {
        setRenaming(p);
        return;
      }
      if (action === "delete") {
        setDeleting(p);
        return;
      }
      // Stop takes 10–40 s server-side (graceful guest shutdown before the
      // container stop) and Start boots a VM — show it immediately instead
      // of leaving the dot green/grey until the request returns (operator
      // feedback: "I press stop and the light is still green").
      setPending((m) => ({
        ...m,
        [p.id]: action === "stop" ? "stopping" : "starting",
      }));
      try {
        if (action === "stop") await api.stopProject(p.id);
        else await api.startProject(p.id);
        refresh();
      } finally {
        setPending((m) => {
          const next = { ...m };
          delete next[p.id];
          return next;
        });
      }
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
        <div className="flex items-center justify-between px-1 pb-1 pt-2">
          <span className="text-[11px] font-medium uppercase tracking-wider text-text-dim">
            Projects
          </span>
          {projects.length > 0 && (
            <button
              onClick={toggleAll}
              className="rounded px-1.5 py-0.5 text-[10px] font-medium text-text-dim hover:bg-bg-hover hover:text-text"
              title={
                allExpanded ? "Collapse all projects" : "Expand all projects"
              }
            >
              {allExpanded ? "Collapse all" : "Expand all"}
            </button>
          )}
        </div>
        {projects.length === 0 &&
          (error ? (
            // The empty state must not lie when the LIST failed (audit
            // finding) — the poll keeps retrying.
            <p className="px-2 py-4 text-xs text-danger">
              Could not load projects — retrying…
            </p>
          ) : (
            <p className="px-2 py-4 text-xs text-text-dim">
              No projects yet. Create one to get a terminal in its own microVM.
            </p>
          ))}
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
                  pending[p.id]
                    ? "animate-pulse bg-warn"
                    : p.state === "running"
                      ? "bg-ok"
                      : p.state === "deleting"
                        ? "animate-pulse bg-danger"
                        : "bg-border"
                }`}
                title={pending[p.id] ?? p.state}
              />
              {/* The NAME is the keyboard target — putting role=button on
                  the row nested the real action buttons inside it (axe
                  nested-interactive, 2026-08-30 review). */}
              <span
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    if (p.state !== "deleting") openProject(p);
                  }
                }}
                className="min-w-0 flex-1 truncate rounded focus-visible:outline focus-visible:outline-1 focus-visible:outline-accent"
              >
                {p.name}
              </span>
              {(() => {
                const g = gitOf(p.id);
                if (!g || p.state !== "running") return null;
                return (
                  <span
                    className={`shrink-0 font-mono text-[10px] ${g.dirty ? "text-warn" : "text-text-dim"}`}
                    title={`${g.branch || "?"} — ${g.dirty ? `${g.dirty} changed` : "clean"}${g.ahead ? `, ↑${g.ahead}` : ""}${g.behind ? `, ↓${g.behind}` : ""}`}
                  >
                    {g.branch}
                    {g.dirty ? `±${g.dirty}` : ""}
                    {g.ahead ? `↑${g.ahead}` : ""}
                  </span>
                );
              })()}
              {pending[p.id] && (
                <span className="shrink-0 text-[10px] text-text-dim">
                  {pending[p.id]}…
                </span>
              )}
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
                className="invisible inline-flex h-6 w-6 items-center justify-center rounded-md text-text-dim hover:bg-bg-active hover:text-text group-hover:visible group-focus-within:visible"
                onClick={(e) => {
                  e.stopPropagation();
                  setMenuFor(menuFor === p.id ? null : p.id);
                }}
                aria-label={`Project ${p.name} actions`}
              >
                <MoreIcon />
              </button>
              <button
                className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-text-dim hover:bg-bg-active hover:text-text"
                onClick={(e) => {
                  e.stopPropagation();
                  setExpanded(p.id, collapsed.has(p.id));
                }}
                aria-label={
                  collapsed.has(p.id)
                    ? `Expand ${p.name}`
                    : `Collapse ${p.name}`
                }
                aria-expanded={!collapsed.has(p.id)}
                title={collapsed.has(p.id) ? "Expand" : "Collapse"}
              >
                <ChevronRightIcon
                  className={`h-3.5 w-3.5 transition-transform ${
                    collapsed.has(p.id) ? "" : "rotate-90"
                  }`}
                />
              </button>
            </div>
            {menuFor === p.id && p.state !== "deleting" && (
              <div className="mx-2 mb-1 flex gap-1 rounded-lg border border-border bg-bg p-1 text-xs">
                {pending[p.id] ? (
                  <button
                    className="flex-1 rounded-md px-2 py-1 text-text-dim"
                    disabled
                  >
                    {pending[p.id]}…
                  </button>
                ) : p.state === "running" ? (
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
            {!collapsed.has(p.id) && p.state !== "deleting" && (
              <TerminalList
                project={p.id}
                activeTerm={params.project === p.id ? params.term : undefined}
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
          validate={(v) => (v ? projectNameError(v) : "Name required")}
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
            // The project is gone — bypass the unsaved-edits blocker, which
            // would otherwise offer to keep editing a deleted project.
            if (wasOpen) navigate({ to: "/", ignoreBlocker: true });
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
            // termId can be "" when the project was created but its first
            // terminal wasn't (partial success) — land on the dashboard then.
            if (termId) {
              navigate({
                to: "/p/$project/$term",
                params: { project: p, term: termId },
              });
            } else {
              navigate({ to: "/" });
            }
          }}
        />
      )}
    </aside>
  );
}
