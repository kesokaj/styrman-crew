import { useState } from "react";
import { api, tryUI, type ProjectNetwork } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { lanLine, preferOverride } from "../lib/lan";
import { toast } from "../lib/toast";
import { ConfirmDialog, Switch } from "./ui";

/**
 * Per-project network mode (ADR-067) — the control and what it costs.
 *
 * There is no host half to be pending, unavailable or in error: the mode is
 * the bridge interface name the server picks at `docker network create`, so
 * the record IS the state. What the switch does have to say honestly is that
 * applying it REBUILDS the project's container.
 *
 * Enable is admin-only (the LAN is the operator's); the owner may always
 * disable, because tightening is never refused.
 */
export function LocalNetwork({ project }: { project: string }) {
  const { data, error, refresh } = usePolled(
    () => api.projectNetwork(project),
    5000,
    [project],
  );
  // The PUT's reply is authoritative until the poll catches up with it — the
  // PortShare pattern. See preferOverride for why this cannot latch (BUG-020).
  const [override, setOverride] = useState<ProjectNetwork>();
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const view = preferOverride(override, data);

  const on = view?.lanAccess ?? false;
  // canEnable is the SERVER's answer, not something inferred from the shape
  // of the reply (BUG-019). Turning OFF is always allowed.
  const canEnable = view?.canEnable ?? false;
  const disabled = busy || !view || (!on && !canEnable);
  const title = !canEnable && !on
    ? "Only an operator (admin) can put a project on the local network."
    : on
      ? "Sandbox this project again"
      : "Put this project on the local network";

  const apply = (next: boolean) =>
    tryUI(async () => {
      setBusy(true);
      try {
        const res = await api.setProjectLan(project, next);
        setOverride(res);
        toast(
          "success",
          res.rebuilt
            ? next
              ? "On the local network — the project was rebuilt"
              : "Sandboxed again — the project was rebuilt"
            : next
              ? "On the local network — applies when the project next starts"
              : "Sandboxed — applies when the project next starts",
        );
        refresh();
      } finally {
        setBusy(false);
      }
    });

  const line = !view
    ? error
      ? "Could not load the network setting — retrying…"
      : "Loading…"
    : lanLine(on);

  return (
    <div className="mb-2 rounded border border-border px-2 py-1.5">
      <div className="flex items-center gap-2">
        <span
          className={`h-1.5 w-1.5 shrink-0 rounded-full ${on ? "bg-warn" : "bg-border"}`}
          title={on ? "on the local network" : "sandboxed"}
        />
        <span className="text-xs font-medium">Network</span>
        <span className="min-w-0 flex-1" />
        <span className="shrink-0 text-[11px] text-text-dim">{on ? "LAN" : "Sandbox"}</span>
        <Switch
          checked={on}
          onChange={(next) => (next ? setConfirm(true) : void apply(false))}
          disabled={disabled}
          label="Local network access"
          title={title}
        />
      </div>
      <p className="mt-1 text-[11px] text-text-dim">{line}</p>
      {busy && (
        <p className="mt-0.5 text-[11px] text-text-dim">
          Rebuilding the project… this takes about as long as a start.
        </p>
      )}
      {confirm && (
        <ConfirmDialog
          title="Put this project on the local network?"
          danger
          confirmLabel="Allow LAN access"
          onClose={() => setConfirm(false)}
          onConfirm={() => {
            setConfirm(false);
            void apply(true);
          }}
          body={
            <>
              <p>
                This project becomes an ordinary machine on your network.
                Everything it runs — the agent, its shells, its Docker
                containers — can open connections to anything this server can
                reach: other devices on the LAN,{" "}
                <span className="text-danger">this server itself, including its SSH service</span>
                , and the ports it publishes. Connections arrive as this
                server's own address, so any device that trusts this host by IP
                trusts this project too.
              </p>
              <p className="mt-2">
                Still blocked, either way: other projects, and inbound
                connections — nothing on the network can open a connection into
                this project.
              </p>
              <p className="mt-2">
                Applying this <span className="text-text">rebuilds the project's container</span>.
                Your home directory and workspace are untouched, and packages
                you installed are replayed; anything living only in the
                container's own filesystem is lost. Running terminals will
                reconnect.
              </p>
            </>
          }
        />
      )}
    </div>
  );
}
