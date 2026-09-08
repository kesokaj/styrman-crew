import { useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI, type Project, type Terminal } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { firstTerminalId, useProjects } from "../lib/projects";
import { CrewMark } from "../components/icons";
import { LanBadge } from "../components/LanBadge";
import { Dialog } from "../components/ui";

export const Route = createFileRoute("/")({ component: Home });

/**
 * The home surface: with no projects it's the welcome state; with projects
 * it's a dashboard — every project as a card, running terminals showing a
 * live transcript tail (operator item 15), click to jump straight in.
 */
function Home() {
  const { data, error } = useProjects();
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
      const id = term ?? (await firstTerminalId(p.id));
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
        <LanBadge project={p} />
        <span className="shrink-0 text-[11px] text-text-dim">{p.state}</span>
      </button>
      <CardOps project={p.id} running={running} />
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

/** Ops row (v1.14.0): live CPU/mem/disk chips for a running project + a Logs
 *  button (microVM console tail). Polls GENTLY — one stats exec per running
 *  card per 15s. */
function CardOps({ project, running }: { project: string; running: boolean }) {
  const [logsOpen, setLogsOpen] = useState(false);
  const { data: st } = usePolled(
    () => (running ? api.projectStats(project) : Promise.resolve(undefined)),
    15000,
    [project, running],
  );
  if (!running) return null;
  const gb = (b: number) => (b / 1024 / 1024 / 1024).toFixed(1);
  const pct = (used: number, total: number) =>
    total > 0 ? Math.round((used / total) * 100) : 0;
  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-text-dim">
      {st && st.state === "running" ? (
        <>
          <span title={`${st.cpuCount} vCPU, load ${st.load1}`}>
            cpu {st.cpuPercent}%
          </span>
          <span title={`${gb(st.memUsedBytes)} / ${gb(st.memTotalBytes)} GB`}>
            mem {pct(st.memUsedBytes, st.memTotalBytes)}%
          </span>
          <span title={`${gb(st.diskUsedBytes)} / ${gb(st.diskTotalBytes)} GB`}>
            disk {pct(st.diskUsedBytes, st.diskTotalBytes)}%
          </span>
          <span>{st.processes} procs</span>
        </>
      ) : (
        <span>…</span>
      )}
      <button
        onClick={() => setLogsOpen(true)}
        className="ml-auto rounded px-1.5 py-0.5 text-[10px] font-medium text-accent hover:bg-bg"
        title="MicroVM console logs (entrypoint, dockerd)"
      >
        logs
      </button>
      {logsOpen && (
        <LogsDialog project={project} onClose={() => setLogsOpen(false)} />
      )}
    </div>
  );
}

function LogsDialog({
  project,
  onClose,
}: {
  project: string;
  onClose: () => void;
}) {
  const { data, refresh, error } = usePolled(
    () => api.projectLogs(project, 300),
    30000,
    [project],
  );
  return (
    <Dialog title="MicroVM logs" onClose={onClose} width="w-[46rem]">
      <div className="mb-2 flex items-center gap-2 text-[11px] text-text-dim">
        <span>Console tail (entrypoint, dockerd, kernel) — newest last.</span>
        <button
          onClick={refresh}
          className="ml-auto rounded bg-accent-soft px-2 py-0.5 text-on-accent-soft hover:brightness-110"
        >
          Refresh
        </button>
      </div>
      {error && (
        <p className="mb-1 text-[11px] text-danger">
          Could not refresh logs — retrying… ({error})
        </p>
      )}
      <pre className="max-h-[60vh] overflow-auto rounded-lg bg-bg p-2 font-mono text-[11px] leading-snug">
        {data ? data.logs || "(no output)" : "Loading…"}
      </pre>
    </Dialog>
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
