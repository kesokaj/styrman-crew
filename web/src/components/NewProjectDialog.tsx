import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { toast } from "../lib/toast";
import { api, projectNameError, type BackupMeta } from "../lib/api";

type Repo = "local" | "private" | "public" | "clone" | "backup";

export function NewProjectDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (project: string, termId: string) => void;
}) {
  const [name, setName] = useState("");
  const [repo, setRepo] = useState<Repo>("local");
  const [cloneUrl, setCloneUrl] = useState("");
  const [ghReady, setGhReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [backups, setBackups] = useState<BackupMeta[]>([]);
  // "projectId/backupId" of the chosen source backup.
  const [fromBackup, setFromBackup] = useState("");

  // GitHub options only make sense once gh is signed in (styrman-setup);
  // the From-backup option only once a backup exists (ADR-044).
  useEffect(() => {
    api
      .setupStatus()
      .then((s) => setGhReady(s.github))
      .catch(() => {});
    api
      .listAllBackups()
      .then((b) => setBackups(b.backups))
      .catch(() => {});
  }, []);

  const nameValid = !projectNameError(name);
  // Mirror the server's cloneURLRe so the client rejects the same shapes.
  const cloneValid = /^(https:\/\/|ssh:\/\/|git@)[\w.@:/~+-]+$/.test(cloneUrl);
  const valid =
    nameValid &&
    (repo !== "clone" || cloneValid) &&
    (repo !== "backup" || fromBackup !== "");

  const create = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const [srcProject, srcBackup] = fromBackup.split("/");
      const p = await api.createProject(name, {
        github: repo === "private" || repo === "public" ? repo : "none",
        cloneUrl: repo === "clone" ? cloneUrl.trim() : undefined,
        fromBackup:
          repo === "backup" && srcProject && srcBackup
            ? { project: srcProject, id: srcBackup }
            : undefined,
      });
      // The project EXISTS from here on. A failed first-terminal create must
      // not present as "creation failed" — retrying would then hit "name
      // already exists" (audit finding). Enter the project; a terminal can
      // be added there.
      try {
        const term = await api.createTerminal(p.id);
        onCreated(p.id, term.id);
      } catch {
        toast("info", "Project created — add a terminal from the sidebar");
        onCreated(p.id, "");
      }
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };

  const repoOptions: Array<[Repo, string, boolean]> = [
    ["local", "Empty git repo", false],
    ["private", "GitHub · private", !ghReady],
    ["public", "GitHub · public", !ghReady],
    ["clone", "Clone a repo", false],
    ["backup", "From backup", backups.length === 0],
  ];

  // Portal: a transformed ancestor (the sidebar drawer) would otherwise
  // become the containing block for position:fixed and shrink the dialog.
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="w-80 max-w-full rounded-3xl bg-bg-raised p-5 shadow-2xl"
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
          {name && !nameValid
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
                  ? "border-accent bg-accent-soft text-on-accent-soft"
                  : "border-border hover:bg-bg-hover"
              } disabled:cursor-not-allowed disabled:opacity-35`}
              title={
                !disabled
                  ? undefined
                  : value === "backup"
                    ? "No backups yet — create one from a project's ⋯ menu"
                    : "Sign in to GitHub first (styrman-setup)"
              }
            >
              {label}
            </button>
          ))}
        </div>
        {repo === "backup" && (
          <select
            aria-label="Backup to restore from"
            value={fromBackup}
            onChange={(e) => setFromBackup(e.target.value)}
            className="mb-3 w-full rounded-md border border-border bg-bg px-2 py-1.5 text-xs outline-none focus:border-accent"
          >
            <option value="">Choose a backup…</option>
            {backups.map((b) => (
              <option key={b.projectId + b.id} value={`${b.projectId}/${b.id}`}>
                {b.projectName} — {new Date(b.createdAt).toLocaleString()} (
                {(b.sizeBytes / 1024 / 1024).toFixed(1)} MB)
              </option>
            ))}
          </select>
        )}
        {repo === "clone" && (
          <input
            aria-label="Repository URL to clone"
            autoCapitalize="none"
            autoCorrect="off"
            autoComplete="off"
            spellCheck={false}
            value={cloneUrl}
            onChange={(e) => setCloneUrl(e.target.value)}
            placeholder="git@github.com:user/repo.git or https://…"
            className="mb-3 w-full rounded-md border border-border bg-bg px-2 py-1.5 text-xs outline-none focus:border-accent"
          />
        )}
        {!ghReady && (
          <p className="mb-3 text-[11px] text-text-dim">
            GitHub options unlock after the gh sign-in in first-time setup. Empty repos and public
            https clones need no credentials.
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
            className="rounded-md bg-accent px-3 py-1.5 font-medium text-on-accent disabled:opacity-40"
          >
            {busy ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  , document.body);
}
