import { usePolled } from "../lib/hooks";
import { api } from "../lib/api";

function fmtBytes(n: number): string {
  if (!n || n < 0) return "—";
  const gb = n / 1024 ** 3;
  if (gb >= 1) return `${gb.toFixed(gb < 10 ? 1 : 0)}G`;
  return `${Math.round(n / 1024 ** 2)}M`;
}

function pct(used: number, total: number): string {
  if (!total) return "—";
  return `${Math.round((used / total) * 100)}%`;
}

function fmtUptime(s: number): string {
  if (!s || s < 0) return "—";
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d) return `${d}d ${h}h`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
}

function Stat({
  label,
  value,
  title,
}: {
  label: string;
  value: string;
  title?: string;
}) {
  return (
    <span className="flex shrink-0 items-center gap-1" title={title}>
      <span className="text-[9px] font-medium uppercase tracking-wider opacity-60">
        {label}
      </span>
      <span className="font-mono tabular-nums">{value}</span>
    </span>
  );
}

/**
 * A thin status bar under the terminal showing the project microVM's live
 * resource usage — CPU, memory, disk, processes, IP, uptime. Polls every few
 * seconds. Doubles as the terminal's hard bottom edge.
 */
export function TerminalStatusBar({ project }: { project: string }) {
  const { data, error } = usePolled(() => api.projectStats(project), 4000, [
    project,
  ]);
  // Broker credentials are user-wide and materialized into every project, so
  // the count is "what's synced into this VM". Polled slowly — it rarely moves.
  const { data: creds } = usePolled(() => api.credentials(), 30000);
  const credCount = creds?.items.length ?? 0;

  const base =
    "flex h-7 shrink-0 items-center gap-3 overflow-x-auto border-t border-border bg-accent-soft px-3 text-[11px] text-on-accent-soft [scrollbar-width:none]";

  if (!data && !error) {
    return <div className={base}>…</div>;
  }
  if (error || !data || data.state !== "running") {
    return (
      <div className={base}>
        <span className="opacity-70">
          {data?.state === "stopped"
            ? "VM stopped — no live stats"
            : "stats unavailable"}
        </span>
      </div>
    );
  }

  return (
    <div className={base}>
      <span className="flex shrink-0 items-center gap-1.5" title="running">
        <span className="h-1.5 w-1.5 rounded-full bg-ok" />
        <span className="font-mono">{data.host || "vm"}</span>
      </span>
      <span className="opacity-30">│</span>
      <Stat
        label="cpu"
        value={`${data.cpuPercent}%`}
        title={`${data.cpuPercent}% of ${data.cpuCount} vCPU · load ${data.load1.toFixed(2)}`}
      />
      <Stat
        label="mem"
        value={`${fmtBytes(data.memUsedBytes)}/${fmtBytes(data.memTotalBytes)}`}
        title={`memory ${pct(data.memUsedBytes, data.memTotalBytes)} used`}
      />
      <Stat
        label="disk"
        value={`${pct(data.diskUsedBytes, data.diskTotalBytes)}`}
        title={`${fmtBytes(data.diskUsedBytes)} of ${fmtBytes(data.diskTotalBytes)} used`}
      />
      <Stat
        label="proc"
        value={`${data.processes}`}
        title="running processes"
      />
      {data.ip && <Stat label="ip" value={data.ip} title="bridge IP address" />}
      <Stat
        label="up"
        value={fmtUptime(data.uptimeSeconds)}
        title="VM uptime"
      />
      {creds && (
        <Stat
          label="creds"
          value={credCount > 0 ? `${credCount} synced` : "none"}
          title={
            credCount > 0
              ? `${credCount} credentials synced from the broker: ${creds.items.map((i) => i.name).join(", ")}`
              : "no credentials synced yet — finish setup"
          }
        />
      )}
    </div>
  );
}
