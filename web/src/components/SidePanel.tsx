import { useState, useSyncExternalStore } from "react";
import { api, portUrl, type PortInfo, type PublicLink } from "../lib/api";
import { clearClips, getClips, subscribeClips, type Clip } from "../lib/clips";
import { usePolled } from "../lib/hooks";
import { toast } from "../lib/toast";
import { FileExplorer } from "./FileExplorer";
import { ConfirmDialog } from "./ui";
import { XIcon } from "./icons";

type Tab = "files" | "ports" | "history" | "git" | "clips";
const TABS: Array<[Tab, string]> = [
  ["files", "Files"],
  ["ports", "Ports"],
  ["history", "History"],
  ["git", "Git"],
  ["clips", "Clipboard"],
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
  // Desktop: inline panel, open by default. Below xl it OVERLAYS the
  // terminal, so it starts collapsed there (the route is client-rendered —
  // never prerendered — so reading the viewport in the initializer is safe).
  const [open, setOpen] = useState(
    () =>
      typeof window === "undefined" ||
      window.matchMedia("(min-width: 1280px)").matches,
  );
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
    <aside className="fixed inset-y-0 right-0 z-40 flex h-full w-80 max-w-[85vw] shrink-0 flex-col border-l border-border bg-bg-raised shadow-2xl xl:static xl:z-auto xl:max-w-none xl:shadow-none">
      <div className="flex items-center border-b border-border">
        {TABS.map(([k, label]) => (
          <button
            key={k}
            onClick={() => setTab(k)}
            className={`px-2.5 py-2 text-xs ${
              tab === k
                ? "border-b-2 border-accent font-medium text-text"
                : "text-text-dim hover:text-text"
            }`}
          >
            {label}
          </button>
        ))}
        <button
          onClick={() => setOpen(false)}
          className="ml-auto inline-flex h-7 w-7 items-center justify-center rounded-lg text-text-dim hover:bg-bg-hover hover:text-text"
          title="Hide panel"
          aria-label="Hide panel"
        >
          <XIcon className="h-3.5 w-3.5" />
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
  // One project-wide links poll shared by all rows (not one per open row).
  const { data: linkData, refresh: refreshLinks } = usePolled(
    () => api.listLinks(project),
    30000,
  );
  const ports = data?.ports ?? [];
  return (
    <div className="h-full overflow-y-auto p-2">
      <p className="px-1 pb-2 text-[11px] text-text-dim">
        Ports listening inside this project's microVM. Links open the app
        through styrman; “share” grants specific Google accounts access to that
        one port.
      </p>
      {ports.length === 0 && (
        <p className="px-1 text-xs text-text-dim">
          Nothing listening. Start a dev server in a terminal and it appears
          here.
        </p>
      )}
      {ports.map((p) => (
        <PortRow
          key={p.port}
          project={project}
          term={term}
          info={p}
          links={linkData?.links ?? []}
          refreshLinks={refreshLinks}
        />
      ))}
    </div>
  );
}

function PortRow({
  project,
  term,
  info,
  links,
  refreshLinks,
}: {
  project: string;
  term: string;
  info: PortInfo;
  links: PublicLink[];
  refreshLinks: () => void;
}) {
  const [sharing, setSharing] = useState(false);
  return (
    <div className="rounded hover:bg-bg-hover">
      <div className="flex items-center gap-2 px-1 py-1 text-xs">
        <a
          href={portUrl(project, term, info.port)}
          target="_blank"
          rel="noreferrer"
          className="rounded bg-accent-soft px-2 py-0.5 text-on-accent-soft hover:brightness-110"
        >
          :{info.port}
        </a>
        <span className="min-w-0 flex-1 truncate text-[11px] text-text-dim">
          {info.image ? `🐳 ${info.image}` : info.process || ""}
        </span>
        <button
          className="shrink-0 text-[11px] text-text-dim hover:text-text"
          onClick={() =>
            navigator.clipboard?.writeText(
              location.origin + portUrl(project, term, info.port),
            )
          }
          title="Copy link"
        >
          copy link
        </button>
        <button
          className={`shrink-0 text-[11px] ${sharing ? "text-accent" : "text-text-dim hover:text-text"}`}
          onClick={() => setSharing((v) => !v)}
          aria-expanded={sharing}
        >
          share
        </button>
      </div>
      {sharing && (
        <PortShare
          project={project}
          port={info.port}
          links={links}
          refreshLinks={refreshLinks}
        />
      )}
    </div>
  );
}

