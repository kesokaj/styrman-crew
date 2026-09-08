import type { Project } from "../lib/api";
import { lanChip } from "../lib/lan";

/**
 * LanBadge — the small standing-exception flag beside a project's name
 * (sidebar row, dashboard card): "LAN" in warn colours while on or pending,
 * danger while root reports an error, nothing otherwise. The derivation is
 * lib/lan.ts so vitest covers it; this is only the markup.
 */
export function LanBadge({ project, className = "" }: { project: Pick<Project, "lanAccess" | "lanState" | "lanMessage">; className?: string }) {
  const c = lanChip(project);
  if (!c.text) return null;
  return (
    <span
      className={`shrink-0 rounded px-1 text-[10px] ${c.className} ${c.pending ? "animate-pulse" : ""} ${className}`}
      title={c.title}
      data-lan={project.lanState}
    >
      {c.text}
    </span>
  );
}
