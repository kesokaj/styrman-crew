import { useState, type ReactNode } from "react";
import { api, type GitFile } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { toast } from "../lib/toast";
import { ConfirmDialog } from "./ui";
import { openDiffInPane } from "../lib/editorPane";

export function GitTab({
  project,
  term,
}: {
  project: string;
  /** The terminal whose main pane diffs open in. */
  term: string;
}) {
  const { data, refresh, error } = usePolled(
    () => api.gitStatus(project),
    8000,
    [project],
  );
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  const [sel, setSel] = useState<GitFile | null>(null);
  const [selStaged, setSelStaged] = useState(false);
  const [discarding, setDiscarding] = useState<GitFile | null>(null);
  const [fastCommitting, setFastCommitting] = useState(false);

  const files = data?.files ?? [];
  const staged = files.filter((f) => f.staged && f.staged !== "?");
  const changed = files.filter((f) => f.unstaged && f.unstaged !== "");

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      refresh();
    } catch (e) {
      toast("error", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // The diff opens in the MAIN pane, not in a 64px-tall box down here
  // (operator request 2026-09-04). This only records which row is highlighted;
  // DiffPane does the fetching, so a slow diff no longer blocks the panel.
  const view = (f: GitFile, isStaged: boolean) => {
    setSel(f);
    setSelStaged(isStaged);
    openDiffInPane(project, term, {
      path: f.path,
      staged: isStaged,
      untracked: f.untracked,
    });
  };

  if (!data) {
    if (error) return <p className="p-2 text-xs text-danger">{error}</p>;
    return <p className="p-2 text-xs text-text-dim">Loading…</p>;
  }
  if (data.state !== "running") {
    return (
      <p className="p-2 text-xs text-text-dim">Start the project to use git.</p>
    );
  }
  if (!data.repo) {
    return (
      <p className="p-2 text-xs text-text-dim">
        Not a git repository yet — it initializes on first boot.
      </p>
    );
  }

  const track =
    data.ahead > 0 || data.behind > 0
      ? `${data.ahead > 0 ? `↑${data.ahead}` : ""}${data.behind > 0 ? ` ↓${data.behind}` : ""}`.trim()
      : data.upstream
        ? "up to date"
        : "no upstream";

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-2 border-b border-border px-2 py-1.5 text-xs">
        <span className="font-medium">{data.branch || "main"}</span>
        <span className="text-text-dim">{track}</span>
        <button
          onClick={() =>
            run(async () => {
              await api.gitPush(project);
              toast("success", "Pushed");
            })
          }
          disabled={busy}
          className="ml-auto rounded bg-accent-soft px-2 py-0.5 text-on-accent-soft hover:brightness-110 disabled:opacity-40"
          title={
            data.upstream ? "git push" : `git push -u origin ${data.branch || "main"}`
          }
        >
          Push{data.ahead > 0 ? ` ${data.ahead}` : ""}
        </button>
        <button
          onClick={() =>
            openDiffInPane(project, term, { staged: false, untracked: false })
          }
          disabled={busy || data.clean}
          className="rounded px-2 py-0.5 text-text-dim hover:bg-bg-hover hover:text-text disabled:opacity-40"
          title="Show the whole working tree's diff in the main pane"
        >
          Diff
        </button>
        <button
          onClick={refresh}
          disabled={busy}
          className="rounded px-1 text-text-dim hover:text-text"
          title="Refresh"
        >
          ⟳
        </button>
      </div>

      <div className="border-b border-border p-2">
        <textarea
          value={msg}
          onChange={(e) => setMsg(e.target.value)}
          placeholder={`Commit message${staged.length ? ` (${staged.length} staged)` : ""}`}
          rows={2}
          className="w-full resize-none rounded border border-border bg-bg px-2 py-1 text-xs outline-none focus:border-accent"
        />
        <div className="mt-1 flex items-center gap-2">
          <button
            onClick={() =>
              run(async () => {
                await api.gitCommit(project, msg.trim());
                setMsg("");
                toast("success", "Committed");
              })
            }
            disabled={busy || staged.length === 0 || !msg.trim()}
            className="rounded bg-accent px-2.5 py-1 text-xs font-medium text-on-accent hover:brightness-110 disabled:opacity-40"
          >
            Commit
          </button>
          {changed.length > 0 && (
            <button
              onClick={() => run(() => api.gitStage(project, { all: true }))}
              disabled={busy}
              className="text-xs text-accent hover:brightness-110 disabled:opacity-40"
            >
              Stage all
            </button>
          )}
          {staged.length > 0 && (
            <button
              onClick={() => run(() => api.gitUnstage(project, { all: true }))}
              disabled={busy}
              className="text-xs text-text-dim hover:text-text disabled:opacity-40"
            >
              Unstage all
            </button>
          )}
          {!data.clean && (
            <button
              onClick={() => setFastCommitting(true)}
              disabled={busy}
              className="ml-auto text-xs text-text-dim hover:text-text disabled:opacity-40"
              title="Stage everything and commit it with a generated UUID message"
            >
              Fast commit
            </button>
          )}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {data.clean && (
          <p className="p-3 text-center text-xs text-text-dim">
            Nothing to commit — working tree clean.
          </p>
        )}
        {staged.length > 0 && (
          <FileGroup title={`Staged (${staged.length})`}>
            {staged.map((f) => (
              <GitFileRow
                key={"s:" + f.path}
                file={f}
                code={f.staged}
                active={sel?.path === f.path && selStaged}
                onView={() => view(f, true)}
                actions={
                  <ActBtn
                    label="−"
                    title="Unstage"
                    onClick={() =>
                      run(() => api.gitUnstage(project, { path: f.path }))
                    }
                  />
                }
              />
            ))}
          </FileGroup>
        )}
        {changed.length > 0 && (
          <FileGroup title={`Changes (${changed.length})`}>
            {changed.map((f) => (
              <GitFileRow
                key={"c:" + f.path}
                file={f}
                code={f.untracked ? "?" : f.unstaged}
                active={sel?.path === f.path && !selStaged}
                onView={() => view(f, false)}
                actions={
                  <>
                    <ActBtn
                      label="↩"
                      title="Discard changes"
                      danger
                      onClick={() => setDiscarding(f)}
                    />
                    <ActBtn
                      label="+"
                      title="Stage"
                      onClick={() =>
                        run(() => api.gitStage(project, { path: f.path }))
                      }
                    />
                  </>
                }
              />
            ))}
          </FileGroup>
        )}
      </div>

      {fastCommitting && (
        <ConfirmDialog
          title="Commit everything?"
          body={`All ${files.length} changed file${files.length === 1 ? "" : "s"} — including any you have not looked at — will be staged and committed with a generated UUID as the message. Nothing is pushed.`}
          confirmLabel="Commit everything"
          onConfirm={() =>
            run(async () => {
              // Generated client-side so the toast can show the id you will
              // grep the log for. crypto.randomUUID is available in every
              // browser this SPA supports and over HTTPS/localhost only, which
              // is the deployment (R8: the ingress terminates TLS).
              const message = crypto.randomUUID();
              await api.gitCommit(project, message, { all: true });
              setFastCommitting(false);
              toast("success", `Committed as ${message}`);
            })
          }
          onClose={() => setFastCommitting(false)}
        />
      )}

      {discarding && (
        <ConfirmDialog
          title={`Discard changes to “${discarding.path}”?`}
          body={
            discarding.untracked
              ? "This untracked file will be deleted. This cannot be undone."
              : "Your uncommitted changes to this file will be lost. This cannot be undone."
          }
          confirmLabel="Discard"
          danger
          onConfirm={() =>
            run(() =>
              api.gitDiscard(project, discarding.path, discarding.untracked),
            )
          }
          onClose={() => setDiscarding(null)}
        />
      )}
    </div>
  );
}

