import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { api } from "../lib/api";
import { usePolled } from "../lib/hooks";

/**
 * Terminals waiting for attention, across all projects. A terminal "wants
 * you" when it rang the bell (agents ring BEL on questions/permission
 * prompts) while nobody was viewing it; opening the terminal clears it.
 */
export function NotificationBell() {
  const navigate = useNavigate();
  const { data } = usePolled(() => api.notifications(), 8000);
  const [open, setOpen] = useState(false);
  const items = data?.notifications ?? [];

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className={`relative rounded-md px-1.5 py-1 text-sm ${
          items.length ? "text-warn" : "text-text-dim hover:text-text"
        }`}
        title={
          items.length
            ? `${items.length} terminal${items.length > 1 ? "s" : ""} waiting for input`
            : "No terminals waiting"
        }
        aria-label="Notifications"
      >
        🔔
        {items.length > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-warn px-0.5 text-[9px] font-bold text-black">
            {items.length}
          </span>
        )}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div className="absolute left-0 top-8 z-40 w-64 rounded-lg border border-border bg-bg-raised p-1 shadow-2xl">
            <div className="px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
              Waiting for you
            </div>
            {items.length === 0 && (
              <p className="px-2 py-2 text-xs text-text-dim">
                Nothing right now. When an agent asks a question in an unopened
                terminal, it shows up here.
              </p>
            )}
            {items.map((n) => (
              <button
                key={`${n.projectId}/${n.terminalId}`}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-bg-hover"
                onClick={() => {
                  setOpen(false);
                  navigate({
                    to: "/p/$project/$term",
                    params: { project: n.projectId, term: n.terminalId },
                  });
                }}
              >
                <span className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-warn" />
                <span className="min-w-0 flex-1 truncate">
                  <span className="text-text">{n.projectName}</span>
                  <span className="text-text-dim"> / {n.title}</span>
                </span>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
