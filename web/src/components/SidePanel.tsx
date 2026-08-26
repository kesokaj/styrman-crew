import { useState } from "react";
import { api, portUrl } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { FileExplorer } from "./FileExplorer";

type Tab = "files" | "ports" | "history" | "diff";
const TABS: Array<[Tab, string]> = [
  ["files", "Files"],
  ["ports", "Ports"],
  ["history", "History"],
  ["diff", "Diff"],
];

/**
 * Right-hand context panel for the open project: file explorer, exposed
 * ports, terminal transcripts, and the working-tree diff. Terminals are NOT
 * here — they live in the sidebar, because a terminal is the main surface,
 * not a side note (ADR-016).
 */
export function SidePanel({
  project,
  term,
}: {
  project: string;
  term: string;
}) {
  const [open, setOpen] = useState(true);
  const [tab, setTab] = useState<Tab>("files");

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="flex h-full w-8 shrink-0 items-center justify-center border-l border-border bg-bg-raised text-text-dim hover:text-text"
        title="Show panel"
      >
        <span className="rotate-90 whitespace-nowrap text-[11px] tracking-wider">
          PANEL
        </span>
      </button>
    );
  }

  return (
    <aside className="flex h-full w-80 shrink-0 flex-col border-l border-border bg-bg-raised">
      <div className="flex items-center border-b border-border">
        {TABS.map(([k, label]) => (
          <button
            key={k}
            onClick={() => setTab(k)}
            className={`px-2.5 py-2 text-xs ${
              tab === k
                ? "border-b border-accent text-text"
                : "text-text-dim hover:text-text"
            }`}
          >
            {label}
          </button>
        ))}
        <button
          onClick={() => setOpen(false)}
          className="ml-auto px-2 text-text-dim hover:text-text"
          title="Hide panel"
        >
          ⨯
        </button>
      </div>
      <div className="min-h-0 flex-1">
        {tab === "files" && <FileExplorer project={project} />}
        {tab === "ports" && <PortsTab project={project} term={term} />}
        {tab === "history" && <HistoryTab project={project} />}
        {tab === "diff" && <DiffTab project={project} />}
      </div>
    </aside>
  );
}

function PortsTab({ project, term }: { project: string; term: string }) {
  const { data } = usePolled(() => api.listPorts(project), 7000);
  const ports = data?.ports ?? [];
  return (
    <div className="h-full overflow-y-auto p-2">
      <p className="px-1 pb-2 text-[11px] text-text-dim">
        Ports listening inside this project's microVM. Links open the app
        through styrman — shareable with anyone who can sign in.
      </p>
      {ports.length === 0 && (
        <p className="px-1 text-xs text-text-dim">
          Nothing listening. Start a dev server in a terminal and it appears
          here.
        </p>
      )}
      {ports.map((p) => (
        <div
          key={p.port}
          className="flex items-center gap-2 rounded px-1 py-1 text-xs hover:bg-bg-hover"
        >
          <a
            href={portUrl(project, term, p.port)}
            target="_blank"
            rel="noreferrer"
            className="rounded bg-accent-soft px-2 py-0.5 text-accent hover:brightness-110"
          >
            :{p.port}
          </a>
          <span className="min-w-0 flex-1 truncate text-[11px] text-text-dim">
            {p.image ? `🐳 ${p.image}` : p.process || ""}
          </span>
          <button
            className="shrink-0 text-[11px] text-text-dim hover:text-text"
            onClick={() =>
              navigator.clipboard?.writeText(
                location.origin + portUrl(project, term, p.port),
              )
            }
            title="Copy link"
          >
            copy link
          </button>
        </div>
      ))}
    </div>
  );
}

function HistoryTab({ project }: { project: string }) {
  const { data } = usePolled(() => api.listHistory(project), 10000);
  const [openId, setOpenId] = useState<string | null>(null);
  const [text, setText] = useState("");
  const transcripts = data?.transcripts ?? [];

  const open = async (id: string) => {
    setOpenId(id);
    setText("loading…");
    const r = await api
      .readHistory(project, id)
      .catch((e: Error) => ({ text: e.message }));
    setText(r.text);
  };

  if (openId) {
    return (
      <div className="flex h-full flex-col">
        <div className="flex items-center gap-2 border-b border-border px-2 py-1.5 text-xs">
          <button
            onClick={() => setOpenId(null)}
            className="text-text-dim hover:text-text"
          >
            ← back
          </button>
          <span className="truncate font-medium">{openId}</span>
        </div>
        <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap p-2 font-mono text-[11px] leading-snug">
          {text}
        </pre>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-2">
      <p className="px-1 pb-2 text-[11px] text-text-dim">
        Everything typed and printed in this project's terminals — you and the
        agents — captured to the shared home, surviving container rebuilds.
      </p>
      {transcripts.length === 0 && (
        <p className="px-1 text-xs text-text-dim">No transcripts yet.</p>
      )}
      {transcripts.map((t) => (
        <button
          key={t.id}
          onClick={() => open(t.id)}
          className="flex w-full items-center gap-2 rounded px-1 py-1 text-left text-xs hover:bg-bg-hover"
        >
          <span className="min-w-0 flex-1 truncate">{t.id}</span>
          <span className="shrink-0 text-[10px] text-text-dim">
            {(t.size / 1024).toFixed(0)} KB ·{" "}
            {t.modified.slice(5, 16).replace("T", " ")}
          </span>
        </button>
      ))}
    </div>
  );
}

function DiffTab({ project }: { project: string }) {
  const { data } = usePolled(() => api.diff(project), 10000);
  if (!data) return <p className="p-2 text-xs text-text-dim">Loading…</p>;
  if (!data.repo) {
    return <p className="p-2 text-xs text-text-dim">Not a git repository.</p>;
  }
  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-border px-2 py-1.5 text-xs">
        branch <span className="font-medium">{data.branch}</span>
      </div>
      <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap p-2 font-mono text-[11px] leading-snug">
        {data.status}
        {data.diff ? "\n" + data.diff : "\nNo uncommitted changes."}
      </pre>
    </div>
  );
}
