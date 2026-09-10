// The sidebar's state/error badge for a project, as a pure function so
// vitest covers the precedence (TODO close-out 2026-09-10, item 29 — the
// `deleteError` badge had no automated coverage; the LAN chip is separate,
// see lan.ts / LanBadge).

import type { Project } from "./api";

export type ProjectBadge = {
  text: string;
  /** Tailwind text colour class. */
  className: string;
  /** Hover text; the full error for the failed states. */
  title: string;
};

/**
 * projectBadge — one badge at most, worst first: a delete in flight hides
 * every error (the record is going away); a failed delete outranks a failed
 * repo setup (the project should not exist at all); provisioning is the
 * transient between the 201 and the clone finishing.
 */
export function projectBadge(
  p: Pick<Project, "state" | "deleteError" | "provisionError">,
): ProjectBadge | null {
  if (p.state === "deleting") {
    return { text: "deleting…", className: "text-text-dim", title: "deleting" };
  }
  if (p.deleteError) {
    return { text: "delete failed", className: "text-danger", title: p.deleteError };
  }
  if (p.provisionError) {
    return { text: "repo setup failed", className: "text-danger", title: p.provisionError };
  }
  if (p.state === "provisioning") {
    return {
      text: "provisioning…",
      className: "text-text-dim",
      title: "setting up the repository — the workspace is not a repo yet",
    };
  }
  return null;
}
