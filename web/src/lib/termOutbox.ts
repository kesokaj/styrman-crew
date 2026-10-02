// The terminal pane's queue of keys typed while its socket is down.
//
// Typing during a brief reconnect used to vanish silently ("I typed and
// nothing happened"), so the pane queues those keys and sends them when the
// socket is back. A queue is only safe to replay while it is FRESH: a laptop
// that sleeps inside the reconnect window wakes up hours later with the same
// queue, and replaying it then types a stale line into a live shell behind
// the user's back (security review 2026-10-02). So the queue carries the time
// of its oldest key, and one that has aged past the limit is dropped whole —
// never trimmed, because half a command line is worse than none.
//
// Pure, with the clock injected, so the ageing is unit-tested without a
// socket or a sleep (termOutbox.test.ts).

export interface Outbox {
  /** Queue one chunk of input. Ignored once the queue is full. */
  push(data: string): void;
  /** Take everything queued, oldest first, and empty the queue — or drop it
   *  all and return nothing when its oldest key is older than the limit. */
  drain(): string[];
  /** Empty the queue without sending. */
  clear(): void;
  readonly size: number;
}

/** How long a queued key stays worth sending. Longer than the pane's whole
 *  first few backoff steps (2 + 4 + 8 s), far shorter than any sleep. */
export const OUTBOX_MAX_AGE_MS = 30_000;
export const OUTBOX_MAX_ENTRIES = 256;

export function createOutbox(
  o: { maxEntries?: number; maxAgeMs?: number; now?: () => number } = {},
): Outbox {
  const maxEntries = o.maxEntries ?? OUTBOX_MAX_ENTRIES;
  const maxAgeMs = o.maxAgeMs ?? OUTBOX_MAX_AGE_MS;
  const now = o.now ?? Date.now;
  let queue: string[] = [];
  let oldestAt = 0; // when the first key of the current queue was typed

  return {
    push(data) {
      if (queue.length >= maxEntries) return;
      if (queue.length === 0) oldestAt = now();
      queue.push(data);
    },
    drain() {
      const out = queue;
      queue = [];
      if (out.length === 0) return out;
      return now() - oldestAt > maxAgeMs ? [] : out;
    },
    clear() {
      queue = [];
    },
    get size() {
      return queue.length;
    },
  };
}