/**
 * Who can open ONE port's preview. Owner-only by default; add Google emails to
 * grant preview-only access to this port (they sign in with Google and see only
 * this port — never the terminal, files, dashboard, or other ports).
 */
function PortShare({
  project,
  port,
  links,
  refreshLinks,
}: {
  project: string;
  port: number;
  links: PublicLink[];
  refreshLinks: () => void;
}) {
  const { data, error } = usePolled(
    () => api.getPortShares(project, port),
    30000,
  );
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  // `override` holds the canonical list returned by the last PUT, so a save is
  // never based on a value that's up to 30 s stale (lost-update) and never on a
  // not-yet-loaded list (which would wipe it). Until the first load, emails is
  // undefined and editing is disabled.
  const [override, setOverride] = useState<{ emails: string[]; url: string }>();
  const emails = override?.emails ?? data?.emails;
  // The stable private URL for this port's shares (empty until someone is
  // shared). Served on its own *.private subdomain — recipients sign in with
  // Google there and only listed emails get through.
  const shareUrl = override ? override.url : (data?.url ?? "");
  const loaded = emails !== undefined;
  const valid = /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.trim());

  const save = async (next: string[]): Promise<boolean> => {
    setBusy(true);
    try {
      const res = await api.setPortShares(project, port, next);
      setOverride(res); // trust the server's normalized list + link URL
      return true;
    } catch (e) {
      toast("error", (e as Error).message || "Request failed");
      return false;
    } finally {
      setBusy(false);
    }
  };

  const add = async () => {
    const e = email.trim().toLowerCase();
    if (!valid || !loaded || emails.includes(e)) return;
    if (await save([...emails, e])) setEmail(""); // keep input on failure
  };

  return (
    <div className="ml-3 border-l border-border pl-2 pb-1.5">
      <p className="px-1 py-1 text-[11px] text-text-dim">
        Add a Google email to let that person sign in and open{" "}
        <span className="text-text">:{port}</span> — this port only, nothing
        else. Empty = owner-only.
      </p>
      {error && !loaded && (
        <p className="px-1 pb-1 text-[11px] text-danger">
          Couldn't load the share list — not editable until it loads.
        </p>
      )}
      {(emails ?? []).map((e) => (
        <div
          key={e}
          className="flex items-center gap-2 rounded px-1 py-0.5 text-[11px] hover:bg-bg-hover"
        >
          <span className="min-w-0 flex-1 truncate">{e}</span>
          <button
            disabled={busy}
            onClick={() => save((emails ?? []).filter((x) => x !== e))}
            className="shrink-0 rounded border border-border px-1.5 py-0.5 text-text-dim hover:border-danger hover:text-danger disabled:opacity-40"
            aria-label={`Remove ${e}`}
          >
            remove
          </button>
        </div>
      ))}
      {shareUrl && (
        <div className="flex items-center gap-2 rounded px-1 py-0.5 text-[11px]">
          <button
            onClick={() =>
              navigator.clipboard?.writeText(
                shareUrl.startsWith("http")
                  ? shareUrl
                  : location.origin + shareUrl,
              )
            }
            className="min-w-0 flex-1 truncate text-left text-accent hover:brightness-110"
            title={shareUrl}
          >
            copy private link · Google sign-in required
          </button>
        </div>
      )}
      <div className="mt-1 flex gap-1">
        <input
          value={email}
          disabled={!loaded}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          onChange={(e) => setEmail(e.target.value)}
          onKeyDown={(e) =>
            e.key === "Enter" && !e.nativeEvent.isComposing && add()
          }
          placeholder={loaded ? "name@gmail.com" : "loading…"}
          className="min-w-0 flex-1 rounded border border-border bg-bg px-2 py-1 text-[11px] outline-none focus:border-accent disabled:opacity-50"
        />
        <button
          onClick={add}
          disabled={!valid || busy || !loaded}
          className="rounded bg-accent-soft px-2 py-1 text-[11px] text-on-accent-soft hover:brightness-110 disabled:opacity-40"
        >
          add
        </button>
      </div>
      <PublicLinks
        project={project}
        port={port}
        links={links}
        refresh={refreshLinks}
      />
    </div>
  );
}

/**
 * Public, expiring links to one port — no Google login needed. The recipient
 * just opens the URL until it expires or you revoke it.
 */
