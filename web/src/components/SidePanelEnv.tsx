import { useState } from "react";
import { api, tryUI } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { toast } from "../lib/toast";

/**
 * Project-scoped env variables (ADR-043). Same idea as Settings → "Add a
 * variable", but these reach ONLY this project's login shells — the broker
 * merges them over the global set for this project, and a project value wins
 * when both define the same name. Values apply to NEW shells (open a fresh
 * terminal); running shells keep what they already exported.
 */
export function EnvTab({ project }: { project: string }) {
  const { data, refresh, error } = usePolled(
    () => api.projectEnv(project),
    15000,
    [project],
  );
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const keys = [...(data?.keys ?? [])].sort();

  if (!data) {
    // Don't assert "No variables yet" (or render the form) before the first
    // fetch resolves — the broker-disabled state isn't known yet either.
    return (
      <p className={`p-3 text-xs ${error ? "text-danger" : "text-text-dim"}`}>
        {error ? "Could not load variables — retrying…" : "Loading…"}
      </p>
    );
  }
  if (!data.brokerEnabled) {
    return (
      <p className="p-3 text-xs text-text-dim">
        Not available in this deployment mode (the credential broker runs only
        on the containerized server).
      </p>
    );
  }

  const store = () =>
    tryUI(async () => {
      if (!key.trim() || !value.trim()) return;
      await api.putProjectEnv(project, key.trim(), value);
      setKey("");
      setValue("");
      toast("success", "Stored — applies to new terminals");
      refresh();
    });

  return (
    <div className="flex h-full flex-col overflow-y-auto p-2 text-xs">
      <p className="px-1 pb-2 text-[11px] text-text-dim">
        Env variables for THIS project only — exported in every new login
        shell here, never in other projects. A project variable overrides a
        global one (Settings → Credentials) with the same name. Open a new
        terminal to pick up changes.
      </p>
      {keys.length === 0 && (
        <p className="px-1 pb-2 text-xs text-text-dim">No variables yet.</p>
      )}
      <div className="flex flex-col gap-0.5 pb-3">
        {keys.map((k) => (
          <EnvRow key={k} project={project} name={k} onRemoved={refresh} />
        ))}
      </div>
      <div className="mt-auto border-t border-border pt-2">
        <div className="flex flex-col gap-1.5 px-1">
          <input
            value={key}
            onChange={(e) => setKey(e.target.value.toUpperCase())}
            placeholder="MY_VARIABLE"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            className="rounded border border-border bg-bg px-2 py-1 font-mono"
          />
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && store()}
            placeholder="value"
            type="password"
            className="rounded border border-border bg-bg px-2 py-1 font-mono"
          />
          <button
            onClick={store}
            disabled={!key.trim() || !value.trim()}
            className="self-start rounded bg-accent-soft px-2.5 py-1 text-on-accent-soft hover:brightness-110 disabled:opacity-40"
          >
            Store
          </button>
          <span className="pb-1 text-[10px] text-text-dim">
            Storing the same name again updates it.
          </span>
        </div>
      </div>
    </div>
  );
}

function EnvRow({
  project,
  name,
  onRemoved,
}: {
  project: string;
  name: string;
  onRemoved: () => void;
}) {
  // null = hidden; "" while loading is fine (renders as revealed-empty briefly).
  const [revealed, setRevealed] = useState<string | null>(null);
  const toggle = () =>
    tryUI(async () => {
      if (revealed !== null) {
        setRevealed(null);
        return;
      }
      const { value } = await api.projectEnvValue(project, name);
      setRevealed(value);
    });
  return (
    <div className="group flex items-center gap-2 rounded px-1 py-1 hover:bg-bg-hover">
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-ok" />
      <button
        onClick={toggle}
        className="min-w-0 shrink-0 truncate font-mono font-medium hover:text-accent"
        title={revealed !== null ? "Hide value" : "Click to reveal value"}
      >
        {name}
      </button>
      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-text-dim">
        {revealed !== null ? revealed : "••••••"}
      </span>
      <button
        onClick={() =>
          tryUI(async () => {
            await api.deleteProjectEnv(project, name);
            onRemoved();
          })
        }
        className="shrink-0 text-[10px] text-text-dim opacity-0 hover:text-danger group-hover:opacity-100"
      >
        remove
      </button>
    </div>
  );
}
