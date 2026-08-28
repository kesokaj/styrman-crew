import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { api, tryUI, type Terminal } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { Button, ConfirmDialog, Dialog, TextInput } from "./ui";
import { BoltIcon, PauseIcon, PencilIcon, PlusIcon, XIcon } from "./icons";

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
  const [renaming, setRenaming] = useState<Terminal | null>(null);

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
          {t.command && (
            <span className="shrink-0" title={`Autostart: ${t.command}`}>
              <BoltIcon className="h-3 w-3 text-accent" />
            </span>
          )}
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
            className="invisible inline-flex h-5 w-5 items-center justify-center rounded text-text-dim hover:text-text group-hover:visible"
            onClick={(e) => {
              e.stopPropagation();
              setRenaming(t);
            }}
            aria-label={`Settings for ${t.title}`}
            title="Rename / autostart"
          >
            <PencilIcon className="h-3 w-3" />
          </button>
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

      {renaming && (
        <TerminalSettingsDialog
          terminal={renaming}
          onSave={(patch) =>
            tryUI(async () => {
              await api.updateTerminal(project, renaming.id, patch);
              refresh();
            })
          }
          onClose={() => setRenaming(null)}
        />
      )}
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

/** Name + autostart for one terminal. The autostart command runs when the
 *  tmux session is CREATED (first open, and re-opens after pause or a VM
 *  restart) and then drops to the shell — a reconnect never re-runs it. */
function TerminalSettingsDialog({
  terminal,
  onSave,
  onClose,
}: {
  terminal: Terminal;
  onSave: (patch: { title?: string; command?: string }) => void;
  onClose: () => void;
}) {
  const [title, setTitle] = useState(terminal.title);
  const [command, setCommand] = useState(terminal.command ?? "");
  const save = () => {
    const patch: { title?: string; command?: string } = {};
    if (title.trim() && title.trim() !== terminal.title)
      patch.title = title.trim();
    if (command.trim() !== (terminal.command ?? ""))
      patch.command = command.trim();
    if (Object.keys(patch).length) onSave(patch);
    onClose();
  };
  return (
    <Dialog title={`Terminal “${terminal.title}”`} onClose={onClose}>
      <label className="mb-1 block text-xs text-text-dim" htmlFor="ts-name">
        Name
      </label>
      <TextInput
        id="ts-name"
        autoFocus
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && save()}
      />
      <label
        className="mb-1 mt-3 block text-xs text-text-dim"
        htmlFor="ts-cmd"
      >
        Autostart command
      </label>
      <TextInput
        id="ts-cmd"
        value={command}
        onChange={(e) => setCommand(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && save()}
        placeholder="e.g. docker compose up"
        className="font-mono"
      />
      <p className="mt-1 text-[11px] text-text-dim">
        Runs when the session starts (first open, and re-opens after pause or
        a VM restart), then drops to the shell. Empty = plain shell.
      </p>
      <div className="mt-4 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="filled" onClick={save} disabled={!title.trim()}>
          Save
        </Button>
      </div>
    </Dialog>
  );
}