const GIT_CODE_COLOR: Record<string, string> = {
  M: "text-warn",
  A: "text-ok",
  D: "text-danger",
  R: "text-accent",
  C: "text-accent",
  "?": "text-text-dim",
  U: "text-danger",
};

function GitFileRow({
  file,
  code,
  active,
  onView,
  actions,
}: {
  file: GitFile;
  code: string;
  active: boolean;
  onView: () => void;
  actions: ReactNode;
}) {
  return (
    <div
      className={`group flex cursor-pointer items-center gap-2 px-2 py-0.5 text-xs hover:bg-bg-hover ${active ? "bg-bg-hover" : ""}`}
      onClick={onView}
    >
      <span
        className={`w-3 shrink-0 text-center font-mono ${GIT_CODE_COLOR[code] || "text-text-dim"}`}
      >
        {code}
      </span>
      <span className="min-w-0 flex-1 truncate font-mono" title={file.path}>
        {file.origPath ? `${file.origPath} → ${file.path}` : file.path}
      </span>
      <span
        className="flex shrink-0 items-center gap-1 opacity-0 group-hover:opacity-100"
        onClick={(e) => e.stopPropagation()}
      >
        {actions}
      </span>
    </div>
  );
}

function FileGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="py-1">
      <div className="px-2 pb-0.5 text-[10px] font-medium uppercase tracking-wider text-text-dim">
        {title}
      </div>
      {children}
    </div>
  );
}

function ActBtn({
  label,
  title,
  onClick,
  danger,
}: {
  label: string;
  title: string;
  onClick: () => void;
  danger?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className={`inline-flex h-5 w-5 items-center justify-center rounded font-mono text-sm hover:bg-bg-active ${danger ? "text-danger" : "text-accent"}`}
    >
      {label}
    </button>
  );
}
