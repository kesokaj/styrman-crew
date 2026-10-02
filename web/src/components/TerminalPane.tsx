import { useEffect, useRef, useState } from "react";
import {
  CLOSE_EXITED,
  CLOSE_REPLACED,
  CLOSE_REVOKED,
  CLOSE_STOPPED,
  termSocketUrl,
} from "../lib/api";
import { FONT_STACKS, useAppearance, useTheme } from "../lib/hooks";
import {
  createTerminal,
  terminalTheme,
  type StyrmanTerminal,
} from "../lib/terminal";
import { createOutbox } from "../lib/termOutbox";
import {
  createLiveness,
  isPong,
  PING_FRAME,
  type Liveness,
} from "../lib/termLiveness";

export type TermTarget =
  | { kind: "term"; project: string; id: string }
  | { kind: "setup"; tool?: string };

type ConnState =
  | "connecting"
  | "connected"
  | "exited"
  | "replaced"
  | "stopped" // project VM deliberately stopped (close 4002)
  | "revoked" // the user was removed while the tab was open (close 4003)
  | "reconnecting"
  | "disconnected";

// Only these close codes are transient per the protocol spec; everything
// else (failed upgrade, server error) must not loop forever.
const TRANSIENT_CODES = new Set([1000, 1001, 1006]);
const MAX_RETRIES = 8;
// How often a visible pane asks its socket whether it is still alive (ADR-079).
const PROBE_INTERVAL_MS = 25_000;

/**
 * One styrman terminal (lib/terminal.ts) bound to a WebSocket. Implements
 * the wire protocol: binary frames are PTY bytes, JSON text frames carry
 * control messages (resize out; the ping/pong liveness check, ADR-079),
 * close 4000 = paused (no auto-reconnect), close 4001 = replaced by another
 * tab (never reconnect BY ITSELF: the pane takes the terminal back when its
 * page is shown again or the user presses "Use here"), anything else =
 * transient → reconnect.
 *
 * A project terminal also notices a socket that died without a close event
 * (sleep, a network change): it probes when the tab is shown, when the
 * network comes back and every 25 s while visible, and reconnects in place
 * when the probe goes unanswered. A setup pane takes none of this.
 *
 * (The former URL "link bar" was removed in v1.3 — copy-on-select made it
 * redundant, and it mostly harvested junk URLs from apt output. The
 * extraction machinery survives in lib/terminal.ts if it's ever wanted.)
 */
