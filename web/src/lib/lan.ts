// Per-project network mode (ADR-067): the pure derivations the sidebar chip,
// the dashboard card and the Ports-tab block share. Kept free of React so
// vitest covers every branch (web/src/lib/lan.test.ts).
//
// There are two states, not five. ADR-065/066 needed off/pending/on/error/
// unavailable because the mode was a request to the root firewall that could
// be in flight, refused, or unreportable. The mode is now the bridge
// interface name the server picks, so the record IS the state.

import type { Project } from "./api";

export type LanChip = {
  /** The visible word; empty = render nothing. */
  text: string;
  /** Tailwind classes for the chip. */
  className: string;
  /** Hover text: what the chip means. */
  title: string;
};

const NONE: LanChip = { text: "", className: "", title: "" };

/**
 * lanChip — a standing exception is FLAGGED, not celebrated: warn colours
 * while a project is on the LAN, nothing while it is sandboxed. Sandbox is
 * the default and the safe state, so it gets no decoration.
 */
export function lanChip(p: Pick<Project, "lanAccess">): LanChip {
  if (!p.lanAccess) return NONE;
  return {
    text: "LAN",
    className: "bg-warn-soft text-warn",
    title: "On the local network: this project reaches whatever the host reaches",
  };
}

/**
 * lanLine — the one-sentence description of a mode, for the Ports-tab block.
 */
export function lanLine(lanAccess: boolean): string {
  return lanAccess
    ? "On the local network. This project reaches whatever this server reaches — other machines on the LAN, this host, and its published ports — exactly like a normal Docker container. It still cannot reach other projects, and nothing on the network can open a connection into it."
    : "Sandboxed. This project reaches the internet and nothing else: not this host, not the local network, not other projects.";
}

/**
 * preferOverride — the PUT's reply is authoritative until the poll catches up
 * with it, so a 5 s poll cannot briefly show the pre-change state after the
 * toast (the PortShare pattern).
 *
 * BUG-020 was the previous version of this function: it compared a monotonic
 * `lanChangedAt`, and equality on a timestamp meant an out-of-band change
 * never displaced a local one — the switch latched on the last PUT forever.
 * The timestamp is gone with ADR-067, and the state is one boolean, so
 * "caught up" is just "the server now says what we said" and the latch is
 * structurally impossible.
 */
export function preferOverride<T extends { lanAccess: boolean }>(
  override: T | undefined,
  data: T | undefined,
): T | undefined {
  if (!override) return data;
  if (!data) return override;
  return data.lanAccess === override.lanAccess ? data : override;
}
