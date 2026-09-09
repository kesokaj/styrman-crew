import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI, type CredItem } from "../lib/api";
import {
  FONT_FAMILY_LABELS,
  FONT_SIZE_MAX,
  FONT_STACKS,
  FONT_SIZE_MIN,
  setAppearance,
  usePolled,
  useAppearance,
  useTheme,
  type Appearance,
  type FontFamilyPref,
  type ThemePref,
} from "../lib/hooks";
import { useProjects } from "../lib/projects";
import { toast } from "../lib/toast";
import { useModalA11y } from "../components/ui";

const CodeEditor = lazy(() => import("../components/CodeEditor"));
// Lazy: the terminal (xterm) is only pulled in when the operator actually opens
// the "add a credential" flow, keeping it out of the settings bundle.
const TerminalPane = lazy(() =>
  import("../components/TerminalPane").then((m) => ({
    default: m.TerminalPane,
  })),
);

// Display label for each styrman-setup tool in the add-credential picker.
const SETUP_TOOL_LABEL: Record<string, string> = {
  git: "git",
  ssh: "ssh",
  gh: "github",
  gcloud: "gcloud",
  claude: "claude",
  agy: "agy",
};

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
  const { data: st, refresh: refreshSt } = usePolled(
    () => api.setupStatus(),
    20000,
  );
  const { data: toolData } = usePolled(() => api.tools(), 20000);
  const { data: projData } = useProjects();
  const running = (projData?.projects ?? []).filter(
    (p) => p.state === "running",
  );
  const [importFrom, setImportFrom] = useState("");
  const [busy, setBusy] = useState(false);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [viewing, setViewing] = useState<CredItem | null>(null);
  // Non-null → the "add / re-sync a credential" modal is open, seeded with this
  // styrman-setup tool (git|ssh|gh|gcloud|claude|agy).
  const [manageTool, setManageTool] = useState<string | null>(null);

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
  // The agent harness roster (skills, subagents) seeded into every project
  // home, read from the newest seeded project; absent until one has booted.
  const rosterOf = (name: string, fallback: string) => {
    const t = (toolData?.tools ?? []).find((t) => t.name === name);
    const parts = [
      t?.skills?.length ? `${t.skills.length} skills` : "",
      t?.agents?.length ? `${t.agents.length} subagents` : "",
    ].filter(Boolean);
    return parts.length ? `${fallback} — ${parts.join(" · ")}` : fallback;
  };
  const std = [
    {
      label: "git",
      tool: "git",
      set: !!st?.gitIdentity,
      item: find((i) => i.kind === "git"),
      synced: !!find((i) => i.name === "STYRMAN_GIT_NAME"),
      detail: detailOf("git"),
    },
    {
      label: "ssh",
      tool: "ssh",
      set: !!st?.sshKey,
      item: find((i) => i.id.startsWith("ssh-")),
      synced: !!find((i) => i.id.startsWith("ssh-")),
      detail: "~/.ssh/id_ed25519",
    },
    {
      label: "github",
      tool: "gh",
      set: !!st?.github,
      item: find((i) => i.name === "GH_TOKEN"),
      synced: !!find((i) => i.name === "GH_TOKEN"),
      detail: detailOf("github"),
    },
    {
      label: "gcloud (ADC)",
      tool: "gcloud",
      set: !!st?.gcloud,
      item: find((i) => i.id === "gcloud-adc"),
      synced: !!find((i) => i.id === "gcloud-adc"),
      detail: "application-default — terraform + client libraries",
    },
    {
      label: "gcloud CLI",
      tool: "gcloud",
      set: !!find((i) => i.id === "gcloud-credentials-db"),
      item: find((i) => i.id === "gcloud-credentials-db"),
      synced: !!find((i) => i.id === "gcloud-credentials-db"),
      detail: "account login — `gcloud` commands",
    },
    {
      label: "claude",
      tool: "claude",
      set: !!st?.claude,
      item: find((i) => i.id === "claude-credentials"),
      synced: !!find((i) => i.id === "claude-credentials"),
      detail: rosterOf("claude", "~/.claude"),
    },
    {
      label: "agy",
      tool: "agy",
      set: !!st?.agy,
      item: find((i) => i.id === "agy-oauth-token"),
      synced: !!find((i) => i.id === "agy-oauth-token"),
      detail: rosterOf("agy", "~/.gemini (Antigravity)"),
    },
  ];
  // Deduped by tool (gcloud ADC + gcloud CLI both sign in via `styrman-setup
  // gcloud`) — the picker inside the add-credential modal.
  const setupTools: { tool: string; label: string; synced: boolean }[] = [];
  for (const c of std) {
    const ex = setupTools.find((t) => t.tool === c.tool);
    if (ex) ex.synced = ex.synced || c.synced;
    else
      setupTools.push({
        tool: c.tool,
        label: SETUP_TOOL_LABEL[c.tool] ?? c.tool,
        synced: c.synced,
      });
  }
  const afterHarvest = () => {
    refresh();
    refreshSt();
  };
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
          Sign in once; the broker keeps one live copy of each and delivers it
          to every project — no re-auth. Green = synced (click to read), grey =
          not set up, red = signed in but not synced. Click a grey or red one
          (or its <span className="text-accent">sign in</span> button) to boot
          the setup VM and add it.
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
              onClick={() =>
                c.synced && c.item ? setViewing(c.item) : setManageTool(c.tool)
              }
              className="w-16 shrink-0 text-left font-medium hover:text-accent hover:underline"
              title={c.synced ? "Click to read" : "Click to sign in"}
            >
              {c.label}
            </button>
            <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-text-dim">
              {c.set ? (c.detail ?? "signed in") : "not set up — click to sign in"}
            </span>
            {!c.synced && (
              <button
                onClick={() => setManageTool(c.tool)}
                className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium text-accent hover:bg-bg"
                title="Boot the setup VM and sign in — shared with every project"
              >
                {c.set ? "re-sync" : "sign in"}
              </button>
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
            it. Variables for ONE project only live in that project's side
            panel → Env (they override a global with the same name).
          </span>
        </div>
      </Row>
      {viewing && (
        <CredViewer item={viewing} onClose={() => setViewing(null)} />
      )}
      {manageTool && (
        <ManageCredsModal
          tools={setupTools}
          initialTool={manageTool}
          onHarvested={afterHarvest}
          onClose={() => {
            setManageTool(null);
            afterHarvest();
          }}
        />
      )}
    </Section>
  );
}

