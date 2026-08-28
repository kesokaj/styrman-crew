import { useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI, type ToolInfo } from "../lib/api";
import { usePolled, useTheme, type ThemePref } from "../lib/hooks";
import { toast } from "../lib/toast";

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

function ToolCard({ tool }: { tool: ToolInfo }) {
  const chips = (label: string, items?: string[]) =>
    items && items.length > 0 ? (
      <div className="mt-1">
        <span className="text-[10px] uppercase tracking-wide text-text-dim">
          {label}
        </span>
        <div className="mt-0.5 flex flex-wrap gap-1">
          {items.map((i) => (
            <span
              key={i}
              className="rounded bg-bg px-1.5 py-0.5 text-[10px] text-text-dim"
            >
              {i}
            </span>
          ))}
        </div>
      </div>
    ) : null;

  return (
    <div className="mb-3 rounded-lg border border-border bg-bg-raised p-3">
      <div className="flex items-center gap-2">
        <span
          className={`h-2 w-2 rounded-full ${tool.ready ? "bg-ok" : "bg-border"}`}
        />
        <span className="font-medium">{tool.name}</span>
        <span className="text-[11px] text-text-dim">
          {tool.ready
            ? "configured"
            : "not set up — run styrman-setup in a terminal"}
        </span>
        {tool.configDir && (
          <span className="ml-auto font-mono text-[10px] text-text-dim">
            {tool.configDir}
          </span>
        )}
      </div>
      {tool.details?.map((d) => (
        <p
          key={d}
          className="mt-1 break-all font-mono text-[10px] text-text-dim"
        >
          {d}
        </p>
      ))}
      {chips("skills", tool.skills)}
      {chips("subagents", tool.agents)}
      {chips("commands", tool.commands)}
    </div>
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
  const { data: projData } = usePolled(() => api.listProjects(), 15000);
  const running = (projData?.projects ?? []).filter((p) => p.state === "running");
  const [importFrom, setImportFrom] = useState("");
  const [busy, setBusy] = useState(false);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");

  const items = data?.items ?? [];
  const brokerOff = data?.brokerEnabled === false;
  const kindLabel: Record<string, string> = {
    git: "git helper",
    env: "shell var",
    file: "file",
    keyring: "keyring",
  };

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

  return (
    <Section title="Credentials">
      <Row label="How it works">
        <span className="text-text-dim">
          The broker keeps one live copy of each credential and delivers the
          current one to every project — sign in once, no re-auth. Removing one
          stops sharing it with new sessions; restart a project to clear a
          token or file already materialized in it.
        </span>
      </Row>
      <Row label="Shared now">
        {items.length === 0 ? (
          <span className="text-text-dim">
            None yet — import from a project you've signed into.
          </span>
        ) : (
          <div className="flex flex-col gap-1 py-1">
            {items.map((it) => (
              <div
                key={`${it.kind}:${it.name}`}
                className="flex items-center gap-2"
              >
                <span className="h-2 w-2 rounded-full bg-ok" />
                <span className="font-mono">{it.name}</span>
                <span className="rounded bg-bg px-1.5 py-0.5 text-[10px] text-text-dim">
                  {kindLabel[it.kind] ?? it.kind}
                </span>
                <button
                  onClick={() =>
                    tryUI(async () => {
                      await api.deleteCredential(it.kind, it.id);
                      refresh();
                    })
                  }
                  className="ml-auto text-[10px] text-text-dim hover:text-danger"
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
              reads gh · claude · gcloud · ssh · npm · agy
            </span>
          </div>
        )}
      </Row>
      <Row label="Add a token">
        <div className="flex flex-wrap items-center gap-2 py-1">
          <input
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="NPM_TOKEN"
            className="w-36 rounded border border-border bg-bg px-2 py-1 font-mono"
          />
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="value"
            type="password"
            className="w-48 rounded border border-border bg-bg px-2 py-1 font-mono"
          />
          <button
            onClick={() =>
              tryUI(async () => {
                if (!key.trim() || !value.trim()) return;
                await api.putCredentialToken(key.trim(), value);
                setKey("");
                setValue("");
                toast("success", "Token stored");
                refresh();
              })
            }
            className="rounded bg-accent-soft px-2 py-1 text-on-accent-soft hover:brightness-110"
          >
            Store
          </button>
        </div>
      </Row>
    </Section>
  );
}

function SettingsPage() {
  const navigate = useNavigate();
  const { data, refresh, error } = usePolled(() => api.settings(), 15000);
  const { data: toolData } = usePolled(() => api.tools(), 20000);
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

        <h2 className="mb-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
          Tools & credentials
        </h2>
        {(toolData?.tools ?? []).map((t) => (
          <ToolCard key={t.name} tool={t} />
        ))}

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
