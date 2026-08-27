import { useEffect, useRef, useState } from "react";
import { CLOSE_EXITED, CLOSE_REPLACED, termSocketUrl } from "../lib/api";
import { addClip } from "../lib/clips";
import { createTerminal, type StyrmanTerminal } from "../lib/terminal";

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
 * The link bar above the terminal surfaces URLs found in the buffer
 * (wrapped lines reassembled) as Open/Copy buttons — auth flows print long
 * URLs that TUIs wrap and redraw, making them unclickable and unselectable
 * in any terminal; lifting them out of the buffer sidesteps all of that.
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
  const [urls, setUrls] = useState<string[]>([]);
  const [dismissed, setDismissed] = useState<string[]>([]);
  const [copied, setCopied] = useState<string>();
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [toast, setToast] = useState<string>();
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

  const targetKey =
    (target.kind === "setup" ? "setup" : `${target.project}/${target.id}`) +
    `#${epoch}`;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const styr = createTerminal(host, {
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

    // Debounced URL harvesting: scan after output settles, not per byte.
    let scanTimer: ReturnType<typeof setTimeout> | undefined;
    const scheduleScan = () => {
      clearTimeout(scanTimer);
      scanTimer = setTimeout(() => {
        // Latest URLs win; cap the bar at the 3 most recent.
        setUrls(styr.extractUrls().slice(-3).reverse());
      }, 600);
    };

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
        term.focus();
      };
      ws.onmessage = (ev) => {
        const bytes = new Uint8Array(ev.data as ArrayBuffer);
        styr.sniffChunk(bytes);
        term.write(bytes);
        scheduleScan();
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
          onExited?.();
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

    const dataSub = term.onData((d) => {
      if (ws?.readyState === WebSocket.OPEN) {
        // Binary frame = raw input bytes (text frames are control messages).
        ws.send(new TextEncoder().encode(d).buffer as ArrayBuffer);
      }
    });

    const onResize = () => {
      fit.fit();
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }),
        );
      }
    };
    const resizeObserver = new ResizeObserver(onResize);
    resizeObserver.observe(host);

    return () => {
      closedForGood = true;
      clearTimeout(reconnectTimer);
      clearTimeout(scanTimer);
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

  const copyUrl = (u: string) => {
    navigator.clipboard
      ?.writeText(u)
      .then(() => {
        addClip(u);
        setCopied(u);
        setTimeout(() => setCopied(undefined), 1500);
      })
      .catch(() => {});
  };

  const visibleUrls = urls.filter((u) => !dismissed.includes(u));
  const short = (u: string) =>
    u.length > 46 ? u.slice(0, 30) + "…" + u.slice(-12) : u;

  const runSearch = (backwards: boolean) => {
    const s = styrRef.current;
    if (!s || !query) return;
    if (backwards) s.search.findPrevious(query);
    else s.search.findNext(query);
  };

  return (
    <div className="flex h-full w-full flex-col bg-term-bg">
      {visibleUrls.length > 0 && state === "connected" && (
        <div className="flex flex-wrap items-center gap-1.5 border-b border-border bg-bg-raised px-2 py-1.5">
          <span className="text-[10px] uppercase tracking-wider text-text-dim">
            links
          </span>
          {visibleUrls.map((u) => (
            <span
              key={u}
              className="flex items-center gap-1 rounded-full border border-border bg-bg px-2 py-0.5 text-[11px]"
              title={u}
            >
              <span className="max-w-[40vw] truncate text-text-dim">
                {short(u)}
              </span>
              <a
                href={u}
                target="_blank"
                rel="noopener noreferrer"
                className="font-medium text-accent hover:brightness-110"
              >
                Open
              </a>
              <button
                onClick={() => copyUrl(u)}
                className="text-text-dim hover:text-text"
              >
                {copied === u ? "✓" : "Copy"}
              </button>
              <button
                onClick={() => setDismissed((d) => [...d, u])}
                className="text-text-dim hover:text-text"
                aria-label="Dismiss link"
              >
                ✕
              </button>
            </span>
          ))}
        </div>
      )}
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
          <span className="absolute bottom-2 right-3 z-10 rounded-full bg-bg-raised/90 px-2.5 py-0.5 text-[11px] text-ok shadow">
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
                className="rounded-full border border-accent bg-accent-soft px-3 py-1 text-xs text-accent hover:brightness-110"
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
