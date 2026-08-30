import { useState, useSyncExternalStore } from "react";
import { clearClips, getClips, subscribeClips, type Clip } from "../lib/clips";
import { toast } from "../lib/toast";

/**
 * Everything copied inside crew (terminal selections, Ctrl+C) — the OS
 * clipboard can't be read back by a web app, so crew remembers what it put
 * there. Stored per-browser, never uploaded.
 */
export function ClipsTab() {
  const [copiedAt, setCopiedAt] = useState<number>();
  // N8: subscribe via useSyncExternalStore (tear-free under React 19).
  const clips = useSyncExternalStore(subscribeClips, getClips);

  const recopy = (c: Clip) =>
    navigator.clipboard?.writeText(c.text).then(
      () => {
        setCopiedAt(c.at);
        setTimeout(() => setCopiedAt(undefined), 1200);
      },
      () => toast("error", "Could not copy — select and copy manually"),
    );

  const fmt = (at: number) => {
    const d = new Date(at);
    return `${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center border-b border-border px-2 py-1.5">
        <p className="flex-1 text-[11px] text-text-dim">
          Copied in crew, newest first — kept for an hour, this tab only. Click
          to copy again.
        </p>
        {clips.length > 0 && (
          <button
            onClick={clearClips}
            className="text-[11px] text-text-dim hover:text-danger"
          >
            clear
          </button>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-1.5">
        {clips.length === 0 && (
          <p className="px-1 text-xs text-text-dim">
            Nothing yet. Select text in a terminal — it's copied the moment you
            release, and lands here too.
          </p>
        )}
        {clips.map((c) => (
          <button
            key={c.at}
            onClick={() => recopy(c)}
            className="group mb-0.5 flex w-full items-start gap-2 rounded px-1.5 py-1 text-left text-[11px] hover:bg-bg-hover"
            title={c.text.length > 400 ? c.text.slice(0, 400) + "…" : c.text}
          >
            <span className="shrink-0 pt-px text-[10px] text-text-dim">
              {fmt(c.at)}
            </span>
            <span className="min-w-0 flex-1 break-all font-mono leading-snug">
              {c.text.length > 160 ? c.text.slice(0, 160) + "…" : c.text}
            </span>
            <span className="invisible shrink-0 text-accent group-hover:visible">
              {copiedAt === c.at ? "✓" : "copy"}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
