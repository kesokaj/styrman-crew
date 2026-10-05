import type { ProjectResources } from "./api";

/** The Resources dialog's editable state (ADR-083). CPU and memory each have
 *  an "Unlimited" checkbox; the number is kept as typed so an invalid entry
 *  can be shown, not silently coerced. */
export interface ResourcesForm {
  cpusUnlimited: boolean;
  cpus: string;
  memoryUnlimited: boolean;
  memoryGiB: string;
  diskGB: string;
}

export interface ResourcesBody {
  cpus: number;
  memoryGiB: number;
  diskGB: number;
}

/** The form for a project's current values. An unlimited field's number box
 *  starts at a sensible value for when the box is unticked. */
export function formFrom(r: ProjectResources): ResourcesForm {
  return {
    cpusUnlimited: r.cpus === 0,
    cpus: String(r.cpus || Math.min(4, r.limits.maxCpus)),
    memoryUnlimited: r.memoryGiB === 0,
    memoryGiB: String(r.memoryGiB || Math.max(1, Math.min(8, r.limits.maxMemoryGiB))),
    diskGB: String(r.diskGB),
  };
}

function wholeNumber(s: string): number | undefined {
  const t = s.trim();
  return /^\d+$/.test(t) ? Number(t) : undefined;
}

/** Validates the form against the server's limits. Returns the PUT body, or
 *  the first error in the order the dialog shows the fields. The server
 *  checks the same bounds; this only spares a round trip. */
export function bodyFrom(
  f: ResourcesForm,
  limits: ProjectResources["limits"],
): { body: ResourcesBody } | { error: string } {
  let cpus = 0;
  if (!f.cpusUnlimited) {
    const n = wholeNumber(f.cpus);
    if (n === undefined || n < 1 || n > limits.maxCpus)
      return { error: `CPU must be a whole number from 1 to ${limits.maxCpus}` };
    cpus = n;
  }
  let memoryGiB = 0;
  if (!f.memoryUnlimited && limits.maxMemoryGiB > 0) {
    const n = wholeNumber(f.memoryGiB);
    if (n === undefined || n < 1 || n > limits.maxMemoryGiB)
      return { error: `Memory must be a whole number of GiB from 1 to ${limits.maxMemoryGiB}` };
    memoryGiB = n;
  }
  const disk = wholeNumber(f.diskGB);
  if (disk === undefined || disk < limits.minDiskGB || disk > limits.maxDiskGB)
    return {
      error: `Docker disk must be ${limits.minDiskGB}–${limits.maxDiskGB} GB (it can only grow)`,
    };
  return { body: { cpus, memoryGiB, diskGB: disk } };
}

/** Whether saving would change anything. Saving unchanged values still
 *  matters when the project is running at an older size (`pending`). */
export function unchanged(b: ResourcesBody, r: ProjectResources): boolean {
  return b.cpus === r.cpus && b.memoryGiB === r.memoryGiB && b.diskGB === r.diskGB;
}
