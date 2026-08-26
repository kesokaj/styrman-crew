import { useEffect, useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { api } from "../lib/api";
import { TerminalPane } from "../components/TerminalPane";

export const Route = createFileRoute("/setup")({ component: SetupPage });

function SetupPage() {
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string>();
  useEffect(() => {
    api
      .setupStart()
      .then(() => setReady(true))
      .catch((e: Error) => setError(e.message));
  }, []);
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
        {error && <p className="p-4 text-xs text-danger">{error}</p>}
        {!error && !ready && (
          <p className="p-4 text-xs text-text-dim">
            Booting the hidden setup VM…
          </p>
        )}
        {ready && <TerminalPane target={{ kind: "setup" }} />}
      </div>
    </div>
  );
}
