import { lazy, Suspense, useEffect, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI, type CredItem } from "../lib/api";
import { usePolled, useTheme, type ThemePref } from "../lib/hooks";
import { toast } from "../lib/toast";

const CodeEditor = lazy(() => import("../components/CodeEditor"));

export const Route = createFileRoute("/settings")({ component: SettingsPage });

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-baseline gap-3 border-b border-border py-2 last:border-0">
      <span className="w-40 shrink-0 text-text-dim">{label}</span>
      <span className="min-w-0 flex-1 break-words">{children}</span>
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="mb-6">
      <h2 className="mb-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
        {title}
      </h2>
      <div className="rounded-lg border border-border bg-bg-raised px-3">
        {children}
      </div>
    </section>
  );
}

function TerminalUsername({
  current,
  onSaved,
}: {
  current: string;
  onSaved: () => void;
}) {
  const [value, setValue] = useState(current);
  const [editing, setEditing] = useState(false);
  if (!editing) {
    return (
      <span className="flex items-center gap-2">
        <span className="font-mono">{current}</span>
        <button
          onClick={() => {
            setValue(current);
            setEditing(true);
          }}
          className="text-accent hover:underline"
        >
          change
        </button>
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1">
      <input
        autoFocus
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="rounded border border-border bg-bg px-2 py-1 font-mono outline-none focus:border-accent"
      />
      <button
        onClick={() =>
          tryUI(async () => {
            await api.setTerminalUsername(value.trim());
            setEditing(false);
            onSaved();
          })
        }
        className="rounded bg-accent px-2 py-1 font-medium text-on-accent"
      >
        Save
      </button>
      <button
        onClick={() => setEditing(false)}
        className="px-1 text-text-dim hover:text-text"
      >
        cancel
      </button>
      <span className="text-[10px] text-text-dim">
        applies to project VMs started after the change (stop + start to apply)
      </span>
    </span>
  );
}

/** The credential broker (ADR-032): one live copy of each credential, served
 *  to every project. Sign in once in any project, import here, and every
 *  project — new ones included — is current with no re-auth. */
function CredentialsSection() {
  const { data, refresh } = usePolled(() => api.credentials(), 20000);
  const { data: st } = usePolled(() => api.setupStatus(), 20000);
  const { data: toolData } = usePolled(() => api.tools(), 20000);
  const { data: projData } = usePolled(() => api.listProjects(), 15000);
  const running = (projData?.projects ?? []).filter(
    (p) => p.state === "running",
  );
  const [importFrom, setImportFrom] = useState("");
  const [busy, setBusy] = useState(false);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [viewing, setViewing] = useState<CredItem | null>(null);

  const items = data?.items ?? [];
  const brokerOff = data?.brokerEnabled === false;

  if (brokerOff) {
    return (
      <Section title="Credentials">
        <Row label="Broker">
          <span className="text-text-dim">
            Not available in this deployment mode (the broker runs only on the
            containerized server, on each project's bridge).
          </span>
        </Row>
      </Section>
    );
  }

  const runImport = async () => {
    const pid = importFrom || running[0]?.id;
    if (!pid) return;
    setBusy(true);
    try {
      const { report } = await api.importCredentials(pid);
      const stored = Object.entries(report).filter(([, v]) =>
        v.startsWith("stored"),
      );
      toast(
        stored.length ? "success" : "info",
        stored.length
          ? `Imported: ${stored.map(([k]) => k).join(", ")}`
          : "Nothing signed in yet in that project",
      );
      refresh();
    } catch (e) {
      toast("error", (e as Error).message || "Import failed");
    } finally {
      setBusy(false);
    }
  };

  // Each standard tool/credential, mapped to its setup-status flag, its primary
  // broker item (click to read), and a one-line detail. Status dot: GREEN =
  // signed in AND synced to the broker, GREY = not set up, RED = signed in
  // locally but not synced (a gap to fix by re-running setup / Import).
  const find = (pred: (i: CredItem) => boolean) => items.find(pred);
  const detailOf = (name: string) =>
    (toolData?.tools ?? []).find((t) => t.name === name)?.details?.join("  ");
  const std = [
    {
      label: "git",
      set: !!st?.gitIdentity,
      item: find((i) => i.kind === "git"),
      synced: !!find((i) => i.name === "STYRMAN_GIT_NAME"),
      detail: detailOf("git"),
    },
    {
      label: "ssh",
      set: !!st?.sshKey,
      item: find((i) => i.id.startsWith("ssh-")),
      synced: !!find((i) => i.id.startsWith("ssh-")),
      detail: "~/.ssh/id_ed25519",
    },
    {
      label: "github",
      set: !!st?.github,
      item: find((i) => i.name === "GH_TOKEN"),
      synced: !!find((i) => i.name === "GH_TOKEN"),
      detail: detailOf("github"),
    },
    {
      label: "gcloud",
      set: !!st?.gcloud,
      item: find((i) => i.id === "gcloud-adc"),
      synced: !!find((i) => i.id === "gcloud-adc"),
      detail: "ADC (terraform + client libraries)",
    },
    {
      label: "claude",
      set: !!st?.claude,
      item: find((i) => i.id === "claude-credentials"),
      synced: !!find((i) => i.id === "claude-credentials"),
      detail: "~/.claude",
    },
    {
      label: "agy",
      set: !!st?.agy,
      item: find((i) => i.id === "agy-oauth-token"),
      synced: !!find((i) => i.id === "agy-oauth-token"),
      detail: "~/.gemini (Antigravity)",
    },
  ];
  const stdEnv = new Set(["STYRMAN_GIT_NAME", "STYRMAN_GIT_EMAIL", "GH_TOKEN"]);
  const custom = items.filter((i) => i.kind === "env" && !stdEnv.has(i.name));

  const dotClass = (c: (typeof std)[number]) =>
    !c.set ? "bg-border" : c.synced ? "bg-ok" : "bg-danger";
  const dotTitle = (c: (typeof std)[number]) =>
    !c.set
      ? "not set up"
      : c.synced
        ? "signed in · synced to every project"
        : "signed in locally but NOT synced — re-run setup or Import";

  return (
    <Section title="Credentials">
      <Row label="How it works">
        <span className="text-text-dim">
          Sign in once during setup; the broker keeps one live copy of each and
          delivers it to every project — no re-auth. Click any credential to
          read it. Green = synced, grey = not set up, red = signed in but not
          synced.
        </span>
      </Row>
      <div className="border-b border-border py-1">
        {std.map((c) => (
          <div
            key={c.label}
            className="flex items-center gap-2 px-1 py-1 text-xs"
          >
            <span
              className={`h-2 w-2 shrink-0 rounded-full ${dotClass(c)}`}
              title={dotTitle(c)}
            />
            <button
              onClick={() => c.item && setViewing(c.item)}
              disabled={!c.item}
              className="w-16 shrink-0 text-left font-medium enabled:hover:text-accent enabled:hover:underline disabled:cursor-default"
              title={c.item ? "Click to read" : undefined}
            >
              {c.label}
            </button>
            <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-text-dim">
              {c.set ? (c.detail ?? "signed in") : "not set up"}
            </span>
            {c.set && !c.synced && (
              <span className="shrink-0 text-[10px] text-danger">
                not synced
              </span>
            )}
          </div>
        ))}
      </div>
      <Row
        label={`Custom variables${custom.length ? ` (${custom.length})` : ""}`}
      >
        {custom.length === 0 ? (
          <span className="text-text-dim">none yet</span>
        ) : (
          <div className="flex flex-col gap-1 py-1">
            {custom.map((it) => (
              <div key={it.id} className="group flex items-center gap-2">
                <span className="h-2 w-2 shrink-0 rounded-full bg-ok" />
                <button
                  onClick={() => setViewing(it)}
                  className="min-w-0 truncate text-left font-mono hover:text-accent hover:underline"
                  title="Click to read"
                >
                  {it.name}
                </button>
                <button
                  onClick={() =>
                    tryUI(async () => {
                      await api.deleteCredential(it.kind, it.id);
                      refresh();
                    })
                  }
                  className="ml-auto shrink-0 text-[10px] text-text-dim opacity-0 hover:text-danger group-hover:opacity-100"
                >
                  remove
                </button>
              </div>
            ))}
          </div>
        )}
      </Row>
      <Row label="Import from a project">
        {running.length === 0 ? (
          <span className="text-text-dim">
            Start a project and sign in (gh, claude, gcloud, ssh, npm), then
            import here.
          </span>
        ) : (
          <div className="flex flex-wrap items-center gap-2 py-1">
            <select
              value={importFrom || running[0]?.id}
              onChange={(e) => setImportFrom(e.target.value)}
              className="rounded border border-border bg-bg px-2 py-1"
            >
              {running.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <button
              onClick={runImport}
              disabled={busy}
              className="rounded bg-accent-soft px-2 py-1 text-on-accent-soft hover:brightness-110 disabled:opacity-50"
            >
              {busy ? "Importing…" : "Import"}
            </button>
            <span className="text-[10px] text-text-dim">
              optional — setup already auto-syncs; use this to refresh/add from
              a project (gh · claude · gcloud · ssh · npm · agy)
            </span>
          </div>
        )}
      </Row>
      <Row label="Add a variable">
        <div className="flex flex-col gap-1 py-1">
          <div className="flex flex-wrap items-center gap-2">
            <input
              value={key}
              onChange={(e) => setKey(e.target.value.toUpperCase())}
              placeholder="MY_VARIABLE"
              className="w-40 rounded border border-border bg-bg px-2 py-1 font-mono"
            />
            <input
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="value"
              type="password"
              className="w-52 rounded border border-border bg-bg px-2 py-1 font-mono"
            />
            <button
              onClick={() =>
                tryUI(async () => {
                  if (!key.trim() || !value.trim()) return;
                  await api.putCredentialToken(key.trim(), value);
                  setKey("");
                  setValue("");
                  toast("success", "Variable stored");
                  refresh();
                })
              }
              className="rounded bg-accent-soft px-2 py-1 text-on-accent-soft hover:brightness-110"
            >
              Store
            </button>
          </div>
          <span className="text-[10px] text-text-dim">
            Any custom env variable (e.g. NPM_TOKEN, DATABASE_URL) — exported as
            a shell var in every project. Storing the same name again updates
            it.
          </span>
        </div>
      </Row>
      {viewing && (
        <CredViewer item={viewing} onClose={() => setViewing(null)} />
      )}
    </Section>
  );
}

/**
 * Read-only viewer for one stored credential — the operator's own secret, the
 * exact value the broker materializes into each project. Files show their text
 * (syntax-highlighted); env/git show the value; binary secrets are described,
 * not dumped.
 */
function CredViewer({
  item,
  onClose,
}: {
  item: CredItem;
  onClose: () => void;
}) {
  const [data, setData] = useState<{
    content?: string;
    path?: string;
    binary?: boolean;
  } | null>(null);
  const [error, setError] = useState<string>();
  const { resolved } = useTheme();
  useEffect(() => {
    let stale = false;
    api
      .credentialContent(item.kind, item.id)
      .then((d) => !stale && setData(d))
      .catch((e: Error) => !stale && setError(e.message));
    return () => {
      stale = true;
    };
  }, [item]);
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 md:p-8"
      onClick={onClose}
    >
      <div
        className="flex h-[80vh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl bg-bg-raised shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs">
          <span className="truncate font-mono font-medium">
            {data?.path || item.name}
          </span>
          <span className="shrink-0 rounded bg-warn/20 px-1.5 py-0.5 text-[10px] text-warn">
            read-only · secret
          </span>
          <button
            onClick={onClose}
            className="ml-auto inline-flex h-7 w-7 items-center justify-center rounded-lg text-text-dim hover:bg-bg-hover hover:text-text"
            aria-label="Close"
          >
            ✕
          </button>
        </div>
        {error ? (
          <p className="p-4 text-xs text-danger">{error}</p>
        ) : !data ? (
          <p className="p-4 text-xs text-text-dim">Loading…</p>
        ) : data.binary ? (
          <p className="p-4 text-xs text-text-dim">
            Binary secret — not shown as text. It's materialized into every
            project's home as {item.name}.
          </p>
        ) : (
          <Suspense
            fallback={
              <p className="p-4 text-xs text-text-dim">Loading editor…</p>
            }
          >
            <CodeEditor
              value={data.content ?? ""}
              filename={data.path ?? item.name}
              dark={resolved === "dark"}
              readOnly
              onChange={() => {}}
              onSave={() => {}}
            />
          </Suspense>
        )}
      </div>
    </div>
  );
}

function SettingsPage() {
  const navigate = useNavigate();
  const { data, refresh, error } = usePolled(() => api.settings(), 15000);
  const { pref, setPref } = useTheme();

  // Surface a failed load instead of "Loading…" forever (audit finding).
  if (!data && error)
    return (
      <div className="p-6 text-xs text-danger">
        Could not load settings: {error}
      </div>
    );
  if (!data)
    return <div className="p-6 text-xs text-text-dim">Loading settings…</div>;

  return (
    <div className="min-w-0 flex-1 overflow-y-auto">
      <div className="mx-auto max-w-2xl p-6 text-xs">
        <h1 className="mb-5 text-sm font-semibold">Settings</h1>

        <Section title="Account">
          <Row label="Signed in as">
            {data.user}{" "}
            {data.isAdmin && <span className="text-accent">(admin)</span>}
          </Row>
          <Row label="Terminal username">
            <TerminalUsername
              current={data.terminalUsername}
              onSaved={refresh}
            />
          </Row>
          <Row label="Your home">{data.homeDir}</Row>
          <Row label="Auth mode">
            {data.authMode === "header"
              ? "Google sign-in via the ingress"
              : "none (local access)"}
          </Row>
        </Section>

        <Section title="First-time setup">
          <Row label="Status">
            {data.setupComplete ? (
              <span className="text-ok">complete</span>
            ) : (
              <span className="text-warn">not marked complete</span>
            )}
          </Row>
          <Row label="">
            <button
              onClick={() => navigate({ to: "/setup" })}
              className="my-1 rounded bg-accent-soft px-2 py-1 text-on-accent-soft hover:brightness-110"
            >
              Open setup terminal
            </button>
          </Row>
        </Section>

        <CredentialsSection />

        <Section title="Appearance">
          <Row label="Theme">
            <div
              className="inline-flex rounded-full bg-bg p-0.5"
              role="radiogroup"
              aria-label="Theme"
            >
              {(
                [
                  ["light", "Light"],
                  ["dark", "Dark"],
                  ["system", "System"],
                ] as Array<[ThemePref, string]>
              ).map(([p, label]) => (
                <button
                  key={p}
                  role="radio"
                  aria-checked={pref === p}
                  onClick={() => setPref(p)}
                  className={`rounded-full px-3 py-1 ${
                    pref === p
                      ? "bg-accent-soft font-medium text-on-accent-soft"
                      : "text-text-dim hover:text-text"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </Row>
        </Section>

        <Section title="Runtime">
          <Row label="Version">{data.version}</Row>
          <Row label="Project image">{data.image}</Row>
          <Row label="Container runtime">
            {data.runtime} {data.runtime === "mvm" && "(Kata microVM)"}
          </Row>
          {data.isAdmin ? (
            <RuntimeEditor
              dockerDiskGB={data.dockerDiskGB}
              idleTimeout={data.idleTimeout}
              onSaved={refresh}
            />
          ) : (
            <>
              <Row label="Docker disk / project">{data.dockerDiskGB} GB</Row>
              <Row label="Idle stop after">{data.idleTimeout}</Row>
            </>
          )}
        </Section>

        <Section title="Storage">
          <Row label="State dir">{data.stateDir}</Row>
          <Row label="Back up">
            Back up the whole state dir — it holds every user's home
            (workspaces, credentials, transcripts) and the project metadata.
          </Row>
        </Section>
      </div>
    </div>
  );
}

/** Admin-editable runtime tunables. Disk applies to newly created project
 *  VMs; idle timeout applies to the reaper immediately. */
function RuntimeEditor({
  dockerDiskGB,
  idleTimeout,
  onSaved,
}: {
  dockerDiskGB: number;
  idleTimeout: string;
  onSaved: () => void;
}) {
  const [disk, setDisk] = useState(String(dockerDiskGB));
  const [idle, setIdle] = useState(idleTimeout);
  const [busy, setBusy] = useState(false);
  const dirty = disk !== String(dockerDiskGB) || idle !== idleTimeout;
  // A typed 0 (or cleared field) must be a visible error, not a silent
  // "keep" — the API treats 0 as keep-current (review finding).
  const diskValid = Number(disk) >= 5 && Number(disk) <= 500;

  const save = () =>
    tryUI(async () => {
      setBusy(true);
      try {
        const r = await api.updateRuntime({
          dockerDiskGB: Number(disk),
          idleTimeout: idle.trim(),
        });
        setDisk(String(r.dockerDiskGB));
        setIdle(r.idleTimeout);
        toast("success", "Runtime settings saved");
        onSaved();
      } finally {
        setBusy(false);
      }
    });

  return (
    <>
      <Row label="Docker disk / project">
        <span className="flex items-center gap-2">
          <input
            value={disk}
            onChange={(e) => setDisk(e.target.value.replace(/\D/g, ""))}
            className="w-16 rounded border border-border bg-bg px-2 py-1 text-right outline-none focus:border-accent"
            aria-label="Docker disk in GB"
          />
          <span className="text-text-dim">GB — applies to new project VMs</span>
        </span>
      </Row>
      <Row label="Idle stop after">
        <span className="flex items-center gap-2">
          <input
            value={idle}
            onChange={(e) => setIdle(e.target.value)}
            className="w-24 rounded border border-border bg-bg px-2 py-1 outline-none focus:border-accent"
            aria-label="Idle timeout (Go duration, 0s = never)"
          />
          <span className="text-text-dim">
            e.g. 24h, 90m — 0s never stops; applies immediately
          </span>
        </span>
      </Row>
      {dirty && (
        <Row label="">
          <span className="flex items-center gap-2">
            <button
              onClick={save}
              disabled={busy || !diskValid}
              className="my-1 rounded bg-accent px-2.5 py-1 font-medium text-on-accent disabled:opacity-40"
            >
              {busy ? "Saving…" : "Save runtime settings"}
            </button>
            {!diskValid && (
              <span className="text-danger">disk must be 5–500 GB</span>
            )}
          </span>
        </Row>
      )}
    </>
  );
}
