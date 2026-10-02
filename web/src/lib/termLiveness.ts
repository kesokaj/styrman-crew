// Dead-socket detection for one terminal WebSocket (ADR-079).
//
// After sleep or a network change a socket can be dead while the browser
// still reports it OPEN: the server's own ping closes ITS side, but that
// close never reaches a page whose connection is gone, and a browser gives
// script no ping of its own. The pane then looked frozen until a reload. So
// the page asks: a `{"type":"ping"}` text frame, which the bridge answers
// with `{"type":"pong"}`; a probe that gets no frame back in time means the
// socket is dead and the pane reconnects.
//
// One instance per socket. Everything here is timers and two callbacks, so
// it is unit-tested without a WebSocket (termLiveness.test.ts).

/** The control frame the bridge answers with a pong. */
export const PING_FRAME = '{"type":"ping"}';

/** True for the bridge's pong control frame, false for anything else. */
export function isPong(data: string): boolean {
  try {
    const msg: unknown = JSON.parse(data);
    return (
      typeof msg === "object" &&
      msg !== null &&
      !Array.isArray(msg) &&
      (msg as { type?: unknown }).type === "pong"
    );
  } catch {
    return false;
  }
}

export interface Liveness {
  /** The socket opened: send one ping, with no deadline. Its pong is how
   *  this connection shows it answers pings at all. */
  opened(): void;
  /** Any frame arrived: the socket is alive, a pending deadline is cleared. */
  received(): void;
  /** A pong arrived: this connection answers pings. */
  pong(): void;
  /** Ask whether the socket is still alive. Does nothing until this
   *  connection has answered a ping, or while an earlier probe is pending;
   *  otherwise sends a ping and calls onDead — once — if no frame arrives
   *  within the timeout. */
  probe(): void;
  /** The socket is gone or replaced: clear timers; nothing fires after this. */
  stop(): void;
}

export function createLiveness(o: {
  sendPing: () => void;
  onDead: () => void;
  /** How long a probe waits for any frame. Default 5000 ms. */
  timeoutMs?: number;
}): Liveness {
  const timeoutMs = o.timeoutMs ?? 5000;
  // Learned per connection: a server without the pong reply (a rollback
  // under an open page) never answers, and must never be declared dead —
  // every wake would otherwise become a reconnect.
  let answers = false;
  let deadline: ReturnType<typeof setTimeout> | undefined;
  let over = false; // stopped, or already declared dead

  const clear = () => {
    if (deadline !== undefined) {
      clearTimeout(deadline);
      deadline = undefined;
    }
  };

  return {
    opened() {
      if (over) return;
      o.sendPing();
    },
    received() {
      clear();
    },
    pong() {
      answers = true;
      clear();
    },
    probe() {
      if (over || !answers || deadline !== undefined) return;
      o.sendPing();
      deadline = setTimeout(() => {
        deadline = undefined;
        if (over) return;
        over = true;
        o.onDead();
      }, timeoutMs);
    },
    stop() {
      over = true;
      clear();
    },
  };
}
