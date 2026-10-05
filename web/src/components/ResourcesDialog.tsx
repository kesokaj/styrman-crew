import { useEffect, useState } from "react";
import { Button, Dialog } from "./ui";
import { api, type ProjectResources } from "../lib/api";
import { bodyFrom, formFrom, unchanged, type ResourcesForm } from "../lib/resources";

const inputClass =
  "w-20 rounded-md border border-border bg-bg px-2 py-1 text-right font-mono outline-none focus:border-accent disabled:opacity-40";

/**
 * A project's size (ADR-083): CPU and memory, each unlimited (the whole
 * host) or a number, and the Docker disk, which can only grow. Saving a
 * change restarts a running project at the new size; a stopped one takes it
 * at its next start.
 */
export function ResourcesDialog({
  projectId,
  projectName,
  onClose,
  onSaved,
}: {
  projectId: string;
  projectName: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [saved, setSaved] = useState<ProjectResources>();
  const [form, setForm] = useState<ResourcesForm>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    let live = true;
    api
      .projectResources(projectId)
      .then((r) => {
        if (!live) return;
        setSaved(r);
        setForm(formFrom(r));
      })
      .catch((e) => live && setError((e as Error).message));
    return () => {
      live = false;
    };
  }, [projectId]);

  const set = (patch: Partial<ResourcesForm>) =>
    setForm((f) => (f ? { ...f, ...patch } : f));

  const checked = saved && form ? bodyFrom(form, saved.limits) : undefined;
  const noop = saved && checked && "body" in checked && unchanged(checked.body, saved) && !saved.pending;

  const save = async () => {
    if (!checked || !("body" in checked)) return;
    setBusy(true);
    setError(undefined);
    try {
      await api.setProjectResources(projectId, checked.body);
      onSaved();
      onClose();
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  };

  return (
    <Dialog title={`Resources — “${projectName}”`} onClose={onClose} width="w-[28rem]">
      {!saved || !form ? (
        <p className="mb-3 text-xs text-text-dim">{error ?? "Loading…"}</p>
      ) : (
        <div className="space-y-3 text-xs">
          {saved.pending && (
            <p className="rounded-md bg-warn-soft px-2 py-1.5 text-on-warn-soft">
              Running at its previous size; Save restarts it at this one.
            </p>
          )}
          <div>
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium">CPU</span>
              <label className="flex items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={form.cpusUnlimited}
                  onChange={(e) => set({ cpusUnlimited: e.target.checked })}
                />
                {saved.limits.capped ? `Unlimited (up to ${saved.limits.maxCpus})` : "Unlimited"}
              </label>
              <span className="flex items-center gap-1.5">
                <input
                  aria-label="CPUs"
                  inputMode="numeric"
                  className={inputClass}
                  disabled={form.cpusUnlimited}
                  value={form.cpusUnlimited ? String(saved.limits.maxCpus) : form.cpus}
                  onChange={(e) => set({ cpus: e.target.value })}
                />
                <span className="w-8 text-text-dim">of {saved.limits.maxCpus}</span>
              </span>
            </div>
            <p className="mt-1 text-text-dim">
              The terminal shows one more CPU than this (the VM's own); the
              workload is capped at this number.
            </p>
          </div>
          {saved.limits.maxMemoryGiB > 0 ? (
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium">Memory</span>
              <label className="flex items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={form.memoryUnlimited}
                  onChange={(e) => set({ memoryUnlimited: e.target.checked })}
                />
                {saved.limits.capped ? `Unlimited (up to ${saved.limits.maxMemoryGiB} GiB)` : "Unlimited"}
              </label>
              <span className="flex items-center gap-1.5">
                <input
                  aria-label="Memory in GiB"
                  inputMode="numeric"
                  className={inputClass}
                  disabled={form.memoryUnlimited}
                  value={form.memoryUnlimited ? String(saved.limits.maxMemoryGiB) : form.memoryGiB}
                  onChange={(e) => set({ memoryGiB: e.target.value })}
                />
                <span className="w-8 text-text-dim">GiB</span>
              </span>
            </div>
          ) : (
            <p className="text-text-dim">Memory: not settable on this host (too small).</p>
          )}
          <div>
            <div className="flex items-center justify-between gap-3">
              <span className="font-medium">Docker disk</span>
              <span className="flex items-center gap-1.5">
                <input
                  aria-label="Project Docker disk in GB"
                  inputMode="numeric"
                  className={inputClass}
                  value={form.diskGB}
                  onChange={(e) => set({ diskGB: e.target.value })}
                />
                <span className="w-8 text-text-dim">GB</span>
              </span>
            </div>
            <p className="mt-1 text-text-dim">
              Grow only, {saved.limits.minDiskGB}–{saved.limits.maxDiskGB} GB.
            </p>
          </div>
          <p className="text-text-dim">
            Saving restarts this project if it is running. Home and workspace
            are kept; anything installed outside them (except apt packages,
            which are replayed) is lost.
          </p>
          {checked && "error" in checked && <p className="text-danger">{checked.error}</p>}
          {error && <p className="text-danger">{error}</p>}
        </div>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="filled"
          onClick={save}
          disabled={busy || !checked || !("body" in checked) || !!noop}
        >
          {busy ? "Saving…" : "Save"}
        </Button>
      </div>
    </Dialog>
  );
}
