import { useNavigate } from "@tanstack/react-router";
import { api, tryUI } from "../lib/api";
import { usePolled } from "../lib/hooks";

/**
 * The project's terminals. Each is a durable record backed by a tmux session
 * — run several to work with several agents at once (ADR-016).
 */
export function TerminalList({
  project,
  activeTerm,
}: {
  project: string;
  activeTerm?: string;
}) {
  const navigate = useNavigate();
  const { data, refresh } = usePolled(() => api.listTerminals(project), 5000);
  const terminals = data?.terminals ?? [];

  const open = (id: string) =>
    navigate({ to: "/p/$project/$term", params: { project, term: id } });

  const create = () =>
    tryUI(async () => {
      const rec = await api.createTerminal(project);
      refresh();
      open(rec.id);
    });

  const remove = async (id: string, title: string) => {
    if (
      !confirm(
        `Delete "${title}"? Its session ends; the transcript stays in History.`,
      )
    )
      return;
    await tryUI(async () => {
      await api.deleteTerminal(project, id);
      refresh();
      if (activeTerm === id) navigate({ to: "/" });
    });
  };

  return (
    <div className="ml-4 border-l border-border pl-1">
      {terminals.map((t) => (
        <div
          key={t.id}
          className={`group flex cursor-pointer items-center gap-1.5 rounded-md px-2 py-1 text-xs hover:bg-bg-hover ${
            activeTerm === t.id ? "bg-bg-active text-text" : "text-text-dim"
          }`}
          onClick={() => open(t.id)}
        >
          <span className="min-w-0 flex-1 truncate">{t.title}</span>
          {t.live && (
            <span className="h-1 w-1 rounded-full bg-ok" title="live session" />
          )}
          <button
            className="invisible text-text-dim hover:text-danger group-hover:visible"
            onClick={(e) => {
              e.stopPropagation();
              remove(t.id, t.title);
            }}
            aria-label={`Delete ${t.title}`}
          >
            ✕
          </button>
        </div>
      ))}
      <button
        className="w-full rounded-md px-2 py-1 text-left text-xs text-accent hover:bg-bg-hover"
        onClick={create}
      >
        + New terminal
      </button>
    </div>
  );
}
