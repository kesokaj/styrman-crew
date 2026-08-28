import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { api, tryUI } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { ConfirmDialog } from "./ui";
import { PauseIcon, PlusIcon, XIcon } from "./icons";

/**
 * The project's terminals. Each is a durable record backed by a tmux session
 * — run several to work with several agents at once (ADR-016). Live = the
 * session (and whatever runs in it) exists; pausing kills the session but
 * keeps the terminal and its transcript, ready to start fresh.
 */
export function TerminalList({
  project,
  activeTerm,
  onNavigate,
}: {
  project: string;
  activeTerm?: string;
  onNavigate?: () => void;
}) {
  const navigate = useNavigate();
  const { data, refresh } = usePolled(() => api.listTerminals(project), 5000);
  const terminals = data?.terminals ?? [];
  const [confirming, setConfirming] = useState<
    { kind: "one"; id: string; title: string } | { kind: "all" } | null
  >(null);

  const open = (id: string) => {
    onNavigate?.();
    navigate({ to: "/p/$project/$term", params: { project, term: id } });
  };

  const create = () =>
    tryUI(async () => {
      const rec = await api.createTerminal(project);
      refresh();
      open(rec.id);
    });

  const pause = (id: string) =>
    tryUI(async () => {
      await api.pauseTerminal(project, id);
      refresh();
    });

  const remove = (id: string) =>
    tryUI(async () => {
      await api.deleteTerminal(project, id);
      refresh();
      if (activeTerm === id) navigate({ to: "/" });
    });

  const removeAll = () =>
    tryUI(async () => {
      await api.deleteAllTerminals(project);
      refresh();
      navigate({ to: "/" });
    });

  return (
    <div className="ml-4 border-l border-border pl-1">
      {terminals.map((t) => (
        <div
          key={t.id}
          className={`group flex cursor-pointer items-center gap-1.5 rounded-lg px-2 py-1 text-xs hover:bg-bg-hover ${
            activeTerm === t.id ? "bg-bg-active text-text" : "text-text-dim"
          }`}
          onClick={() => open(t.id)}
        >
          <span className="min-w-0 flex-1 truncate">{t.title}</span>
          {t.bell && (
            <span
              className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-warn"
              title="waiting for input"
            />
          )}
          {t.live ? (
            <button
              className="invisible inline-flex h-5 w-5 items-center justify-center rounded text-text-dim hover:text-warn group-hover:visible"
              onClick={(e) => {
                e.stopPropagation();
                pause(t.id);
              }}
              title="Pause: end the session, keep the terminal + transcript"
              aria-label={`Pause ${t.title}`}
            >
              <PauseIcon className="h-3 w-3" />
            </button>
          ) : (
            <span className="text-[9px] uppercase tracking-wide text-text-dim/60">
              paused
            </span>
          )}
          {t.live && (
            <span className="h-1 w-1 rounded-full bg-ok" title="live session" />
          )}
          <button
            className="invisible inline-flex h-5 w-5 items-center justify-center rounded text-text-dim hover:text-danger group-hover:visible"
            onClick={(e) => {
              e.stopPropagation();
              setConfirming({ kind: "one", id: t.id, title: t.title });
            }}
            aria-label={`Delete ${t.title}`}
          >
            <XIcon className="h-3 w-3" />
          </button>
        </div>
      ))}
      <div className="flex items-center">
        <button
          className="inline-flex flex-1 items-center gap-1 rounded-lg px-2 py-1 text-left text-xs text-accent hover:bg-bg-hover"
          onClick={create}
        >
          <PlusIcon className="h-3 w-3" />
          New terminal
        </button>
        {terminals.length > 1 && (
          <button
            className="rounded-lg px-2 py-1 text-[10px] text-text-dim hover:text-danger"
            onClick={() => setConfirming({ kind: "all" })}
            title="Close all terminals"
          >
            close all
          </button>
        )}
      </div>

      {confirming?.kind === "one" && (
        <ConfirmDialog
          title={`Delete “${confirming.title}”?`}
          body="Its session ends; the transcript stays in History."
          confirmLabel="Delete"
          danger
          onConfirm={() => remove(confirming.id)}
          onClose={() => setConfirming(null)}
        />
      )}
      {confirming?.kind === "all" && (
        <ConfirmDialog
          title={`Close all ${terminals.length} terminals?`}
          body="Sessions end; transcripts stay in History."
          confirmLabel="Close all"
          danger
          onConfirm={removeAll}
          onClose={() => setConfirming(null)}
        />
      )}
    </div>
  );
}
