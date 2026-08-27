import { useState, useSyncExternalStore } from "react";
import { api, portUrl, tryUI } from "../lib/api";
import { clearClips, getClips, subscribeClips, type Clip } from "../lib/clips";
import { usePolled } from "../lib/hooks";
import { FileExplorer } from "./FileExplorer";

type Tab = "files" | "ports" | "history" | "git" | "clips";
const TABS: Array<[Tab, string]> = [
  ["files", "Files"],
  ["ports", "Ports"],
  ["history", "History"],
  ["git", "Git"],
  ["clips", "Clips"],
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
        {tab === "git" && <GitTab project={project} />}
        {tab === "clips" && <ClipsTab />}
      </div>
    </aside>
  );
}

function PortsTab({ project, term }: { project: string; term: string }) {
  const { data } = usePolled(() => api.listPorts(project), 7000);
  const ports = data?.ports ?? [];
  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        <p className="px-1 pb-2 text-[11px] text-text-dim">
          Ports listening inside this project's microVM. Links open the app
          through styrman.
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
      <ShareBox project={project} />
    </div>
  );
}

/**
 * Who can open this project's port previews. Owner-only by default; add Google
 * emails to grant preview-only access (they never see the terminal, files, or
 * dashboard — only the port links you send them).
 */
function ShareBox({ project }: { project: string }) {
  const { data, refresh } = usePolled(() => api.getShares(project), 30000);
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const emails = data?.emails ?? [];
  const valid = /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.trim());

  const save = (next: string[]) =>
    tryUI(async () => {
      setBusy(true);
      try {
        await api.setShares(project, next);
        refresh();
      } finally {
        setBusy(false);
      }
    });

  const add = () => {
    const e = email.trim().toLowerCase();
    if (!valid || emails.includes(e)) return;
    setEmail("");
    save([...emails, e]);
  };

  return (
    <div className="shrink-0 border-t border-border p-2">
      <div className="px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
        Share port previews
      </div>
      <p className="px-1 pb-1.5 text-[11px] text-text-dim">
        Owner-only by default. Add a Google email to let that person open this
        project's port links (previews only — nothing else).
      </p>
      {emails.map((e) => (
        <div
          key={e}
          className="group flex items-center gap-2 rounded px-1 py-0.5 text-[11px] hover:bg-bg-hover"
        >
          <span className="min-w-0 flex-1 truncate">{e}</span>
          <button
            disabled={busy}
            onClick={() => save(emails.filter((x) => x !== e))}
            className="invisible text-text-dim hover:text-danger group-hover:visible"
            aria-label={`Remove ${e}`}
          >
            remove
          </button>
        </div>
      ))}
      <div className="mt-1 flex gap-1">
        <input
          value={email}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          onChange={(e) => setEmail(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && add()}
          placeholder="name@gmail.com"
          className="min-w-0 flex-1 rounded border border-border bg-bg px-2 py-1 text-[11px] outline-none focus:border-accent"
        />
        <button
          onClick={add}
          disabled={!valid || busy}
          className="rounded bg-accent-soft px-2 py-1 text-[11px] text-accent hover:brightness-110 disabled:opacity-40"
        >
          add
        </button>
      </div>
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

/**
 * Every project is a git repo (R13): working-tree diff on top, checkpoints
 * below. Checkpoints are automatic whole-tree snapshots (plus a manual
 * button) living under refs/styrman/* — invisible to normal git, never
 * pushed — and restoring one rewrites the tree to exactly that moment (a
 * safety checkpoint of the current state is taken first).
 */
function GitTab({ project }: { project: string }) {
  const { data, error: diffError, refresh: refreshDiff } = usePolled(
    () => api.diff(project),
    10000,
  );
  const { data: cpData, error: cpError, refresh: refreshCps } = usePolled(
    () => api.listCheckpoints(project),
    15000,
  );
  const [busy, setBusy] = useState(false);
  const checkpoints = cpData?.checkpoints ?? [];

  const afterChange = () => {
    refreshCps();
    refreshDiff(); // the tree just changed — don't wait for the 10s poll
  };

  const checkpointNow = async () => {
    setBusy(true);
    try {
      await api.createCheckpoint(project, "manual");
      afterChange();
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const restore = async (id: string, subject: string) => {
    if (
      !confirm(
        `Restore "${subject}"?\n\nThe working tree is rewritten to that exact state — later edits and new files are removed. The current state is checkpointed first, so this can be undone.`,
      )
    )
      return;
    setBusy(true);
    try {
      await api.restoreCheckpoint(project, id);
      afterChange();
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (!data) {
    if (diffError)
      return <p className="p-2 text-xs text-danger">{diffError}</p>;
    return <p className="p-2 text-xs text-text-dim">Loading…</p>;
  }
  if (!data.repo) {
    return (
      <p className="p-2 text-xs text-text-dim">
        Not a git repository yet — it initializes on first boot.
      </p>
    );
  }
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border px-2 py-1.5 text-xs">
        branch <span className="font-medium">{data.branch || "main"}</span>
        <button
          onClick={checkpointNow}
          disabled={busy}
          className="ml-auto rounded bg-accent-soft px-2 py-0.5 text-accent hover:brightness-110 disabled:opacity-40"
          title="Snapshot the whole working tree (kept outside your branches)"
        >
          Checkpoint now
        </button>
      </div>
      <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap p-2 font-mono text-[11px] leading-snug">
        {data.status}
        {data.diff ? "\n" + data.diff : "\nNo uncommitted changes."}
      </pre>
      <div className="max-h-48 shrink-0 overflow-y-auto border-t border-border p-1.5">
        <div className="px-1 pb-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
          Checkpoints
        </div>
        {cpError && (
          <p className="px-1 pb-1 text-[11px] text-danger">{cpError}</p>
        )}
        {!cpError && checkpoints.length === 0 && (
          <p className="px-1 pb-1 text-[11px] text-text-dim">
            None yet. Snapshots are taken automatically when terminals pause or
            the project stops — or use "Checkpoint now".
          </p>
        )}
        {checkpoints.map((c) => (
          <div
            key={c.id}
            className="group flex items-center gap-2 rounded px-1 py-0.5 text-[11px] hover:bg-bg-hover"
          >
            <span className="shrink-0 text-text-dim">
              {c.time.slice(5, 16).replace("T", " ")}
            </span>
            <span className="min-w-0 flex-1 truncate">{c.subject}</span>
            <button
              onClick={() => restore(c.id, c.subject)}
              disabled={busy}
              className="invisible shrink-0 text-accent hover:brightness-110 disabled:opacity-40 group-hover:visible"
              aria-label={`Restore checkpoint ${c.subject}`}
            >
              restore
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * Everything copied inside styrman (terminal selections, Ctrl+C, link-bar
 * Copy) — the OS clipboard can't be read back by a web app, so styrman
 * remembers what it put there. Stored per-browser, never uploaded.
 */
function ClipsTab() {
  const [copiedAt, setCopiedAt] = useState<number>();
  // N8: subscribe via useSyncExternalStore (tear-free under React 19).
  const clips = useSyncExternalStore(subscribeClips, getClips);

  const recopy = (c: Clip) =>
    navigator.clipboard?.writeText(c.text).then(() => {
      setCopiedAt(c.at);
      setTimeout(() => setCopiedAt(undefined), 1200);
    });

  const fmt = (at: number) => {
    const d = new Date(at);
    return `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center border-b border-border px-2 py-1.5">
        <p className="flex-1 text-[11px] text-text-dim">
          Copied in styrman, newest first. Click to copy again.
        </p>
        {clips.length > 0 && (
          <button
            onClick={clearClips}
            className="text-[11px] text-text-dim hover:text-danger"
          >
            clear
          </button>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-1.5">
        {clips.length === 0 && (
          <p className="px-1 text-xs text-text-dim">
            Nothing yet. Select text in a terminal — it's copied the moment
            you release, and lands here too.
          </p>
        )}
        {clips.map((c) => (
          <button
            key={c.at}
            onClick={() => recopy(c)}
            className="group mb-0.5 flex w-full items-start gap-2 rounded px-1.5 py-1 text-left text-[11px] hover:bg-bg-hover"
            title={c.text.length > 400 ? c.text.slice(0, 400) + "…" : c.text}
          >
            <span className="shrink-0 pt-px text-[10px] text-text-dim">
              {fmt(c.at)}
            </span>
            <span className="min-w-0 flex-1 break-all font-mono leading-snug">
              {c.text.length > 160 ? c.text.slice(0, 160) + "…" : c.text}
            </span>
            <span className="invisible shrink-0 text-accent group-hover:visible">
              {copiedAt === c.at ? "✓" : "copy"}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
