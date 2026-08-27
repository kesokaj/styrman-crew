import { useEffect, useState } from "react";
import { api } from "../lib/api";

type Repo = "none" | "local" | "private" | "public";

export function NewProjectDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (project: string, termId: string) => void;
}) {
  const [name, setName] = useState("");
  const [repo, setRepo] = useState<Repo>("local");
  const [ghReady, setGhReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  // GitHub options only make sense once gh is signed in (styrman-setup).
  useEffect(() => {
    api
      .setupStatus()
      .then((s) => setGhReady(s.github))
      .catch(() => {});
  }, []);

  const valid = /^[a-z][a-z0-9-]{0,30}$/.test(name);

  const create = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const p = await api.createProject(
        name,
        repo !== "none",
        repo === "private" || repo === "public" ? repo : "none",
      );
      const term = await api.createTerminal(p.id);
      onCreated(p.id, term.id);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };

  const repoOptions: Array<[Repo, string, boolean]> = [
    ["local", "Local git repo", false],
    ["private", "GitHub · private", !ghReady],
    ["public", "GitHub · public", !ghReady],
    ["none", "No git", false],
  ];

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        className="w-80 rounded-lg border border-border bg-bg-raised p-4 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-3 text-sm font-semibold">New project</h2>
        <label className="mb-1 block text-xs text-text-dim" htmlFor="np-name">
          Name (lowercase, digits, dashes)
        </label>
        <input
          id="np-name"
          autoFocus
          autoCapitalize="none"
          autoCorrect="off"
          autoComplete="off"
          spellCheck={false}
          value={name}
          onChange={(e) => setName(e.target.value.toLowerCase())}
          onKeyDown={(e) => e.key === "Enter" && valid && !busy && create()}
          placeholder="my-project"
          className="mb-1 w-full rounded-md border border-border bg-bg px-2 py-1.5 outline-none focus:border-accent"
        />
        <p className="mb-3 min-h-4 text-[11px] text-warn">
          {name && !valid
            ? "Must start with a letter; only a-z, 0-9 and dashes."
            : " "}
        </p>

        <div className="mb-1 text-xs text-text-dim">Repository</div>
        <div className="mb-3 grid grid-cols-2 gap-1.5">
          {repoOptions.map(([value, label, disabled]) => (
            <button
              key={value}
              disabled={disabled}
              onClick={() => setRepo(value)}
              className={`rounded-md border px-2 py-1.5 text-left text-xs ${
                repo === value
                  ? "border-accent bg-accent-soft text-accent"
                  : "border-border hover:bg-bg-hover"
              } disabled:cursor-not-allowed disabled:opacity-35`}
              title={disabled ? "Sign in to GitHub first (styrman-setup)" : undefined}
            >
              {label}
            </button>
          ))}
        </div>
        {!ghReady && (
          <p className="mb-3 text-[11px] text-text-dim">
            GitHub options unlock after the gh sign-in in first-time setup. Local repos need no
            credentials.
          </p>
        )}
        <p className="mb-3 text-xs text-text-dim">
          Both agents are preinstalled — run <code>claude</code> or <code>agy</code> in any
          terminal.
        </p>
        {error && <p className="mb-3 text-xs text-danger">{error}</p>}
        <div className="flex justify-end gap-2 text-xs">
          <button onClick={onClose} className="rounded-md px-3 py-1.5 hover:bg-bg-hover">
            Cancel
          </button>
          <button
            onClick={create}
            disabled={!valid || busy}
            className="rounded-md bg-accent px-3 py-1.5 font-medium text-white disabled:opacity-40"
          >
            {busy ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}
