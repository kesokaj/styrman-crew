/**
 * A minimal Chrome DevTools Protocol client over one WebSocket (ADR-068).
 *
 * The Browser pane speaks CDP to the project's in-guest Chromium through the
 * server's relay (`/ws/browser/{project}`), which forwards text frames and
 * knows nothing of the protocol. This client does the three things a CDP
 * session needs and nothing more: correlate replies to requests by id, fan
 * events out to subscribers, and fail every pending call when the socket
 * closes. No dependency: the protocol is JSON-RPC-shaped and a few dozen
 * lines cover it.
 *
 * Flattened sessions: a call may carry a `sessionId` (from
 * `Target.attachToTarget {flatten: true}`); replies and events carry it back,
 * and subscribers receive it as their second argument.
 */

/** What the client needs from a socket — a real WebSocket satisfies it. */
export interface CdpTransport {
  send(data: string): void;
  close(): void;
  onmessage: ((ev: { data: unknown }) => void) | null;
  onclose: ((ev: { code: number; reason: string }) => void) | null;
}

export class CdpError extends Error {
  readonly code: number;
  constructor(message: string, code: number) {
    super(message);
    this.name = "CdpError";
    this.code = code;
  }
}

export type CdpEventHandler = (params: unknown, sessionId?: string) => void;

type Pending = { resolve: (v: unknown) => void; reject: (e: CdpError) => void };

/** Code the client uses for a call that was still pending when the socket closed. */
export const CDP_CLOSED = -32000;

export class CdpClient {
  private nextId = 1;
  private readonly pending = new Map<number, Pending>();
  private readonly handlers = new Map<string, Set<CdpEventHandler>>();
  private closedState = false;
  /** Called once, when the socket closes (from either side). */
  onClose: ((code: number, reason: string) => void) | null = null;

  private readonly ws: CdpTransport;

  constructor(ws: CdpTransport) {
    this.ws = ws;
    ws.onmessage = (ev) => this.handleMessage(ev.data);
    ws.onclose = (ev) => this.handleClose(ev.code, ev.reason);
  }

  get closed(): boolean {
    return this.closedState;
  }

  /** Send one command; resolves with its `result`, rejects with a CdpError. */
  send<T = unknown>(method: string, params?: object, sessionId?: string): Promise<T> {
    if (this.closedState) {
      return Promise.reject(new CdpError("socket closed", CDP_CLOSED));
    }
    const id = this.nextId++;
    const msg: Record<string, unknown> = { id, method, params: params ?? {} };
    if (sessionId) msg.sessionId = sessionId;
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject });
      try {
        this.ws.send(JSON.stringify(msg));
      } catch (e) {
        this.pending.delete(id);
        reject(new CdpError(e instanceof Error ? e.message : String(e), CDP_CLOSED));
      }
    });
  }

  /** Subscribe to an event; returns the unsubscribe function. */
  on(method: string, fn: CdpEventHandler): () => void {
    let set = this.handlers.get(method);
    if (!set) {
      set = new Set();
      this.handlers.set(method, set);
    }
    set.add(fn);
    return () => {
      set.delete(fn);
    };
  }

  /** Close the socket; every pending call rejects with CDP_CLOSED. */
  close(): void {
    if (this.closedState) return;
    try {
      this.ws.close();
    } catch {
      // A socket that is already gone is what we wanted anyway.
    }
    this.handleClose(1000, "closed by client");
  }

  private handleMessage(raw: unknown): void {
    if (typeof raw !== "string") return; // the relay only carries text
    let msg: {
      id?: unknown;
      result?: unknown;
      error?: { code?: unknown; message?: unknown };
      method?: unknown;
      params?: unknown;
      sessionId?: unknown;
    };
    try {
      msg = JSON.parse(raw);
    } catch {
      return; // a malformed frame is dropped, never fatal
    }
    if (typeof msg !== "object" || msg === null) return;
    if (typeof msg.id === "number") {
      const p = this.pending.get(msg.id);
      if (!p) return; // a reply we no longer wait for (e.g. after close)
      this.pending.delete(msg.id);
      if (msg.error) {
        const code = typeof msg.error.code === "number" ? msg.error.code : -1;
        const message =
          typeof msg.error.message === "string" ? msg.error.message : "protocol error";
        p.reject(new CdpError(message, code));
      } else {
        p.resolve(msg.result);
      }
      return;
    }
    if (typeof msg.method === "string") {
      const set = this.handlers.get(msg.method);
      if (!set) return;
      const sid = typeof msg.sessionId === "string" ? msg.sessionId : undefined;
      // A handler may unsubscribe itself mid-dispatch; Set iteration
      // tolerates deletion, so no copy is needed.
      for (const fn of set) fn(msg.params, sid);
    }
  }

  private handleClose(code: number, reason: string): void {
    if (this.closedState) return;
    this.closedState = true;
    for (const p of this.pending.values()) {
      p.reject(new CdpError(`socket closed (${code})`, CDP_CLOSED));
    }
    this.pending.clear();
    this.onClose?.(code, reason);
  }
}