function PublicLinks({
  project,
  port,
  links: projectLinks,
  refresh,
}: {
  project: string;
  port: number;
  links: PublicLink[];
  refresh: () => void;
}) {
  const [ttl, setTtl] = useState("24h");
  const [busy, setBusy] = useState(false);
  // Local override updated from create/revoke responses — the shared poll can be
  // up to 30 s stale (and refresh() no-ops while one is in flight), so don't
  // rely on it to reflect a change just made.
  const [override, setOverride] = useState<PublicLink[]>();
  const all = override ?? projectLinks;
  const links = all.filter((l) => l.port === port); // this port only

  const create = () =>
    run(async () => {
      const l = await api.createLink(project, port, ttl);
      setOverride([...all, l]);
      refresh();
    });
  const revoke = (token: string) =>
    run(async () => {
      await api.revokeLink(project, token);
      setOverride(all.filter((l) => l.token !== token));
      refresh();
    });

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      toast("error", (e as Error).message || "Request failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-2 border-t border-border pt-1.5">
      <p className="px-1 pb-1 text-[11px] text-text-dim">
        Or make a public link — no login, expires on its own.
      </p>
      {links.map((l) => (
        <div
          key={l.token}
          className="flex items-center gap-2 rounded px-1 py-0.5 text-[11px] hover:bg-bg-hover"
        >
          <button
            onClick={() =>
              navigator.clipboard?.writeText(
                l.url.startsWith("http") ? l.url : location.origin + l.url,
              )
            }
            className="min-w-0 flex-1 truncate text-left text-accent hover:brightness-110"
            title={l.url}
          >
            copy link · expires {new Date(l.expires).toLocaleString()}
          </button>
          <button
            disabled={busy}
            onClick={() => revoke(l.token)}
            className="shrink-0 rounded border border-border px-1.5 py-0.5 text-text-dim hover:border-danger hover:text-danger disabled:opacity-40"
            aria-label="Revoke link"
          >
            revoke
          </button>
        </div>
      ))}
      <div className="mt-1 flex gap-1">
        <select
          value={ttl}
          onChange={(e) => setTtl(e.target.value)}
          className="rounded border border-border bg-bg px-1 py-1 text-[11px] outline-none focus:border-accent"
        >
          <option value="1h">1 hour</option>
          <option value="24h">24 hours</option>
          <option value="7d">7 days</option>
        </select>
        <button
          onClick={create}
          disabled={busy}
          className="flex-1 rounded bg-accent-soft px-2 py-1 text-[11px] text-on-accent-soft hover:brightness-110 disabled:opacity-40"
        >
          create public link
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
  const {
    data,
    error: diffError,
    refresh: refreshDiff,
  } = usePolled(() => api.diff(project), 10000);
  const {
    data: cpData,
    error: cpError,
    refresh: refreshCps,
  } = usePolled(() => api.listCheckpoints(project), 15000);
  const [busy, setBusy] = useState(false);
  const [restoring, setRestoring] = useState<{
    id: string;
    subject: string;
  } | null>(null);
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
      toast("error", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const restore = async (id: string) => {
    setBusy(true);
    try {
      await api.restoreCheckpoint(project, id);
      afterChange();
    } catch (e) {
      toast("error", (e as Error).message);
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
          className="ml-auto rounded bg-accent-soft px-2 py-0.5 text-on-accent-soft hover:brightness-110 disabled:opacity-40"
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
              onClick={() => setRestoring({ id: c.id, subject: c.subject })}
              disabled={busy}
              className="invisible shrink-0 text-accent hover:brightness-110 disabled:opacity-40 group-hover:visible"
              aria-label={`Restore checkpoint ${c.subject}`}
            >
              restore
            </button>
          </div>
        ))}
      </div>
      {restoring && (
        <ConfirmDialog
          title={`Restore “${restoring.subject}”?`}
          body="The working tree is rewritten to that exact state — later edits and new files are removed. The current state is checkpointed first, so this can be undone."
          confirmLabel="Restore"
          danger
          onConfirm={() => restore(restoring.id)}
          onClose={() => setRestoring(null)}
        />
      )}
    </div>
  );
}

/**
 * Everything copied inside crew (terminal selections, Ctrl+C) — the OS
 * clipboard can't be read back by a web app, so crew remembers what it put
 * there. Stored per-browser, never uploaded.
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
          Copied in crew, newest first — kept for an hour, this tab only. Click
          to copy again.
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
            Nothing yet. Select text in a terminal — it's copied the moment you
            release, and lands here too.
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
