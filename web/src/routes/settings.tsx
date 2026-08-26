import { useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api, tryUI } from "../lib/api";
import { usePolled, useTheme } from "../lib/hooks";

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

function SettingsPage() {
  const navigate = useNavigate();
  const { data, refresh } = usePolled(() => api.settings(), 15000);
  const { data: setup, refresh: refreshSetup } = usePolled(
    () => api.setupStatus(),
    15000,
  );
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
      if (!confirm(`Revoke access for ${email}?`)) return;
      await api.removeUser(email);
      refresh();
    });

  if (!data)
    return <div className="p-6 text-xs text-text-dim">Loading settings…</div>;

  return (
    <div className="min-w-0 flex-1 overflow-y-auto">
      <div className="mx-auto max-w-2xl p-6 text-xs">
        <h1 className="mb-5 text-sm font-semibold">Settings</h1>

        <Section title="Account">
          <Row label="Signed in as">{data.user}</Row>
          <Row label="Auth mode">
            {data.authMode === "header"
              ? "Google sign-in via the ingress"
              : "none (local access)"}
          </Row>
          <Row label="In-guest user">
            {data.guestUser} — the account your terminals run as inside every
            microVM
          </Row>
        </Section>

        <Section title="First-time setup">
          <Row label="Status">
            {setup?.done ? (
              <span className="text-ok">complete</span>
            ) : (
              <span className="text-warn">not marked complete</span>
            )}
          </Row>
          <Row label="Credentials">
            {setup ? (
              <span className="flex flex-wrap gap-2">
                {(
                  [
                    ["git", setup.gitIdentity],
                    ["ssh key", setup.sshKey],
                    ["github", setup.github],
                    ["claude", setup.claude],
                    ["keyring (agy)", setup.keyring],
                  ] as const
                ).map(([label, ok]) => (
                  <span
                    key={label}
                    className={ok ? "text-ok" : "text-text-dim"}
                  >
                    {ok ? "✓" : "○"} {label}
                  </span>
                ))}
              </span>
            ) : (
              "—"
            )}
          </Row>
          <Row label="">
            <div className="flex gap-2 py-1">
              <button
                onClick={() => navigate({ to: "/setup" })}
                className="rounded bg-accent-soft px-2 py-1 text-accent hover:brightness-110"
              >
                Open setup terminal
              </button>
              {!setup?.done && (
                <button
                  onClick={() =>
                    tryUI(async () => {
                      await api.setupDone();
                      refreshSetup();
                    })
                  }
                  className="rounded px-2 py-1 text-text-dim hover:text-text"
                >
                  Mark complete
                </button>
              )}
            </div>
          </Row>
        </Section>

        <Section title="Access">
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
                  onKeyDown={(e) =>
                    e.key === "Enter" && newUser.includes("@") && addUser()
                  }
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
                Accounts from the deployment's env are permanent and cannot be
                revoked here.
              </p>
            </div>
          </Row>
        </Section>

        <Section title="Appearance">
          <Row label="Theme">
            <button
              onClick={toggle}
              className="rounded px-2 py-1 text-accent hover:bg-bg-hover"
            >
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
          <Row label="Shared home">{data.homeDir}</Row>
          <Row label="State dir">{data.stateDir}</Row>
          <Row label="Back up">
            The shared home is the only thing that must be backed up — it holds
            every workspace, credential, and transcript.
          </Row>
        </Section>
      </div>
    </div>
  );
}
