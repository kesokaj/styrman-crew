import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI, type Project, type Terminal } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { CrewMark } from "../components/icons";

export const Route = createFileRoute("/")({ component: Home });

/**
 * The home surface: with no projects it's the welcome state; with projects
 * it's a dashboard — every project as a card, running terminals showing a
 * live transcript tail (operator item 15), click to jump straight in.
 */
function Home() {
  const { data, error } = usePolled(() => api.listProjects(), 5000);
  const projects = data?.projects ?? [];

  // A failed load must say so — a permanently blank dashboard reads as
  // "no projects" (audit finding). The poll retries on its own.
  if (!data && error)
    return (
      <div className="flex flex-1 items-center justify-center p-6 text-xs text-danger">
        Could not load projects: {error} — retrying…
      </div>
    );
  // Until the first poll lands (and during prerender) render a calm blank —
  // not a zero-card dashboard that flashes before the data arrives.
  if (!data) return <div className="flex-1" />;

  if (projects.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-text-dim">
        <CrewMark className="h-12 w-12" />
        <p className="text-sm text-text">Pick a project, or create one.</p>
        <p className="max-w-sm text-center text-xs">
          Every project runs in its own microVM with its own Docker daemon. The
          agent CLI in the terminal is the conversation.
        </p>
      </div>
    );
  }

  return (
    <div className="min-w-0 flex-1 overflow-y-auto">
      <div className="mx-auto max-w-5xl p-4 md:p-6">
        <h1 className="mb-4 text-lg font-medium">Your projects</h1>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {projects.map((p) => (
            <ProjectCard key={p.id} project={p} />
          ))}
        </div>
      </div>
    </div>
  );
}

function ProjectCard({ project: p }: { project: Project }) {
  const navigate = useNavigate();
  const running = p.state === "running";
  // Terminals only matter for the tails; poll GENTLY — this endpoint execs
  // into the microVM, and the dashboard multiplies it by every running
  // project (review finding).
  const { data } = usePolled(
    () => (running ? api.listTerminals(p.id) : Promise.resolve(undefined)),
    30000,
  );
  const terminals = (data?.terminals ?? []).filter((t) => t.live).slice(0, 4);

  const open = (term?: string) =>
    tryUI(async () => {
      let id = term;
      if (!id) {
        const { terminals } = await api.listTerminals(p.id);
        id = terminals[0]?.id ?? (await api.createTerminal(p.id)).id;
      }
      navigate({
        to: "/p/$project/$term",
        params: { project: p.id, term: id },
      });
    });

  return (
    <div className="rounded-2xl bg-bg-raised p-4">
      <button
        className="flex w-full items-center gap-2 text-left"
        onClick={() => open()}
        disabled={p.state === "deleting"}
      >
        <span
          className={`h-2 w-2 shrink-0 rounded-full ${
            running
              ? "bg-ok"
              : p.state === "deleting"
                ? "animate-pulse bg-danger"
                : "bg-border"
          }`}
        />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {p.name}
        </span>
        <span className="shrink-0 text-[11px] text-text-dim">{p.state}</span>
      </button>
      {running && terminals.length > 0 && (
        <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          {terminals.map((t) => (
            <TerminalTail
              key={t.id}
              project={p.id}
              terminal={t}
              onOpen={() => open(t.id)}
            />
          ))}
        </div>
      )}
      {running && data && terminals.length === 0 && (
        <p className="mt-2 text-[11px] text-text-dim">
          Running — no live terminals. Click to open one.
        </p>
      )}
    </div>
  );
}

/** The last lines of a terminal's transcript, refreshed live — a glance at
 *  what every agent is doing without opening anything. */
function TerminalTail({
  project,
  terminal,
  onOpen,
}: {
  project: string;
  terminal: Terminal;
  onOpen: () => void;
}) {
  const { data } = usePolled(
    () => api.readHistory(project, `conv-${terminal.id}`, 4096),
    15000,
  );
  const lines = (data?.text ?? "")
    .split("\n")
    .filter((l) => l.trim() !== "")
    .slice(-9);
  return (
    <button
      onClick={onOpen}
      className="group min-w-0 rounded-xl bg-term-bg p-2.5 text-left ring-1 ring-transparent transition-shadow hover:ring-accent"
      aria-label={`Open terminal ${terminal.title}`}
    >
      <div className="mb-1.5 flex items-center gap-1.5">
        <span className="min-w-0 flex-1 truncate text-[11px] font-medium">
          {terminal.title}
        </span>
        {terminal.bell && (
          <span
            className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-warn"
            title="waiting for input"
          />
        )}
      </div>
      <pre className="h-28 overflow-hidden whitespace-pre-wrap break-all font-mono text-[9.5px] leading-snug text-text-dim">
        {lines.length ? lines.join("\n") : "(quiet)"}
      </pre>
    </button>
  );
}
