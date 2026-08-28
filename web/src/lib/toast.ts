// Tiny toast store — replaces every native alert() (operator request: no
// browser popups). Framework-free so lib modules (api.ts) can emit without
// importing React; ToastHost subscribes via useSyncExternalStore.

export interface Toast {
  id: number;
  kind: "error" | "success" | "info";
  text: string;
}

let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();
const timers = new Map<number, ReturnType<typeof setTimeout>>();

const emit = () => listeners.forEach((l) => l());

export function toast(kind: Toast["kind"], text: string, ttlMs = 5000): void {
  // Identical repeats (polling errors would stack) refresh the existing
  // toast's TTL instead of duplicating — dropping them entirely would leave
  // a repeat arriving near expiry with no feedback at all.
  const dup = toasts.find((x) => x.text === text && x.kind === kind);
  if (dup) {
    clearTimeout(timers.get(dup.id));
    timers.set(
      dup.id,
      setTimeout(() => dismissToast(dup.id), ttlMs),
    );
    return;
  }
  const t = { id: nextId++, kind, text };
  toasts = [...toasts, t];
  emit();
  timers.set(
    t.id,
    setTimeout(() => dismissToast(t.id), ttlMs),
  );
}

export function dismissToast(id: number): void {
  clearTimeout(timers.get(id));
  timers.delete(id);
  if (!toasts.some((t) => t.id === id)) return;
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}

export function subscribeToasts(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function getToasts(): Toast[] {
  return toasts;
}
