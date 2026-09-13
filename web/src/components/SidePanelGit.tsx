import { useEffect, useState, type ReactNode } from "react";
import {
  api,
  type GitBranches,
  type GitCommit,
  type GitFile,
  type GitStashEntry,
} from "../lib/api";
import { usePolled } from "../lib/hooks";
import { toast } from "../lib/toast";
import { ConfirmDialog, PromptDialog } from "./ui";
import {
  evictCleanProjectPanes,
  notifyFileSaved,
  openDiffInPane,
} from "../lib/editorPane";

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
  const [branches, setBranches] = useState<GitBranches | null>(null);
  const [branchOpen, setBranchOpen] = useState(false);
  const [newBranch, setNewBranch] = useState(false);
  const [log, setLog] = useState<GitCommit[] | null>(null);
  const [stashes, setStashes] = useState<GitStashEntry[]>([]);
  const [showLog, setShowLog] = useState(false);

  const files = data?.files ?? [];

  // Branches and stashes are cheap and only interesting while the repo is up.
  // They are NOT on the 8s status poll: a dropdown that reshuffles under the
  // pointer is worse than one that is a few seconds stale.
  const running = data?.state === "running" && data.repo;
  useEffect(() => {
    if (!running) return;
    void api.gitBranches(project).then(setBranches).catch(() => {});
    void api
      .gitStashList(project)
      .then((r) => setStashes(r.entries))
      .catch(() => {});
  }, [project, running, data?.branch]);

  useEffect(() => {
    if (!running || !showLog) return;
    void api
      .gitLog(project, 50)
      .then((r) => setLog(r.commits))
      .catch(() => {});
  }, [project, running, showLog, data?.ahead]);
  const staged = files.filter((f) => f.staged && f.staged !== "?");
  const changed = files.filter((f) => f.unstaged && f.unstaged !== "");

  /**
   * Called after any verb that REWRITES the working tree (checkout, pull,
   * stash push/pop). Open editor buffers now predate the tree; clean ones are
   * dropped so the next open re-reads, and a dirty one is reported rather than
   * discarded — Save would otherwise write pre-switch content over the new
   * tree (release review, 2026-09-04).
   */
  const treeChanged = () => {
    const staleDirty = evictCleanProjectPanes(project);
    notifyFileSaved(); // the explorer re-lists; the tree moved under it too
    if (staleDirty > 0)
      toast(
        "error",
        `${staleDirty} editor buffer${staleDirty === 1 ? "" : "s"} still hold${staleDirty === 1 ? "s" : ""} pre-change content — saving would overwrite the new tree`,
      );
  };

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
        <div className="relative">
          <button
            onClick={() => setBranchOpen((v) => !v)}
            aria-expanded={branchOpen}
            className="rounded px-1 py-0.5 font-medium hover:bg-bg-hover"
            title="Switch branch"
          >
            {data.branch || "main"} ▾
          </button>
          {branchOpen && (
            <div className="absolute left-0 top-full z-20 mt-1 max-h-64 w-56 overflow-y-auto rounded-lg border border-border bg-bg-raised py-1 shadow-2xl">
              <button
                onClick={() => {
                  setBranchOpen(false);
                  setNewBranch(true);
                }}
                className="block w-full px-2 py-1 text-left text-accent hover:bg-bg-hover"
              >
                + New branch from here
              </button>
              {(branches?.local ?? []).map((b) => (
                <button
                  key={b}
                  onClick={() => {
                    setBranchOpen(false);
                    run(async () => {
                      await api.gitCheckout(project, b);
                      treeChanged();
                      toast("success", `Switched to ${b}`);
                    });
                  }}
                  className={`block w-full truncate px-2 py-1 text-left hover:bg-bg-hover ${
                    b === branches?.current
                      ? "font-medium text-on-accent-soft"
                      : "text-text-dim"
                  }`}
                >
                  {b}
                </button>
              ))}
              {(branches?.remote ?? []).length > 0 && (
                <div className="mt-1 border-t border-border px-2 pt-1 text-[10px] uppercase tracking-wide text-text-dim">
                  Remote
                </div>
              )}
              {(branches?.remote ?? []).map((b) => (
                <div
                  key={b}
                  className="truncate px-2 py-1 text-text-dim"
                  title="Remote-tracking ref — fetch, then create a local branch"
                >
                  {b}
                </div>
              ))}
            </div>
          )}
        </div>
        <span className="text-text-dim">{track}</span>
        <button
          onClick={() =>
            run(async () => {
              await api.gitFetch(project);
              // Remote-tracking refs moved; the dropdown reads them.
              setBranches(await api.gitBranches(project));
              toast("success", "Fetched");
            })
          }
          disabled={busy}
          className="rounded px-2 py-0.5 text-text-dim hover:bg-bg-hover hover:text-text disabled:opacity-40"
          title="git fetch --all (updates remote refs, touches no files; no --prune by design)"
        >
          Fetch
        </button>
        <button
          onClick={() =>
            run(async () => {
              await api.gitPull(project);
              treeChanged();
              toast("success", "Pulled");
            })
          }
          disabled={busy}
          className="rounded px-2 py-0.5 text-text-dim hover:bg-bg-hover hover:text-text disabled:opacity-40"
          title="git pull --ff-only"
        >
          Pull{data.behind > 0 ? ` ${data.behind}` : ""}
        </button>
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
        <div className="border-t border-border">
          <button
            onClick={() => setShowLog((v) => !v)}
            aria-expanded={showLog}
            className="flex w-full items-center gap-1 px-2 py-1 text-[10px] uppercase tracking-wide text-text-dim hover:bg-bg-hover hover:text-text"
          >
            {showLog ? "▾" : "▸"} Commits
          </button>
          {showLog &&
            (log === null ? (
              <p className="px-2 pb-2 text-xs text-text-dim">Loading…</p>
            ) : log.length === 0 ? (
              <p className="px-2 pb-2 text-xs text-text-dim">No commits yet.</p>
            ) : (
              log.map((c) => (
                <button
                  key={c.hash}
                  // Pass the subject WITH the hash: the pane renders its
                  // header from what it was handed, never a second fetch.
                  onClick={() =>
                    openDiffInPane(project, term, {
                      commit: c.hash,
                      subject: c.subject,
                      staged: false,
                      untracked: false,
                    })
                  }
                  className="flex w-full items-baseline gap-2 px-2 py-0.5 text-left text-xs hover:bg-bg-hover"
                  title={`${c.hash}\n${c.author} · ${c.date}`}
                >
                  <span className="shrink-0 font-mono text-[10px] text-accent">
                    {c.shortHash}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{c.subject}</span>
                </button>
              ))
            ))}
        </div>

        <div className="border-t border-border">
          <div className="flex items-center gap-2 px-2 py-1">
            <span className="text-[10px] uppercase tracking-wide text-text-dim">
              Stash ({stashes.length})
            </span>
            <button
              onClick={() =>
                run(async () => {
                  await api.gitStash(project, "push");
                  setStashes((await api.gitStashList(project)).entries);
                  treeChanged();
                  toast("success", "Stashed (including untracked files)");
                })
              }
              disabled={busy || data.clean}
              className="ml-auto text-xs text-accent hover:brightness-110 disabled:opacity-40"
            >
              Stash all
            </button>
            <button
              onClick={() =>
                run(async () => {
                  // finally: a CONFLICTING pop answers 409 but has already
                  // rewritten the tree with markers, so open editor buffers
                  // are stale either way (audit 2026-09-13).
                  try {
                    await api.gitStash(project, "pop");
                    setStashes((await api.gitStashList(project)).entries);
                    toast("success", "Popped");
                  } finally {
                    treeChanged();
                  }
                })
              }
              disabled={busy || stashes.length === 0}
              className="text-xs text-text-dim hover:text-text disabled:opacity-40"
              // A conflicting pop leaves markers in the tree AND keeps the
              // entry; say so up front rather than after it happens.
              title="git stash pop — on conflict the tree gets conflict markers and the entry is KEPT"
            >
              Pop
            </button>
          </div>
          {stashes.map((e) => (
            <div
              key={e.ref}
              className="flex items-baseline gap-2 px-2 py-0.5 text-xs"
            >
              <span className="shrink-0 font-mono text-[10px] text-text-dim">
                {e.ref}
              </span>
              <span className="min-w-0 flex-1 truncate text-text-dim">
                {e.subject}
              </span>
            </div>
          ))}
        </div>
      </div>

      {newBranch && (
        <PromptDialog
          title="New branch"
          label="Created from the current HEAD and checked out."
          submitLabel="Create"
          // UX ONLY — NOT a security boundary. The server re-validates every
          // branch name in validBranchName (git.go) before any git call, and
          // that is the gate that counts. This exists so the common mistakes
          // are caught without a round trip. It is kept in step with the
          // server's rules (leading dash, C0/C1/DEL controls, space, 255
          // bytes) so the two do not silently disagree; if they ever do, the
          // server wins and the drift is in the safe direction.
          validate={(v) =>
            v.trim() === ""
              ? "Enter a name"
              : new TextEncoder().encode(v).length > 255
                ? "Too long (max 255 bytes)"
                : // eslint-disable-next-line no-control-regex
                  /^-|[\x00-\x1f\x7f\u0080-\u009f ]/.test(v)
                  ? "No spaces, control characters, or a leading dash"
                  : undefined
          }
          onSubmit={(name) =>
            run(async () => {
              await api.gitCheckout(project, name.trim(), true);
              treeChanged();
              setNewBranch(false);
              toast("success", `Created and switched to ${name.trim()}`);
            })
          }
          onClose={() => setNewBranch(false)}
        />
      )}

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
            run(async () => {
              // Discard rewrites the working tree server-side: evict the path
              // from open editor panes like every other tree-mutating verb,
              // or a stale buffer's next Save silently undoes the discard
              // (audit 2026-09-13).
              try {
                await api.gitDiscard(project, discarding.path, discarding.untracked);
              } finally {
                treeChanged();
              }
            })
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
