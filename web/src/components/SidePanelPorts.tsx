import { useState } from "react";
import { api, portUrl, type PortInfo, type PublicLink } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { copyToClipboard, toast } from "../lib/toast";

export function PortsTab({ project, term }: { project: string; term: string }) {
  const { data } = usePolled(() => api.listPorts(project), 7000, [project]);
  // One project-wide links poll shared by all rows (not one per open row).
  const { data: linkData, refresh: refreshLinks } = usePolled(
    () => api.listLinks(project),
    30000,
    [project],
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
            copyToClipboard(location.origin + portUrl(project, term, info.port))
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
        else. The private link never expires on its own; it works until you
        remove every email (or delete the project). Empty = owner-only.
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
        Or make a public link — no login needed, and it stops working exactly at
        the TTL you pick (revocable early).
      </p>
      {links.map((l) => (
        <div
          key={l.token}
          className="flex items-center gap-2 rounded px-1 py-0.5 text-[11px] hover:bg-bg-hover"
        >
          <button
            onClick={() =>
              copyToClipboard(
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