export function TerminalPane({
  target,
  onExited,
  focusToken,
}: {
  target: TermTarget;
  onExited?: () => void;
  /**
   * Bumped by the parent to demand keyboard focus. The pane is hidden with
   * CSS rather than unmounted when the editor is showing, so nothing restores
   * focus on the way back — without this the first keystrokes after switching
   * Editor → Terminal go to <body> and vanish (review finding).
   */
  focusToken?: number;
}) {
  const hostRef = useRef<HTMLDivElement>(null);
  const styrRef = useRef<StyrmanTerminal | null>(null);
  const [state, setState] = useState<ConnState>("connecting");
  const [epoch, setEpoch] = useState(0);
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [toast, setToast] = useState<string>();
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  // N7: keep onExited fresh without re-running the connection effect.
  const onExitedRef = useRef(onExited);
  const onResizeRef = useRef<(() => void) | null>(null);
  // Set by the connection effect: takes a replaced terminal back in place
  // (the "Use here" button). Null for a setup pane.
  const reclaimRef = useRef<(() => void) | null>(null);
  const lastFocusToken = useRef(focusToken);
  useEffect(() => {
    if (focusToken === undefined || focusToken === lastFocusToken.current) return;
    lastFocusToken.current = focusToken;
    // Re-fit BEFORE focusing. While the pane is hidden (display:none) the
    // ResizeObserver fires with a ~0 host, and onResize calls fit.fit() before
    // its `cols < 10` guard — so the guard stops a nonsense size reaching tmux
    // but the LOCAL xterm has already collapsed (observed: a full-width
    // terminal wrapping at ~12 columns after a Terminal→Editor→Terminal
    // round-trip). The observer does fire again on unhide, but only after a
    // 120ms debounce, so the user sees the mangled layout first. Refit now.
    const timers = [0, 50, 150, 400].map((d) =>
      setTimeout(() => {
        onResizeRef.current?.();
        styrRef.current?.term.focus();
      }, d),
    );
    return () => timers.forEach(clearTimeout);
  }, [focusToken]);
  useEffect(() => {
    onExitedRef.current = onExited;
  });

  const targetKey =
    (target.kind === "setup"
      ? `setup/${target.tool ?? ""}` // tool in the key: switching tools reconnects
      : `${target.project}/${target.id}`) + `#${epoch}`;

  // N5: a reconnect (epoch bump) or a switch to another terminal starts fresh.
  useEffect(() => {
    setToast(undefined); // a stale "✓ Copied" pill must not survive a switch
  }, [targetKey]);

  // Live theme: swap the paired ANSI palette without recreating the
  // terminal (which would drop scrollback).
  const { resolved } = useTheme();
  useEffect(() => {
    const s = styrRef.current;
    if (s) s.term.options.theme = terminalTheme(resolved === "dark");
  }, [resolved]);

  // Live font, same idea as the theme above but with one extra step: a font
  // change alters the CELL size, so rows/cols change and tmux has to be told.
  // Assigning the options is enough to make xterm re-measure (its options
  // service only re-measures when the value actually CHANGES, which is why the
  // mount-time remeasure below has to toggle the value to something else and
  // back — here the new value genuinely differs, so one assignment does it).
  // Then reuse the existing refit-and-ship path rather than duplicating it:
  // onResizeRef does fit() + the resize frame, and its shipSize() already
  // skips the frame when the geometry did not actually change.
  const { termFontSize, termFontFamily } = useAppearance();
  useEffect(() => {
    const s = styrRef.current;
    if (!s) return; // pre-mount; createTerminal receives these via opts
    s.term.options.fontSize = termFontSize;
    s.term.options.fontFamily = FONT_STACKS[termFontFamily];
    // onResizeRef is the IMMEDIATE path — it deliberately skips the 120ms
    // debounce. Safe today because the only control that writes these lives on
    // /settings, a different route, so no terminal is mounted while a slider is
    // being dragged. If a font control ever lands somewhere a terminal is
    // visible, route this through the debounce instead: a range input fires
    // onChange per pixel of travel, and each one would be a real tmux pane
    // resize inside the microVM.
    onResizeRef.current?.();
  }, [termFontSize, termFontFamily]);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const styr = createTerminal(host, {
      // Captured at effect run (targetKey change); the theme and font effects
      // above keep them current afterwards.
      dark: resolved === "dark",
      fontSize: termFontSize,
      fontFamily: termFontFamily,
      onCopy: () => {
        setToast("Copied");
        clearTimeout(toastTimer.current);
        toastTimer.current = setTimeout(() => setToast(undefined), 1200);
      },
    });
    styrRef.current = styr;
    const { term, fit } = styr;
    fit.fit();

    // Ctrl+Shift+F opens search (browser find is useless inside a canvas).
    styr.addKeyHandler((ev) => {
      if (
        ev.type === "keydown" &&
        ev.ctrlKey &&
        ev.shiftKey &&
        ev.code === "KeyF"
      ) {
        ev.preventDefault();
        setSearchOpen(true);
        setTimeout(() => searchInputRef.current?.focus(), 0);
        return false;
      }
      return true;
    });

    // Everything below about liveness, wake events and reclaiming applies to
    // project terminals only: a setup pane must never reconnect once it has
    // been open (see everOpened).
    const isTerm = target.kind === "term";
    let ws: WebSocket | null = null;
    let live: Liveness | null = null;
    let closedForGood = false;
    // What may bring a closed-for-good pane back without its own button
    // (ADR-079). "wake": it ran out of retries or met a non-transient close,
    // and a wake event (tab shown again, network back) tries once more.
    // "reclaim": another tab took the terminal (4001); only this page being
    // shown again, or its "Use here" button, takes it back — being replaced
    // causes neither, so two pages cannot trade the terminal in a loop.
    // "no": exited, stopped, revoked, or a setup pane.
    let resumable: "no" | "wake" | "reclaim" = "no";
    let retries = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    // Setup panes must never auto-reconnect once they've been open: the bridge
    // only answers 4000 when dockerd registers the exec's exit inside a 1 s
    // poll, so a finished styrman-setup can surface as a TRANSIENT close — and
    // a reconnect's `tmux new-session -A` would then RECREATE the ended session
    // and restart the script from the top (the v1.10.0 Ctrl+C-restart bug).
    // Pre-open retries still happen (the VM may still be coming up); post-open,
    // any close is final and hands off to onExited (which is status-aware).
    let everOpened = false;

    // Boot intent: the FIRST connect of an effect run is a user action —
    // opening the terminal, switching to it, or pressing Resume/Start (epoch
    // bump) — and may boot a stopped project VM. Automatic reconnect retries
    // are not user intent and must never resurrect a deliberately-stopped VM
    // (the server answers them with close 4002 instead).
    let bootIntent = true;
    const connect = () => {
      if (closedForGood) return;
      reconnectTimer = undefined;
      setState((prev) =>
        prev === "connecting" ? "connecting" : "reconnecting",
      );
      const sock = new WebSocket(
        termSocketUrl(target, term.cols, term.rows, bootIntent),
      );
      ws = sock;
      bootIntent = false;
      sock.binaryType = "arraybuffer";
      // One liveness check per socket. A probe that gets no frame back means
      // the socket died without telling us: replace it now.
      const lv = isTerm
        ? createLiveness({
            sendPing: () => {
              if (sock.readyState === WebSocket.OPEN) sock.send(PING_FRAME);
            },
            onDead: () => {
              if (ws === sock && !closedForGood) reconnectNow();
            },
          })
        : null;
      live = lv;
      sock.onopen = () => {
        retries = 0;
        everOpened = true;
        setState("connected");
        // The URL size only seeds session *creation*; a reattach needs an
        // explicit resize in case the pane changed size while disconnected.
        fit.fit();
        sock.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }),
        );
        flushOutbox(); // send anything typed while reconnecting
        lv?.opened();
        term.focus();
      };
      sock.onmessage = (ev) => {
        lv?.received();
        if (typeof ev.data === "string") {
          // Text frames are control messages; the bridge sends one kind, the
          // pong. Never terminal output: PTY bytes are binary frames.
          if (isPong(ev.data)) lv?.pong();
          return;
        }
        term.write(new Uint8Array(ev.data as ArrayBuffer));
      };
      sock.onclose = (ev) => {
        lv?.stop();
        if (closedForGood) return;
        if (ev.code === CLOSE_REPLACED) {
          setState("replaced");
          // Protocol: never reconnect BY ITSELF on 4001. `resumable` is what
          // lets this page take the terminal back when it is shown again or
          // its user asks.
          closedForGood = true;
          resumable = isTerm ? "reclaim" : "no";
          return;
        }
        if (ev.code === CLOSE_STOPPED) {
          // The project VM was deliberately stopped (Stop button / idle
          // reaper). Never auto-reconnect — that would boot it back up. The
          // "Start" button (epoch bump → boot intent) resumes explicitly.
          setState("stopped");
          closedForGood = true;
          return;
        }
        if (ev.code === CLOSE_REVOKED) {
          // The server re-checked the allowlist and this user is gone.
          // Reconnecting would only be refused at the handshake.
          setState("revoked");
          closedForGood = true;
          return;
        }
        if (
          ev.code === CLOSE_EXITED ||
          (target.kind === "setup" && everOpened)
        ) {
          setState("exited");
          closedForGood = true;
          onExitedRef.current?.();
          return;
        }
        if (TRANSIENT_CODES.has(ev.code) && retries < MAX_RETRIES) {
          retries++;
          reconnectTimer = setTimeout(
            connect,
            Math.min(1000 * 2 ** retries, 15000),
          );
        } else {
          setState("disconnected");
          closedForGood = true;
          resumable = isTerm ? "wake" : "no";
        }
      };
    };
    // Let go of the current socket without waiting for its close event — a
    // dead socket may never deliver one.
    const dropSocket = () => {
      live?.stop();
      live = null;
      if (!ws) return;
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      try {
        ws.close();
      } catch {
        // already gone
      }
      ws = null;
    };
    // Reconnect IN PLACE: the same xterm (scrollback kept), the retry counter
    // reset, and never boot intent — a wake or a reclaim is not the user
    // asking to start a stopped project; the server answers that with 4002
    // and the pane shows Start.
    const reconnectNow = () => {
      clearTimeout(reconnectTimer);
      reconnectTimer = undefined;
      dropSocket();
      // Coming back from a pane that had given up: whatever was queued while
      // it was still reconnecting belongs to a session the user had left,
      // and must not be typed into the shell behind their back (security
      // review 2026-10-02). A reconnect from a state that had NOT given up (a
      // dead socket just noticed, a backoff cut short) keeps the queue — and
      // the queue itself refuses to be replayed once it is older than 30 s
      // (lib/termOutbox.ts), which is what covers a machine that slept inside
      // the retry window.
      if (closedForGood) outbox.clear();
      closedForGood = false;
      resumable = "no";
      retries = 0;
      connect();
    };
    connect();

    // Wake events (ADR-079). `shown` is true only for this page going from
    // hidden to visible — the one event, beside the button, that may take a
    // replaced terminal back.
    const onWake = (shown: boolean) => {
      if (!isTerm) return;
      if (closedForGood) {
        if (resumable === "wake" || (resumable === "reclaim" && shown)) {
          reconnectNow();
        }
        return;
      }
      if (ws?.readyState === WebSocket.OPEN) live?.probe();
      // Waiting out a backoff: the user is back, try now. (A socket still
      // CONNECTING is left alone — its open handler pings.)
      else if (reconnectTimer !== undefined) reconnectNow();
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") onWake(true);
    };
    const onOnline = () => onWake(false);
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("online", onOnline);
    // The interval only PROBES an open socket. It never reconnects a
    // disconnected pane (that would be the endless retry MAX_RETRIES exists
    // to prevent) and never reclaims.
    const probeTimer = isTerm
      ? setInterval(() => {
          if (
            document.visibilityState === "visible" &&
            ws?.readyState === WebSocket.OPEN
          ) {
            live?.probe();
          }
        }, PROBE_INTERVAL_MS)
      : undefined;
    reclaimRef.current = isTerm
      ? () => {
          if (closedForGood && resumable === "reclaim") reconnectNow();
        }
      : null;

    // N6: keystrokes typed during a brief reconnect window used to vanish
    // silently ("I typed and nothing happened"). Queue them (bounded) and
    // flush on reconnect.
    const outbox = createOutbox();
    const flushOutbox = () => {
      if (ws?.readyState !== WebSocket.OPEN) return;
      for (const d of outbox.drain()) {
        ws.send(new TextEncoder().encode(d).buffer as ArrayBuffer);
      }
    };
    const dataSub = term.onData((d) => {
      if (ws?.readyState === WebSocket.OPEN) {
        // Binary frame = raw input bytes (text frames are control messages).
        ws.send(new TextEncoder().encode(d).buffer as ArrayBuffer);
      } else if (!closedForGood) {
        // Queue only while a reconnect is under way. Bytes typed at a pane
        // that is closed for good are dropped: after exited/stopped they
        // could never flush, and a replaced pane that is later taken back
        // must not replay what was typed while it said it was elsewhere.
        outbox.push(d);
      }
    });

    let resizeTimer: ReturnType<typeof setTimeout> | undefined;
    // Only ship a size that actually CHANGED. The focusToken path refits four
    // times ([0, 50, 150, 400]) so a hidden pane's layout has settled before
    // the user sees it, and every tick used to send its own frame — up to four
    // identical tmux pane resizes per Editor→Terminal toggle, exactly the cost
    // the 120ms debounce exists to avoid (2026-09-01 review). The retries are
    // still needed; the duplicate frames were not.
    let sentCols = 0;
    let sentRows = 0;
    const shipSize = () => {
      fit.fit();
      // FitAddon clamps to 2×1; don't ship a nonsense size for a collapsed pane.
      if (term.cols < 10 || term.rows < 3) return;
      if (term.cols === sentCols && term.rows === sentRows) return;
      if (ws?.readyState !== WebSocket.OPEN) return;
      sentCols = term.cols;
      sentRows = term.rows;
      ws.send(
        JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }),
      );
    };
    // Debounce: dragging the window edge fires this dozens of times/s, and
    // each resize is a full tmux pane resize + TUI redraw inside the microVM.
    const onResize = () => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(shipSize, 120);
    };
    // Let the show-the-pane path refit immediately instead of waiting out the
    // debounce (see the focusToken effect).
    onResizeRef.current = () => {
      clearTimeout(resizeTimer);
      shipSize();
    };
    const resizeObserver = new ResizeObserver(onResize);
    resizeObserver.observe(host);

    // The first render measures the cell size with a FALLBACK font, because the
    // custom terminal font (Google Sans Code) may still be loading. xterm caches
    // that cell height, and fit() reuses it — so the row count stays wrong once
    // the real (taller) font lands, and the last row clips at some window sizes.
    // fit() alone does NOT re-measure; reassigning a font option forces xterm's
    // char-size service to re-measure, and THEN fit() computes the right rows.
    const remeasureAndFit = () => {
      if (styrRef.current !== styr) return; // terminal was swapped/torn down
      // Toggle the font option to a different value and back (same frame, no
      // flash): xterm's options service only re-measures the cell size when the
      // value actually CHANGES, so a self-assignment would be a no-op. Both
      // writes differ from the previous, so both fire — the last one restores
      // the real font and leaves xterm with a freshly measured cell height.
      const ff = term.options.fontFamily;
      term.options.fontFamily = "monospace";
      term.options.fontFamily = ff;
      fit.fit();
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }),
        );
      }
    };
    void document.fonts?.ready.then(remeasureAndFit);

    return () => {
      closedForGood = true;
      resumable = "no";
      reclaimRef.current = null;
      clearTimeout(reconnectTimer);
      clearTimeout(resizeTimer);
      clearInterval(probeTimer);
      clearTimeout(toastTimer.current); // no toast after unmount (N2)
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("online", onOnline);
      dataSub.dispose();
      resizeObserver.disconnect();
      dropSocket();
      styrRef.current = null;
      styr.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetKey]);

  const runSearch = (backwards: boolean) => {
    const s = styrRef.current;
    if (!s || !query) return;
    if (backwards) s.search.findPrevious(query);
    else s.search.findNext(query);
  };

  return (
    <div className="flex h-full w-full flex-col bg-term-bg">
      {searchOpen && (
        <div className="flex items-center gap-1.5 border-b border-border bg-bg-raised px-2 py-1">
          <input
            ref={searchInputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") runSearch(e.shiftKey);
              if (e.key === "Escape") {
                setSearchOpen(false);
                styrRef.current?.search.clearDecorations();
                styrRef.current?.term.focus();
              }
            }}
            placeholder="Search scrollback (Enter next, Shift+Enter prev, Esc close)"
            className="w-72 rounded border border-border bg-bg px-2 py-0.5 text-xs outline-none focus:border-accent"
          />
          <button
            onClick={() => {
              setSearchOpen(false);
              styrRef.current?.search.clearDecorations();
              styrRef.current?.term.focus();
            }}
            className="text-xs text-text-dim hover:text-text"
          >
            ✕
          </button>
        </div>
      )}
      <div className="relative min-h-0 flex-1">
        <div ref={hostRef} className="term-host h-full w-full p-1.5" />
        {toast && (
          <span
            role="status"
            aria-live="polite"
            className="absolute bottom-2 right-3 z-10 rounded-full bg-bg-raised/90 px-2.5 py-0.5 text-[11px] text-ok shadow"
          >
            ✓ {toast}
          </span>
        )}
        {state !== "connected" && (
          <div className="absolute inset-x-0 top-0 z-10 flex justify-center gap-2 pt-2">
            <span className="rounded-full border border-border bg-bg-raised px-3 py-1 text-xs text-text-dim">
              {state === "connecting" && "Connecting…"}
              {state === "reconnecting" && "Reconnecting…"}
              {state === "exited" &&
                "Paused — the shell exited; the terminal and its transcript are kept"}
              {state === "replaced" && "Opened in another tab"}
              {state === "stopped" &&
                "Project stopped — everything is kept; Start boots it right back"}
              {state === "revoked" && "Access revoked — sign in again if this is a mistake"}
              {state === "disconnected" && "Disconnected"}
            </span>
            {state === "replaced" && target.kind === "term" && (
              // In place, not an epoch bump: the xterm and its scrollback
              // stay, and there is no boot intent (see reconnectNow).
              <button
                onClick={() => reclaimRef.current?.()}
                className="rounded-full border border-accent bg-accent-soft px-3 py-1 text-xs text-on-accent-soft hover:brightness-110"
              >
                Use here
              </button>
            )}
            {(state === "exited" ||
              state === "stopped" ||
              state === "disconnected") && (
              <button
                onClick={() => {
                  setState("connecting");
                  setEpoch((e) => e + 1);
                }}
                className="rounded-full border border-accent bg-accent-soft px-3 py-1 text-xs text-on-accent-soft hover:brightness-110"
              >
                {state === "exited"
                  ? "Resume"
                  : state === "stopped"
                    ? "Start"
                    : "Reconnect"}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