/**
 * Add or re-sync one or more credentials after first setup. Boots the hidden
 * onboarding VM once, runs `styrman-setup <tool>` for the chosen tool, and
 * harvests into the broker when that step finishes — the VM stays up so several
 * tools can be added in one sitting without re-booting. Closing (Done / click
 * outside) tears the VM down; the broker keeps whatever was signed in.
 */
function ManageCredsModal({
  tools,
  initialTool,
  onHarvested,
  onClose,
}: {
  tools: { tool: string; label: string; synced: boolean }[];
  initialTool: string;
  onHarvested: () => void;
  onClose: () => void;
}) {
  const [phase, setPhase] = useState<"booting" | "ready" | "error">("booting");
  const [error, setError] = useState<string>();
  const [activeTool, setActiveTool] = useState<string | null>(initialTool);
  const [added, setAdded] = useState<Set<string>>(new Set());
  const [tearing, setTearing] = useState(false);

  // Boot the setup VM once for the whole session (idempotent — reuses a VM
  // that's already up).
  useEffect(() => {
    let stale = false;
    api
      .setupStart()
      .then(() => !stale && setPhase("ready"))
      .catch((e: Error) => {
        if (stale) return;
        setError(e.message);
        setPhase("error");
      });
    return () => {
      stale = true;
    };
  }, []);

  const close = async () => {
    if (tearing) return;
    setTearing(true);
    // Best effort: leaving must always dismiss, even if teardown hiccups.
    await api.setupDiscard().catch(() => {});
    onClose();
  };

  // Shared modal contract (Escape / focus trap / focus restore) — the same
  // hook the Dialog shell uses, keeping this custom-layout overlay identical
  // for keyboard users (v1.14.0 polish batch).
  const panelRef = useRef<HTMLDivElement>(null);
  useModalA11y(panelRef, close);

  const onToolExited = () => {
    // styrman-setup <tool> finished (pane exit → close 4000) — but the pane
    // ALSO exits on a Ctrl+C cancel or a failed login, so success is decided
    // by the harvest REPORT, never by the exit itself (2026-08-30 audit: the
    // old unconditional toast showed a cancelled step as "signed in").
    const tool = activeTool;
    setActiveTool(null); // back to the picker; VM still running
    if (!tool) return;
    api
      .setupHarvest()
      .then(({ report }) => {
        onHarvested();
        const stored = Object.entries(report).some(
          ([k, v]) =>
            (k === tool || k.startsWith(`${tool}-`)) && v.startsWith("stored"),
        );
        if (stored) {
          setAdded((a) => new Set(a).add(tool));
          // File creds are pushed into every RUNNING project immediately
          // (v1.13.2) — no new terminal needed for gcloud/claude/ssh/agy.
          toast(
            "success",
            `${SETUP_TOOL_LABEL[tool] ?? tool} signed in — live in all projects`,
          );
        } else {
          toast(
            "info",
            `${SETUP_TOOL_LABEL[tool] ?? tool}: nothing new signed in`,
          );
        }
      })
      .catch(() => {});
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 md:p-8"
      onClick={close}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="Add or re-sync credentials"
        className="flex h-[80vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl bg-bg-raised shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-xs">
          <span className="font-medium">Add / re-sync credentials</span>
          <span className="min-w-0 flex-1 truncate text-text-dim">
            {phase === "ready"
              ? "setup VM running — sign in; it's shared with every project. Ctrl+C in the terminal cancels a step."
              : phase === "booting"
                ? "booting the setup VM…"
                : ""}
          </span>
          <button
            onClick={close}
            disabled={tearing}
            className="shrink-0 rounded bg-accent-soft px-2 py-1 text-on-accent-soft hover:brightness-110 disabled:opacity-50"
          >
            {tearing ? "Closing…" : "Done"}
          </button>
        </div>
        <div className="min-h-0 flex-1">
          {phase === "error" && (
            <p className="p-4 text-xs text-danger">{error}</p>
          )}
          {phase === "booting" && (
            <p className="p-4 text-xs text-text-dim">Booting the setup VM…</p>
          )}
          {phase === "ready" &&
            (activeTool ? (
              <Suspense
                fallback={
                  <p className="p-4 text-xs text-text-dim">Loading terminal…</p>
                }
              >
                <TerminalPane
                  target={{ kind: "setup", tool: activeTool }}
                  onExited={onToolExited}
                />
              </Suspense>
            ) : (
              <div className="flex flex-col gap-1 p-3 text-xs">
                <p className="mb-1 text-text-dim">
                  Click a credential to sign in. It's shared with every project
                  automatically — click Done when you're finished.
                </p>
                {tools.map((t) => {
                  const green = t.synced || added.has(t.tool);
                  return (
                    <button
                      key={t.tool}
                      onClick={() => setActiveTool(t.tool)}
                      className="flex items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-bg"
                    >
                      <span
                        className={`h-2 w-2 shrink-0 rounded-full ${green ? "bg-ok" : "bg-border"}`}
                      />
                      <span className="w-24 shrink-0 font-medium">
                        {t.label}
                      </span>
                      <span className="text-text-dim">
                        {green ? "signed in · synced" : "sign in →"}
                      </span>
                    </button>
                  );
                })}
              </div>
            ))}
        </div>
      </div>
    </div>
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
  const panelRef = useRef<HTMLDivElement>(null);
  useModalA11y(panelRef, onClose);
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 md:p-8"
      onClick={onClose}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={`Credential ${item.name}`}
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
              wrap
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
  const appearance = useAppearance();

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
          <FontRows
            label="Terminal font"
            appearance={appearance}
            sizeKey="termFontSize"
            familyKey="termFontFamily"
          />
          <FontRows
            label="Editor font"
            appearance={appearance}
            sizeKey="editorFontSize"
            familyKey="editorFontFamily"
          />
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

/**
 * One surface's font size + family. Both write straight through
 * `setAppearance`, which sanitises and persists; the live xterm and CodeMirror
 * instances pick the change up through the store, no reload.
 */
function FontRows({
  label,
  appearance,
  sizeKey,
  familyKey,
}: {
  label: string;
  appearance: Appearance;
  sizeKey: "termFontSize" | "editorFontSize";
  familyKey: "termFontFamily" | "editorFontFamily";
}) {
  const size = appearance[sizeKey];
  const family = appearance[familyKey];
  return (
    <>
      <Row label={`${label} size`}>
        <div className="flex items-center gap-3">
          <input
            type="range"
            min={FONT_SIZE_MIN}
            max={FONT_SIZE_MAX}
            step={1}
            value={size}
            aria-label={`${label} size`}
            onChange={(e) =>
              setAppearance({ [sizeKey]: Number(e.target.value) })
            }
            className="h-1 w-40 cursor-pointer accent-accent"
          />
          <span className="w-10 tabular-nums text-text-dim">{size}px</span>
        </div>
      </Row>
      <Row label={`${label} family`}>
        <div
          className="inline-flex rounded-full bg-bg p-0.5"
          role="radiogroup"
          aria-label={`${label} family`}
        >
          {FONT_FAMILY_LABELS.map(([value, text]) => (
            <button
              key={value}
              role="radio"
              aria-checked={family === value}
              onClick={() =>
                setAppearance({ [familyKey]: value as FontFamilyPref })
              }
              // The button previews the face it selects — picking a monospace
              // font from a list rendered in the UI face tells you nothing.
              style={{ fontFamily: FONT_STACKS[value] }}
              className={`rounded-full px-3 py-1 ${
                family === value
                  ? "bg-accent-soft font-medium text-on-accent-soft"
                  : "text-text-dim hover:text-text"
              }`}
            >
              {text}
            </button>
          ))}
        </div>
      </Row>
    </>
  );
}

/** Admin-editable runtime tunables. Disk applies to newly created project
 *  VMs; idle timeout applies to the reaper immediately. */
/**
 * The operator's view of the host half of per-project LAN access (ADR-065):
 * whether root's firewall script is installed and current, when it last ran,
 * and what "local network" means on this host. Admin-only — the payload
 * itself is only sent to admins.
 */

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
