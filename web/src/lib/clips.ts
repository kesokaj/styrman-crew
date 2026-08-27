// Clipboard history for text copied inside styrman (terminal copy-on-select,
// Ctrl+C, link-bar Copy). Browser security makes the OS clipboard unreadable
// in the background, so this records what styrman itself put there — the
// history a user wants back ("what was that URL from ten minutes ago?").
//
// SECURITY (R10, ADR-023): copy-on-select means selecting an OAuth code or
// token during setup would land it here. So this uses sessionStorage (cleared
// when the tab closes, never synced across tabs/devices) with a short TTL —
// NOT localStorage. It is a short-lived convenience, deliberately not durable.

export interface Clip {
  text: string;
  at: number; // epoch ms
}

const KEY = "styrman-clips";
const MAX = 30;
const TTL_MS = 60 * 60 * 1000; // 1 hour

const listeners = new Set<() => void>();
let loaded = false;
let clips: Clip[] = [];

function store(): Storage | null {
  try {
    return window.sessionStorage;
  } catch {
    return null; // private mode / blocked — fall back to in-memory
  }
}

function fresh(list: Clip[]): Clip[] {
  const cutoff = Date.now() - TTL_MS;
  return list.filter((c) => c.at >= cutoff).slice(0, MAX);
}

// Lazy first access (never touch storage at module import — a prerendered SPA
// has no window during the build).
function ensureLoaded() {
  if (loaded) return;
  loaded = true;
  const s = store();
  if (!s) return;
  try {
    const raw = s.getItem(KEY);
    if (raw) clips = fresh(JSON.parse(raw) as Clip[]);
  } catch {
    clips = [];
  }
}

function persist() {
  const s = store();
  if (!s) return;
  try {
    s.setItem(KEY, JSON.stringify(clips));
  } catch {
    /* storage full/blocked — history is a convenience, never an error */
  }
}

export function addClip(text: string) {
  ensureLoaded();
  const t = text.trim();
  if (!t) return;
  clips = fresh([{ text: t, at: Date.now() }, ...clips.filter((c) => c.text !== t)]);
  persist();
  listeners.forEach((l) => l());
}

export function getClips(): Clip[] {
  ensureLoaded();
  clips = fresh(clips);
  return clips;
}

export function clearClips() {
  clips = [];
  persist();
  listeners.forEach((l) => l());
}

export function subscribeClips(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}
