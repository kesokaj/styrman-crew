// Per-project local-network access (ADR-065): the pure derivations the
// sidebar chip, the dashboard card and the Ports-tab block share. Kept free
// of React so vitest covers every branch (web/src/lib/lan.test.ts).

import type { LanState, Project } from "./api";

export type LanChip = {
  /** The visible word; empty = render nothing. */
  text: string;
  /** Tailwind classes for the chip. */
  className: string;
  /** Hover text: what the chip means, or root's message. */
  title: string;
  /** True while the state is transient (pulse). */
  pending: boolean;
};

const NONE: LanChip = { text: "", className: "", title: "", pending: false };

/**
 * lanChip — a standing exception is FLAGGED, not celebrated: warn colours
 * while on or pending, danger while root reports an error, nothing while off
 * or when the host cannot do it at all (the Ports tab says why).
 */
export function lanChip(p: Pick<Project, "lanAccess" | "lanState" | "lanMessage">, lanCidrs?: string[]): LanChip {
  const where = lanCidrs && lanCidrs.length > 0 ? lanCidrs.join(", ") : "your local network";
  switch (p.lanState) {
    case "on":
      return { text: "LAN", className: "bg-warn-soft text-warn", title: `Local network access allowed: ${where}`, pending: false };
    case "pending":
      return {
        text: "LAN",
        className: "bg-warn-soft text-warn",
        title: p.lanAccess ? `Local network access: applying on the host firewall…` : "Local network access: closing…",
        pending: true,
      };
    case "error":
      return { text: "LAN", className: "text-danger", title: p.lanMessage || "Host firewall reported an error", pending: false };
    default:
      return NONE;
  }
}

/** lanLine — the one-line explanation under the switch, per state. */
export function lanLine(state: LanState | undefined, lanAccess: boolean, message: string, lanCidrs?: string[], appliedAt?: string): string {
  const where = lanCidrs && lanCidrs.length > 0 ? lanCidrs.join(", ") : "the operator's local network";
  switch (state) {
    case "on": {
      const when = appliedAt ? ` Applied ${formatClock(appliedAt)}.` : "";
      return `Can reach ${where} by IP (LAN names and .local do not resolve). Still blocked: this server, other projects, cloud metadata.${when}`;
    }
    case "pending":
      if (!lanAccess) return "Closing on the host firewall — open LAN connections drop immediately.";
      return message.includes("next starts")
        ? "Saved — applies when the project next starts."
        : "Applying on the host firewall — usually under a second, up to a minute.";
    case "error":
      return `Host firewall reported: ${message || "unknown error"}`;
    case "unavailable":
      return `Not available on this server: ${message || "host firewall support is not installed"}`;
    default:
      return "Internet only (default). This project cannot reach your local network, this server, or other projects.";
  }
}

/** formatClock — HH:MM:SS in the viewer's locale; input is an RFC 3339 stamp. */
export function formatClock(iso: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  return new Date(t).toLocaleTimeString();
}
