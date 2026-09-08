import { useState } from "react";
import { api, tryUI, type ProjectNetwork } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { lanLine } from "../lib/lan";
import { toast } from "../lib/toast";
import { ConfirmDialog, Switch } from "./ui";

/**
 * Per-project local-network access (ADR-065) — the control and its honest
 * state. Everything rendered here comes from the SERVER's derivation of the
 * record plus the root firewall's status file, never from the request that
 * changed it: a flip shows "pending" until root has applied it (usually
 * under a second), "error" when root says so, "unavailable" when the host
 * half is missing or outdated. Works for a stopped project — the record
 * exists whether or not the microVM does.
 *
 * Enable is admin-only (the LAN is the operator's); the reply carries `host`
 * only for admins, which is also how this component knows who is looking.
 */
export function LocalNetwork({ project }: { project: string }) {
  const { data, error, refresh } = usePolled(
    () => api.projectNetwork(project),
    5000,
    [project],
  );
  // The PUT's reply is authoritative until the poll reports the same change
  // (same lanChangedAt) — the PortShare pattern; without it a 5 s poll could
  // briefly show the pre-change state after the toast.
  const [override, setOverride] = useState<ProjectNetwork>();
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const view =
    override && data?.lanChangedAt !== override.lanChangedAt ? override : data;

  const admin = view?.host !== undefined;
  const on = view?.lanAccess ?? false;
  const state = view?.lanState;
  const cidrs = view?.host?.lanCidrs;
  const where = cidrs && cidrs.length > 0 ? cidrs.join(", ") : "the operator's local network";
  // While OFF the server derives "off" even when the host half is missing —
  // there is nothing to report about a rule that does not exist. The admin's
  // reply carries whether an enable WOULD be accepted (`host.available`), so
  // the switch is disabled up front with the reason instead of letting the
  // click bounce off a 503.
  const cannotEnable = !!view?.host && !view.host.available;
  const unavailableReason = view?.host && !view.host.available ? view.host.reason : view?.lanMessage ?? "";
  // Turning OFF is always allowed (tightening is never refused); turning ON
  // needs an admin and a host that can do it. Nothing flips while pending.
  const disabled =
    busy || !view || state === "pending" || (!on && (state === "unavailable" || cannotEnable || !admin));
  const title = !admin && !on
    ? "Only an operator (admin) can allow local network access."
    : (state === "unavailable" || cannotEnable) && !on
      ? `Not available on this server: ${unavailableReason}`
      : on
        ? "Turn local network access off"
        : "Allow local network access";

  const apply = (next: boolean) =>
    tryUI(async () => {
      setBusy(true);
      try {
        const res = await api.setProjectLan(project, next);
        setOverride(res);
        toast(
          "success",
          next
            ? "Local network access requested — applying on the host"
            : "Local network access removed — open LAN connections were cut",
        );
        refresh();
      } finally {
        setBusy(false);
      }
    });

  const dot =
    state === "on"
      ? "bg-warn"
      : state === "pending"
        ? "animate-pulse bg-warn"
        : state === "error"
          ? "bg-danger"
          : "bg-border";
  const line = !view
    ? error
      ? "Could not load the network setting — retrying…"
      : "Loading…"
    : !on && cannotEnable
      ? lanLine("unavailable", false, unavailableReason)
      : lanLine(state, on, view.lanMessage, cidrs, view.host?.appliedAt);

  return (
    <div className="mb-2 rounded border border-border px-2 py-1.5">
      <div className="flex items-center gap-2">
        <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${dot}`} title={state ?? "off"} />
        <span className="text-xs font-medium">Local network</span>
        <span className="min-w-0 flex-1" />
        {state === "error" && (
          <button
            className="shrink-0 text-[11px] text-accent hover:brightness-110 disabled:opacity-40"
            onClick={() => void apply(on)}
            disabled={busy}
            title="Ask the host firewall to apply the current setting again"
          >
            retry
          </button>
        )}
        <Switch
          checked={on}
          onChange={(next) => (next ? setConfirm(true) : void apply(false))}
          disabled={disabled}
          label="Local network access"
          title={title}
        />
      </div>
      <p className={`mt-1 text-[11px] ${state === "error" ? "text-danger" : "text-text-dim"}`}>
        {line}
      </p>
      {confirm && (
        <ConfirmDialog
          title={`Allow local network access?`}
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
                Everything this project runs — the agent, its shells, its Docker
                containers — will be able to open connections to devices on{" "}
                <span className="text-text">{where}</span> by IP address, and
                those connections arrive as this server's own address — so any
                device that trusts this host by IP trusts this project too.
              </p>
              <p className="mt-2">
                This project holds your git, SSH and cloud credentials through the
                broker; any LAN machine that trusts them becomes reachable from an
                AI agent. This server, other projects and cloud metadata stay
                blocked, and LAN hosts still cannot connect in. Turning it off
                later cuts open connections immediately.
              </p>
            </>
          }
        />
      )}
    </div>
  );
}
