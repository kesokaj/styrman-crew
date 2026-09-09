import type { Project } from "../lib/api";
import { lanChip } from "../lib/lan";

/**
 * LanBadge — the small standing-exception flag beside a project's name
 * (sidebar row, dashboard card): "LAN" in warn colours while the project is
 * on the local network, nothing while it is sandboxed. The derivation is
 * lib/lan.ts so vitest covers it; this is only the markup.
 */
export function LanBadge({ project, className = "" }: { project: Pick<Project, "lanAccess">; className?: string }) {
  const c = lanChip(project);
  if (!c.text) return null;
  return (
    <span
      className={`shrink-0 rounded px-1 text-[10px] ${c.className} ${className}`}
      title={c.title}
      data-lan="on"
    >
      {c.text}
    </span>
  );
}
