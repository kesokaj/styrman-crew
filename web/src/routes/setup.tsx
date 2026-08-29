import { useEffect, useState } from "react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { api } from "../lib/api";
import { TerminalPane } from "../components/TerminalPane";

export const Route = createFileRoute("/setup")({ component: SetupPage });

type Phase = "loading" | "username" | "booting" | "ready" | "error";

function SetupPage() {
  const navigate = useNavigate();
  const [phase, setPhase] = useState<Phase>("loading");
  const [username, setUsername] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string>();

  // Prefill with the current (email-derived) terminal username so the user
  // can accept or change it before anything is created. If a name was
  // already chosen in an earlier run, skip the picker — each setup step is
  // a durable checkpoint, never re-asked.
  useEffect(() => {
    api
      .settings()
      .then((s) => {
        setUsername(s.terminalUsername);
        if (s.terminalUsernameChosen) {
          setPhase("booting");
          return api.setupStart().then(() => setPhase("ready"));
        }
        setPhase("username");
      })
      .catch((e: Error) => {
        setError(e.message);
        setPhase("error");
      })
      .finally(() => setLoaded(true));
  }, []);

  // Matches the server's guest-name rule (useradd-safe) and rejects reserved
  // system accounts (root/daemon/… break the guest; the server rejects them
  // too — this is just a friendlier client-side check).
  const reserved = new Set([
    "root", "daemon", "bin", "sys", "sync", "games", "man", "lp", "mail",
    "news", "proxy", "www-data", "backup", "list", "nobody", "docker", "sshd",
    "sudo", "adm", "tty", "disk", "messagebus",
  ]);
  const valid =
    /^[a-z][a-z0-9_-]{0,31}$/.test(username) && !reserved.has(username);

  const start = async () => {
    setPhase("booting");
    setError(undefined);
    try {
      // Save first: the setup VM (and every project VM) is created with this
      // name; the server recreates the setup VM if the name changed.
      await api.setTerminalUsername(username.trim());
      await api.setupStart();
      setPhase("ready");
    } catch (e) {
      setError((e as Error).message);
      setPhase("error");
    }
  };

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <header className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-bg-raised px-3 text-xs">
        <span className="font-medium">First-time setup</span>
        <span className="text-text-dim">
          — sign in to git, GitHub, claude and agy once; everything lands in the
          shared home
        </span>
      </header>
      <div className="min-h-0 flex-1">
        {phase === "username" && (
          <div className="max-w-md p-4">
            <label
              htmlFor="setup-username"
              className="mb-1 block text-xs font-medium"
            >
              Choose your terminal username
            </label>
            <p className="mb-2 text-[11px] text-text-dim">
              This is your account name inside every project terminal — your
              shell prompt and home directory (<code>/home/{username || "…"}</code>
              ). Pick it now; it's used for all your projects. You can accept the
              default or change it.
            </p>
            <input
              id="setup-username"
              autoFocus
              autoCapitalize="none"
              autoCorrect="off"
              autoComplete="off"
              spellCheck={false}
              value={username}
              onChange={(e) => setUsername(e.target.value.toLowerCase())}
              onKeyDown={(e) => e.key === "Enter" && valid && loaded && start()}
              placeholder="e.g. simon"
              className="w-full rounded-md border border-border bg-bg px-2 py-1.5 text-sm outline-none focus:border-accent"
            />
            <p className="mb-3 mt-1 min-h-4 text-[11px] text-warn">
              {username && !valid
                ? "Lowercase letters, digits, - and _, starting with a letter."
                : " "}
            </p>
            <button
              onClick={start}
              disabled={!valid || !loaded}
              className="rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-on-accent disabled:opacity-40"
            >
              Start setup
            </button>
          </div>
        )}
        {phase === "loading" && (
          <p className="p-4 text-xs text-text-dim">Loading…</p>
        )}
        {phase === "booting" && (
          <p className="p-4 text-xs text-text-dim">
            Booting the setup VM as {username}…
          </p>
        )}
        {phase === "error" && (
          <div className="flex flex-col items-start gap-2 p-4">
            <p className="text-xs text-danger">{error}</p>
            <button
              onClick={() => setPhase("username")}
              className="rounded bg-accent-soft px-2 py-1 text-xs text-on-accent-soft hover:brightness-110"
            >
              Try again
            </button>
          </div>
        )}
        {phase === "ready" && (
          <TerminalPane
            target={{ kind: "setup" }}
            onExited={() => {
              // styrman-setup finished (the pane exits → close 4000). Mark setup
              // done, which also tears the onboarding VM down, then return to the
              // dashboard — no manual "Done" click, no idle VM left behind.
              api.setupDone().finally(() => navigate({ to: "/" }));
            }}
          />
        )}
      </div>
    </div>
  );
}
