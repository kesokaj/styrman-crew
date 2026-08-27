// Clipboard history for text copied inside styrman (terminal copy-on-select,
// Ctrl+C, link-bar Copy). Browser security makes the OS clipboard unreadable
// in the background, so this records what styrman itself put there — which
// is exactly the history a user wants back ("what was that URL/token from
// ten minutes ago?"). Kept per-browser in localStorage; never sent anywhere.

export interface Clip {
  text: string;
  at: number; // epoch ms
}

const KEY = "styrman-clips";
const MAX = 30;

const listeners = new Set<() => void>();
let clips: Clip[] = load();

function load(): Clip[] {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as Clip[];
    return Array.isArray(parsed) ? parsed.slice(0, MAX) : [];
  } catch {
    return [];
  }
}

function persist() {
  try {
    localStorage.setItem(KEY, JSON.stringify(clips));
  } catch {
    /* storage full/blocked — history is a convenience, never an error */
  }
}

export function addClip(text: string) {
  const t = text.trim();
  if (!t) return;
  clips = [{ text: t, at: Date.now() }, ...clips.filter((c) => c.text !== t)];
  if (clips.length > MAX) clips = clips.slice(0, MAX);
  persist();
  listeners.forEach((l) => l());
}

export function getClips(): Clip[] {
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
