import { useState } from "react";
import { FileExplorer } from "./FileExplorer";
import { XIcon } from "./icons";
import { PortsTab } from "./SidePanelPorts";
import { HistoryTab } from "./SidePanelHistory";
import { GitTab } from "./SidePanelGit";
import { ClipsTab } from "./SidePanelClips";
import { EnvTab } from "./SidePanelEnv";

type Tab = "files" | "ports" | "env" | "history" | "git" | "clips";
const TABS: Array<[Tab, string]> = [
  ["files", "Files"],
  ["ports", "Ports"],
  ["env", "Env"],
  ["history", "History"],
  ["git", "Git"],
  ["clips", "Clipboard"],
];

/**
 * Right-hand context panel for the open project: file explorer, exposed
 * ports, terminal transcripts, and the working-tree diff. Terminals are NOT
 * here — they live in the sidebar, because a terminal is the main surface,
 * not a side note (ADR-016). Each tab's implementation lives in its own
 * SidePanel*.tsx file; this is just the tab shell.
 */
export function SidePanel({
  project,
  term,
  projectName,
}: {
  project: string;
  term: string;
  /** Friendly name for display (the Files crumb); id stays the identity. */
  projectName?: string;
}) {
  // Desktop: inline panel, open by default. Below xl it OVERLAYS the
  // terminal, so it starts collapsed there (the route is client-rendered —
  // never prerendered — so reading the viewport in the initializer is safe).
  const [open, setOpen] = useState(
    () =>
      typeof window === "undefined" ||
      window.matchMedia("(min-width: 1280px)").matches,
  );
  const [tab, setTab] = useState<Tab>("files");

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="flex h-full w-8 shrink-0 items-center justify-center border-l border-border bg-bg-raised text-text-dim hover:text-text"
        title="Show panel"
      >
        <span className="rotate-90 whitespace-nowrap text-[11px] tracking-wider">
          PANEL
        </span>
      </button>
    );
  }

  return (
    <aside className="fixed inset-y-0 right-0 z-40 flex h-full w-80 max-w-[85vw] shrink-0 flex-col border-l border-border bg-bg-raised shadow-2xl xl:static xl:z-auto xl:max-w-none xl:shadow-none">
      <div className="flex items-center border-b border-border">
        {TABS.map(([k, label]) => (
          <button
            key={k}
            onClick={() => setTab(k)}
            className={`px-2.5 py-2 text-xs ${
              tab === k
                ? "border-b-2 border-accent font-medium text-text"
                : "text-text-dim hover:text-text"
            }`}
          >
            {label}
          </button>
        ))}
        <button
          onClick={() => setOpen(false)}
          className="ml-auto inline-flex h-7 w-7 items-center justify-center rounded-lg text-text-dim hover:bg-bg-hover hover:text-text"
          title="Hide panel"
          aria-label="Hide panel"
        >
          <XIcon className="h-3.5 w-3.5" />
        </button>
      </div>
      <div className="min-h-0 flex-1">
        {tab === "files" && (
          <FileExplorer project={project} projectName={projectName} />
        )}
        {tab === "ports" && <PortsTab project={project} term={term} />}
        {tab === "env" && <EnvTab project={project} />}
        {tab === "history" && <HistoryTab project={project} />}
        {tab === "git" && <GitTab project={project} />}
        {tab === "clips" && <ClipsTab />}
      </div>
    </aside>
  );
}
