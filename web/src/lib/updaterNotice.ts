// What the Settings page tells an admin about the host's auto-updater
// (ADR-081). The server hands over a closed set — a state, a code, two
// timestamps, two release tags — and every sentence shown is composed HERE
// from that set: nothing the updater, git or a remote wrote is ever rendered.
// BUG-084: the updater failed for ten days on a rejected token and nothing
// said so.

/** The `updater` key of the settings payload (internal/updatestatus). */
export interface UpdaterStatus {
  state: string; // "ok" | "failing" | "stale" | "unknown"
  code?: string;
  checkedAt?: string;
  since?: string;
  deployed?: string;
  latest?: string;
}

export interface UpdaterNotice {
  tone: "danger" | "warn";
  text: string;
}

/** The notice for an updater status, or null when there is nothing to say
 *  (no status at all, or a healthy one). */
export function updaterNotice(u: UpdaterStatus | undefined): UpdaterNotice | null {
  if (!u || u.state === "ok") return null;
  if (u.state === "stale") {
    return {
      tone: "warn",
      text: `Auto-update has not run since ${u.checkedAt || "an unknown time"}.`,
    };
  }
  if (u.state !== "failing") {
    return { tone: "warn", text: "Auto-update status cannot be read." };
  }
  const latest = u.latest || "the newest release";
  let why: string;
  switch (u.code) {
    case "fetch-auth":
      why = "GitHub rejected this host's token (/etc/styrman/gh-token)";
      break;
    case "fetch":
      why = "the host cannot reach the repository";
      break;
    case "no-tags":
      why = "the repository has no release tag";
      break;
    case "signature":
      why = `${latest} is not signed by a key this host trusts`;
      break;
    case "rolled-back":
      why = `${latest} did not come up and was rolled back`;
      break;
    case "deploy-failed":
      why = `${latest} did not come up and could not be rolled back`;
      break;
    case "latched":
      why = `${latest} failed earlier and is not retried`;
      break;
    default:
      // "error", and anything this page does not know: the same sentence.
      why = "the updater stopped with an error (see journalctl -u styrman-update.service)";
  }
  const since = u.since ? ` since ${u.since}` : "";
  // Not after a failed rollback: the updater's record still names the
  // previous tag while the host runs whatever the failed deploy left.
  const stays = u.deployed && u.code !== "deploy-failed" ? ` This host stays on ${u.deployed}.` : "";
  return { tone: "danger", text: `Auto-update has been failing${since}: ${why}.${stays}` };
}
