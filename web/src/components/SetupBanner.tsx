import { useState } from "react";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import { api, tryUI } from "../lib/api";
import { usePolled } from "../lib/hooks";
import { ConfirmDialog } from "./ui";

export function SetupBanner() {
  const navigate = useNavigate();
  const onSetupPage = useRouterState({
    select: (s) => s.location.pathname === "/setup",
  });
  const {
    data: st,
    refresh,
    error,
  } = usePolled(() => api.setupStatus(), 10000);
  const [confirming, setConfirming] = useState(false);

  // A failed probe hides the banner but must not pretend setup is done —
  // show a one-line note instead of nothing (audit finding). Once st is
  // known, transient poll errors don't flicker the banner.
  if (!st && error)
    return (
      <div className="border-b border-border bg-bg-raised px-3 py-1.5 text-[11px] text-text-dim">
        Setup status unavailable — retrying…
      </div>
    );
  if (!st || st.done) return null;

  const required: Array<[string, boolean]> = [
    ["git", st.gitIdentity],
    ["ssh", st.sshKey],
  ];
  const optional: Array<[string, boolean]> = [
    ["gh", st.github],
    ["gcloud", st.gcloud],
    ["claude", st.claude],
    ["agy", st.agy],
  ];

  const markDone = () =>
    tryUI(async () => {
      await api.setupDone();
      refresh();
      if (onSetupPage) navigate({ to: "/" });
    });

  return (
    <div className="mx-2 mb-1 rounded-md border border-warn/30 bg-warn/10 p-2.5 text-xs">
      <div className="mb-1.5 font-medium">First-time setup</div>
      <div className="mb-2 space-y-0.5 text-text-dim">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          {required.map(([label, ok]) => (
            <span key={label} className={ok ? "text-ok" : "text-warn"}>
              {ok ? "✓" : "○"} {label}
            </span>
          ))}
          <span className="text-[10px] uppercase tracking-wide opacity-60">
            required
          </span>
        </div>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          {optional.map(([label, ok]) => (
            <span key={label} className={ok ? "text-ok" : ""}>
              {ok ? "✓" : "○"} {label}
            </span>
          ))}
          <span className="text-[10px] uppercase tracking-wide opacity-60">
            optional
          </span>
        </div>
      </div>
      <div className="flex gap-1.5">
        <button
          onClick={() => navigate({ to: "/setup" })}
          className="flex-1 rounded bg-warn/20 px-2 py-1 font-medium text-warn hover:brightness-110"
        >
          Open setup
        </button>
        <button
          onClick={() => setConfirming(true)}
          className="rounded px-2 py-1 text-text-dim hover:text-text"
        >
          Done
        </button>
      </div>
      {confirming && (
        <ConfirmDialog
          title="Mark setup as done?"
          body="The hidden setup VM is removed; your sign-ins stay in the shared home."
          confirmLabel="Mark done"
          onConfirm={markDone}
          onClose={() => setConfirming(false)}
        />
      )}
    </div>
  );
}
