import { useEffect, useRef, useState } from "react";
import { CLOSE_EXITED, CLOSE_REPLACED, termSocketUrl } from "../lib/api";
import { useTheme } from "../lib/hooks";
import {
  createTerminal,
  terminalTheme,
  type StyrmanTerminal,
} from "../lib/terminal";

export type TermTarget =
  { kind: "term"; project: string; id: string } | { kind: "setup" };

type ConnState =
  | "connecting"
  | "connected"
  | "exited"
  | "replaced"
  | "reconnecting"
  | "disconnected";

// Only these close codes are transient per the protocol spec; everything
// else (failed upgrade, server error) must not loop forever.
const TRANSIENT_CODES = new Set([1000, 1001, 1006]);
const MAX_RETRIES = 8;

/**
 * One styrman terminal (lib/terminal.ts) bound to a WebSocket. Implements
 * the wire protocol: binary frames are PTY bytes, JSON text frames carry
 * resize, close 4000 = paused (no auto-reconnect), close 4001 = replaced by
 * another tab (NEVER reconnect), anything else = transient → reconnect.
 *
 * (The former URL "link bar" was removed in v1.3 — copy-on-select made it
 * redundant, and it mostly harvested junk URLs from apt output. The
 * extraction machinery survives in lib/terminal.ts if it's ever wanted.)
 */
export function TerminalPane({
  target,
  onExited,
}: {
  target: TermTarget;
  onExited?: () => void;
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
  useEffect(() => {
    onExitedRef.current = onExited;
  });

  const targetKey =
    (target.kind === "setup" ? "setup" : `${target.project}/${target.id}`) +
    `#${epoch}`;

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

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const styr = createTerminal(host, {
      // Captured at effect run (targetKey change); the theme effect above
      // keeps it current afterwards.
      dark: resolved === "dark",
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

    let ws: WebSocket | null = null;
    let closedForGood = false;
    let retries = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

    const connect = () => {
      if (closedForGood) return;
      setState((prev) =>
        prev === "connecting" ? "connecting" : "reconnecting",
      );
      ws = new WebSocket(termSocketUrl(target, term.cols, term.rows));
      ws.binaryType = "arraybuffer";
      ws.onopen = () => {
        retries = 0;
        setState("connected");
        // The URL size only seeds session *creation*; a reattach needs an
        // explicit resize in case the pane changed size while disconnected.
        fit.fit();
        ws?.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }),
        );
        flushOutbox(); // send anything typed while reconnecting
        term.focus();
      };
      ws.onmessage = (ev) => {
        term.write(new Uint8Array(ev.data as ArrayBuffer));
      };
      ws.onclose = (ev) => {
        if (closedForGood) return;
        if (ev.code === CLOSE_REPLACED) {
          setState("replaced");
          closedForGood = true; // protocol: never reconnect on 4001
          return;
        }
        if (ev.code === CLOSE_EXITED) {
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
        }
      };
    };
    connect();

    // N6: keystrokes typed during a brief reconnect window used to vanish
    // silently ("I typed and nothing happened"). Queue them (bounded) and
    // flush on reconnect.
    const outbox: string[] = [];
    const flushOutbox = () => {
      if (ws?.readyState !== WebSocket.OPEN) return;
      for (const d of outbox.splice(0)) {
        ws.send(new TextEncoder().encode(d).buffer as ArrayBuffer);
      }
    };
    const dataSub = term.onData((d) => {
      if (ws?.readyState === WebSocket.OPEN) {
        // Binary frame = raw input bytes (text frames are control messages).
        ws.send(new TextEncoder().encode(d).buffer as ArrayBuffer);
      } else if (!closedForGood && outbox.length < 256) {
        // Queue only while a reconnect is expected; bytes typed after a
        // terminal-for-good close (exited/replaced) could never flush.
        outbox.push(d);
      }
    });

    // Debounce: dragging the window edge fires this dozens of times/s, and
    // each resize is a full tmux pane resize + TUI redraw inside the microVM.
    let resizeTimer: ReturnType<typeof setTimeout> | undefined;
    const onResize = () => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        fit.fit();
        // FitAddon clamps to 2×1; don't ship a nonsense size for a collapsed
        // pane.
        if (term.cols < 10 || term.rows < 3) return;
        if (ws?.readyState === WebSocket.OPEN) {
          ws.send(
            JSON.stringify({
              type: "resize",
              cols: term.cols,
              rows: term.rows,
            }),
          );
        }
      }, 120);
    };
    const resizeObserver = new ResizeObserver(onResize);
    resizeObserver.observe(host);

    // The first fit() runs before the custom terminal font has loaded, so the
    // cell height is measured from a fallback font. When Google Sans Code loads
    // and the cell metrics change, the row count is stale — the last row clips
    // (the visible "wrong bottom margin"). Refit once fonts are ready and tell
    // the server if the row count changed.
    void document.fonts?.ready.then(() => {
      if (styrRef.current !== styr) return; // terminal was swapped/torn down
      fit.fit();
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }),
        );
      }
    });

    return () => {
      closedForGood = true;
      clearTimeout(reconnectTimer);
      clearTimeout(resizeTimer);
      clearTimeout(toastTimer.current); // no toast after unmount (N2)
      dataSub.dispose();
      resizeObserver.disconnect();
      if (ws) {
        ws.onclose = null;
        ws.close();
      }
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
              {state === "disconnected" && "Disconnected"}
            </span>
            {(state === "exited" || state === "disconnected") && (
              <button
                onClick={() => {
                  setState("connecting");
                  setEpoch((e) => e + 1);
                }}
                className="rounded-full border border-accent bg-accent-soft px-3 py-1 text-xs text-on-accent-soft hover:brightness-110"
              >
                {state === "exited" ? "Resume" : "Reconnect"}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
