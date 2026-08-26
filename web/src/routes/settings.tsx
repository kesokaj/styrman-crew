import { useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI, type ToolInfo } from "../lib/api";
import { usePolled, useTheme } from "../lib/hooks";

export const Route = createFileRoute("/settings")({ component: SettingsPage });

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline gap-3 border-b border-border py-2 last:border-0">
      <span className="w-40 shrink-0 text-text-dim">{label}</span>
      <span className="min-w-0 flex-1 break-words">{children}</span>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-6">
      <h2 className="mb-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
        {title}
      </h2>
      <div className="rounded-lg border border-border bg-bg-raised px-3">{children}</div>
    </section>
  );
}

function ToolCard({ tool }: { tool: ToolInfo }) {
  const chips = (label: string, items?: string[]) =>
    items && items.length > 0 ? (
      <div className="mt-1">
        <span className="text-[10px] uppercase tracking-wide text-text-dim">{label}</span>
        <div className="mt-0.5 flex flex-wrap gap-1">
          {items.map((i) => (
            <span key={i} className="rounded bg-bg px-1.5 py-0.5 text-[10px] text-text-dim">
              {i}
            </span>
          ))}
        </div>
      </div>
    ) : null;

  return (
    <div className="mb-3 rounded-lg border border-border bg-bg-raised p-3">
      <div className="flex items-center gap-2">
        <span className={`h-2 w-2 rounded-full ${tool.ready ? "bg-ok" : "bg-border"}`} />
        <span className="font-medium">{tool.name}</span>
        <span className="text-[11px] text-text-dim">
          {tool.ready ? "configured" : "not set up — run styrman-setup in a terminal"}
        </span>
        {tool.configDir && (
          <span className="ml-auto font-mono text-[10px] text-text-dim">{tool.configDir}</span>
        )}
      </div>
      {tool.details?.map((d) => (
        <p key={d} className="mt-1 break-all font-mono text-[10px] text-text-dim">
          {d}
        </p>
      ))}
      {chips("skills", tool.skills)}
      {chips("subagents", tool.agents)}
      {chips("commands", tool.commands)}
    </div>
  );
}

function TerminalUsername({ current, onSaved }: { current: string; onSaved: () => void }) {
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
        className="rounded bg-accent px-2 py-1 font-medium text-white"
      >
        Save
      </button>
      <button onClick={() => setEditing(false)} className="px-1 text-text-dim hover:text-text">
        cancel
      </button>
      <span className="text-[10px] text-text-dim">
        applies to project VMs started after the change (stop + start to apply)
      </span>
    </span>
  );
}

function SettingsPage() {
  const navigate = useNavigate();
  const { data, refresh } = usePolled(() => api.settings(), 15000);
  const { data: toolData } = usePolled(() => api.tools(), 20000);
  const { theme, toggle } = useTheme();
  const [newUser, setNewUser] = useState("");

  const addUser = () =>
    tryUI(async () => {
      await api.addUser(newUser.trim());
      setNewUser("");
      refresh();
    });

  const removeUser = (email: string) =>
    tryUI(async () => {
      if (!confirm(`Revoke access for ${email}? Their home and projects stay on disk.`)) return;
      await api.removeUser(email);
      refresh();
    });

  if (!data) return <div className="p-6 text-xs text-text-dim">Loading settings…</div>;

  return (
    <div className="min-w-0 flex-1 overflow-y-auto">
      <div className="mx-auto max-w-2xl p-6 text-xs">
        <h1 className="mb-5 text-sm font-semibold">Settings</h1>

        <Section title="Account">
          <Row label="Signed in as">
            {data.user} {data.isAdmin && <span className="text-accent">(admin)</span>}
          </Row>
          <Row label="Terminal username">
            <TerminalUsername current={data.terminalUsername} onSaved={refresh} />
          </Row>
          <Row label="Your home">{data.homeDir}</Row>
          <Row label="Auth mode">
            {data.authMode === "header" ? "Google sign-in via the ingress" : "none (local access)"}
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
              className="my-1 rounded bg-accent-soft px-2 py-1 text-accent hover:brightness-110"
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

        {data.isAdmin && (
          <Section title="Admin — user access">
            <Row label="Allowed accounts">
              <div className="space-y-1 py-1">
                {data.allowedUsers.map((u) => (
                  <div key={u} className="flex items-center gap-2">
                    <span>{u}</span>
                    <button
                      onClick={() => removeUser(u)}
                      className="text-text-dim hover:text-danger"
                      title="Revoke"
                    >
                      ✕
                    </button>
                  </div>
                ))}
                <div className="flex gap-1 pt-1">
                  <input
                    value={newUser}
                    onChange={(e) => setNewUser(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && newUser.includes("@") && addUser()}
                    placeholder="person@gmail.com"
                    className="flex-1 rounded border border-border bg-bg px-2 py-1 outline-none focus:border-accent"
                  />
                  <button
                    onClick={addUser}
                    disabled={!newUser.includes("@")}
                    className="rounded bg-accent px-2 py-1 font-medium text-white disabled:opacity-40"
                  >
                    Add
                  </button>
                </div>
                <p className="pt-1 text-[11px] text-text-dim">
                  Each account gets its own home, projects, and setup — they never see yours. Your
                  admin account comes from the deployment's env and cannot be revoked here.
                </p>
              </div>
            </Row>
          </Section>
        )}

        <Section title="Appearance">
          <Row label="Theme">
            <button onClick={toggle} className="rounded px-2 py-1 text-accent hover:bg-bg-hover">
              {theme === "dark" ? "☀ Switch to light" : "☾ Switch to dark"}
            </button>
          </Row>
        </Section>

        <Section title="Runtime">
          <Row label="Version">{data.version}</Row>
          <Row label="Project image">{data.image}</Row>
          <Row label="Container runtime">
            {data.runtime} {data.runtime === "mvm" && "(Kata microVM)"}
          </Row>
          <Row label="Docker disk / project">{data.dockerDiskGB} GB</Row>
          <Row label="Idle stop after">{data.idleTimeout}</Row>
        </Section>

        <Section title="Storage">
          <Row label="State dir">{data.stateDir}</Row>
          <Row label="Back up">
            Back up the whole state dir — it holds every user's home (workspaces, credentials,
            transcripts) and the project metadata.
          </Row>
        </Section>
      </div>
    </div>
  );
}
