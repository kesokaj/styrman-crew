import { useCallback, useEffect, useRef, useState } from "react";
import { api, ApiError, browserSocketUrl, CLOSE_REVOKED } from "../lib/api";
import {
  isChord,
  keyParams,
  mouseParams,
  normalizeUrl,
  wheelParams,
  type FrameGeom,
  type InputMouseParams,
  type RectLike,
} from "../lib/browserInput";
import { CdpClient, type CdpTransport } from "../lib/cdp";
import { clearBrowserRequest, useEditorPane } from "../lib/editorPane";
import { Button, IconButton } from "./ui";
import { ChevronLeftIcon, ChevronRightIcon, PlusIcon, RefreshIcon, XIcon } from "./icons";

/**
 * The Browser view (ADR-068): the project's own headless Chromium, drawn from
 * a CDP screencast and driven with CDP input events, over the server's relay.
 * The agents (claude/agy) drive the SAME browser through their MCP server, so
 * a tab they open shows up in the strip here and the operator can watch or
 * take over. `localhost` in this browser is the project's localhost.
 *
 * Close codes (terminal-protocol.md): 4002 = project stopped (no auto-
 * reconnect; the terminal boots it), 4004 = the browser could not start (the
 * reason is the launcher's log tail; Retry re-runs the launch through the
 * REST start, whose reply carries the FULL tail), 4003 = access revoked
 * (no Reconnect: the handshake would be refused, as TerminalPane's revoked
 * state), anything else — 1009 "frame too large" included — = disconnected
 * (Reconnect). A FAILED HANDSHAKE (the server answered 503 while attaching
 * to the project bridge) is retried once after a second, as the API contract
 * says.
 *
 * Hidden (the terminal or the editor is shown) the pane keeps its relay so
 * the tab strip stays current, but the screencast is stopped and no viewport
 * is pushed: a hidden box is 0×0, and forcing that (or a floor) onto the
 * browser the agents share would shrink THEIR page (review, 2026-09-13).
 */

type Phase = "connecting" | "ready" | "stopped" | "unavailable" | "disconnected" | "revoked";

type Tab = { targetId: string; url: string; title: string };

type TargetInfo = { targetId: string; type: string; url: string; title: string; attached?: boolean };

/** Notice display time — long enough to read a dialog's message. */
const NOTICE_MS = 5000;
/** Resize → device metrics debounce. */
const RESIZE_MS = 150;

export function BrowserPane({
  project,
  term,
  visible,
  onShowTerminal,
}: {
  project: string;
  term: string;
  /** Whether this pane is the one on screen (the route hides, never unmounts). */
  visible: boolean;
  /** Switch the pane back to the terminal view. */
  onShowTerminal: () => void;
}) {
  const { state } = useEditorPane(project, term);
  const request = state.browser;
  const [phase, setPhase] = useState<Phase>("connecting");
  const [reason, setReason] = useState("");
  const [tabs, setTabs] = useState<Tab[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [urlInput, setUrlInput] = useState("");
  const [editingUrl, setEditingUrl] = useState(false);
  const [notice, setNotice] = useState("");
  const [retrying, setRetrying] = useState(false);
  // Bumped to reconnect; the connection effect keys on it.
  const [epoch, setEpoch] = useState(0);
  // Automatic handshake retries left in this chain: ONE (the API contract
  // for a 503 while the server attaches to the bridge). A successful open,
  // a user's Reconnect or a Retry refills it.
  const autoRetries = useRef(1);

  const boxRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const urlRef = useRef<HTMLInputElement>(null);
  const clientRef = useRef<CdpClient | null>(null);
  const sessionRef = useRef<string | null>(null);
  const activeRef = useRef<string | null>(null);
  // The session whose screencast is running, or null: the ONE guard both
  // writers (attach, the visible effect) go through, so neither can start a
  // second screencast on the same session.
  const screencastRef = useRef<string | null>(null);
  const visibleRef = useRef(visible);
  const frameRef = useRef<FrameGeom>({ deviceWidth: 1280, deviceHeight: 800 });
  // The canvas's rect, refreshed on resize and pointer entry — not read
  // from layout on every mouse move.
  const rectRef = useRef<RectLike>({ left: 0, top: 0, width: 0, height: 0 });
  const tabsRef = useRef<Tab[]>([]);
  const noticeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  // Attaches are serialized: two rapid tab clicks used to interleave their
  // detach/attach steps and leave an orphan screencast running.
  const attachChain = useRef<Promise<void>>(Promise.resolve());

  const say = useCallback((text: string) => {
    setNotice(text);
    clearTimeout(noticeTimer.current);
    noticeTimer.current = setTimeout(() => setNotice(""), NOTICE_MS);
  }, []);

  const setTabList = useCallback((next: Tab[]) => {
    tabsRef.current = next;
    setTabs(next);
  }, []);

  const refreshRect = useCallback(() => {
    const canvas = canvasRef.current;
    if (canvas) rectRef.current = canvas.getBoundingClientRect();
  }, []);

  /**
   * The active page's viewport follows the canvas box, 1:1 CSS pixels —
   * only while the box HAS a size. Hidden, it is 0×0, and the old floor of
   * 320×200 was pushed onto the agents' page every time the operator
   * switched to the terminal.
   */
  const applyViewport = useCallback(async () => {
    const c = clientRef.current;
    const sid = sessionRef.current;
    const box = boxRef.current;
    if (!c || !sid || !box) return;
    const width = Math.round(box.clientWidth);
    const height = Math.round(box.clientHeight);
    if (width === 0 || height === 0) return;
    refreshRect();
    try {
      await c.send(
        "Emulation.setDeviceMetricsOverride",
        { width, height, deviceScaleFactor: 1, mobile: false },
        sid,
      );
    } catch {
      // The session may have gone away between the resize and this call.
    }
  }, [refreshRect]);

  /** Start or stop the active session's screencast; idempotent per session. */
  const setScreencast = useCallback(async (on: boolean) => {
    const c = clientRef.current;
    const sid = sessionRef.current;
    if (!c || !sid) return;
    if (on) {
      if (screencastRef.current === sid) return;
      screencastRef.current = sid;
      try {
        await c.send(
          "Page.startScreencast",
          { format: "jpeg", quality: 70, maxWidth: 4096, maxHeight: 4096, everyNthFrame: 1 },
          sid,
        );
      } catch {
        if (screencastRef.current === sid) screencastRef.current = null;
      }
    } else {
      if (screencastRef.current !== sid) return;
      screencastRef.current = null;
      try {
        await c.send("Page.stopScreencast", {}, sid);
      } catch {
        // Gone already; nothing to stop.
      }
    }
  }, []);

  /**
   * Attach to one page target, detaching the previous, and screencast it
   * when the pane is on screen. Calls queue behind each other; the tab is
   * marked active before the first await so a concurrent target-destroyed
   * sees it, and unmarked again if the attach fails.
   */
  const attach = useCallback(
    (targetId: string) => {
      const run = async () => {
        const c = clientRef.current;
        if (!c) return;
        activeRef.current = targetId;
        setActive(targetId);
        const prev = sessionRef.current;
        if (prev) {
          sessionRef.current = null;
          screencastRef.current = null;
          try {
            await c.send("Page.stopScreencast", {}, prev);
            await c.send("Target.detachFromTarget", { sessionId: prev });
          } catch {
            // Already gone — that is the case we are handling.
          }
        }
        try {
          const { sessionId } = await c.send<{ sessionId: string }>("Target.attachToTarget", {
            targetId,
            flatten: true,
          });
          sessionRef.current = sessionId;
          try {
            await c.send("Target.activateTarget", { targetId });
          } catch {
            // Headless has no window to raise; harmless.
          }
          await c.send("Page.enable", {}, sessionId);
          await applyViewport();
          if (visibleRef.current) await setScreencast(true);
        } catch (e) {
          if (activeRef.current === targetId) {
            activeRef.current = null;
            setActive(null);
          }
          // A session created before the failure (Page.enable or the
          // metrics threw) is half-attached: drop it, or the toolbar would
          // keep sending to a session whose Page domain never came up.
          const half = sessionRef.current;
          if (half) {
            sessionRef.current = null;
            screencastRef.current = null;
            c.send("Target.detachFromTarget", { sessionId: half }).catch(() => {});
          }
          throw e;
        }
      };
      const p = attachChain.current.then(run, run);
      attachChain.current = p.catch(() => {});
      return p;
    },
    [applyViewport, setScreencast],
  );

  const reconnect = useCallback(() => {
    autoRetries.current = 1;
    setEpoch((e) => e + 1);
  }, []);

  // ── Connection ─────────────────────────────────────────────────────────────
  useEffect(() => {
    let opened = false;
    let disposed = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    setPhase("connecting");
    setReason("");
    const ws = new WebSocket(browserSocketUrl(project));
    // A thin adapter: the client wants a transport with the two handlers it
    // sets, and a WebSocket's own handler types are wider than that.
    const transport: CdpTransport = {
      send: (d) => ws.send(d),
      close: () => ws.close(),
      onmessage: null,
      onclose: null,
    };
    ws.onmessage = (ev) => transport.onmessage?.({ data: ev.data });
    ws.onclose = (ev) => transport.onclose?.({ code: ev.code, reason: ev.reason });
    const client = new CdpClient(transport);
    clientRef.current = client;
    sessionRef.current = null;
    activeRef.current = null;
    screencastRef.current = null;

    // Frames: decode one at a time, keep only the newest while decoding —
    // latency over completeness. The ack goes back at once either way, so
    // Chromium keeps producing. The frame counter lives on the canvas
    // element (the e2e reads it): a React state per frame re-rendered the
    // whole pane up to 60 times a second.
    let decoding = false;
    let frames = 0;
    let pending: { data: string; sid: string; geom: FrameGeom } | null = null;
    const drain = () => {
      decoding = false;
      if (pending) {
        const next = pending;
        pending = null;
        draw(next);
      }
    };
    const draw = (f: { data: string; sid: string; geom: FrameGeom }) => {
      decoding = true;
      const img = new Image();
      img.onload = () => {
        const canvas = canvasRef.current;
        if (canvas && !disposed && sessionRef.current === f.sid) {
          if (canvas.width !== img.width || canvas.height !== img.height) {
            canvas.width = img.width;
            canvas.height = img.height;
          }
          canvas.getContext("2d")?.drawImage(img, 0, 0);
          frameRef.current = f.geom;
          canvas.dataset.frames = String(++frames);
        }
        drain();
      };
      img.onerror = drain;
      img.src = "data:image/jpeg;base64," + f.data;
    };

    client.on("Page.screencastFrame", (params, sid) => {
      // Shapes are checked, not cast: a frame the guest's browser sends is
      // guest-controlled input (security review, round 3).
      const p = params as { data?: unknown; sessionId?: unknown; metadata?: { deviceWidth?: unknown; deviceHeight?: unknown } } | null;
      if (!p || typeof p !== "object") return;
      if (sid && typeof p.sessionId === "number") {
        client.send("Page.screencastFrameAck", { sessionId: p.sessionId }, sid).catch(() => {});
      }
      if (!sid || sid !== sessionRef.current) return;
      const w = p.metadata?.deviceWidth;
      const hgt = p.metadata?.deviceHeight;
      if (typeof p.data !== "string" || typeof w !== "number" || typeof hgt !== "number") return;
      const f = { data: p.data, sid, geom: { deviceWidth: w, deviceHeight: hgt } };
      if (decoding) pending = f;
      else draw(f);
    });

    client.on("Page.javascriptDialogOpening", (params, sid) => {
      const p = params as { type?: unknown; message?: unknown } | null;
      if (sid) client.send("Page.handleJavaScriptDialog", { accept: true, promptText: "" }, sid).catch(() => {});
      // Rendered as text by React, never as HTML; cut so a page cannot turn
      // the notice line into a wall.
      const type = typeof p?.type === "string" ? p.type : "dialog";
      const msg = typeof p?.message === "string" ? p.message.slice(0, 200) : "";
      say(`Page ${type}: ${msg}`);
    });

    const upsert = (t: TargetInfo) => {
      if (!t || typeof t !== "object" || t.type !== "page" || typeof t.targetId !== "string") return;
      const cur = tabsRef.current;
      const i = cur.findIndex((x) => x.targetId === t.targetId);
      const tab = { targetId: t.targetId, url: t.url, title: t.title };
      setTabList(i < 0 ? [...cur, tab] : cur.map((x, j) => (j === i ? tab : x)));
    };
    // Chromium's Target.targetInfoChanged carries the URL as an interim
    // title and the document's real title only sometimes; a fresh
    // Target.getTargets after each load is what the /json/list endpoint
    // reads too, so re-ask on load and on navigation.
    const refreshTargets = () => {
      client
        .send<{ targetInfos: TargetInfo[] }>("Target.getTargets")
        .then((r) => {
          for (const t of r.targetInfos) upsert(t);
        })
        .catch(() => {});
    };
    client.on("Page.loadEventFired", refreshTargets);
    client.on("Page.frameNavigated", (params) => {
      const f = (params as { frame: { parentId?: string } }).frame;
      if (!f.parentId) refreshTargets();
    });

    client.on("Target.targetCreated", (params) => upsert((params as { targetInfo: TargetInfo }).targetInfo));
    client.on("Target.targetInfoChanged", (params) =>
      upsert((params as { targetInfo: TargetInfo }).targetInfo),
    );
    client.on("Target.targetDestroyed", (params) => {
      const id = (params as { targetId: string }).targetId;
      const rest = tabsRef.current.filter((x) => x.targetId !== id);
      setTabList(rest);
      if (activeRef.current === id) {
        sessionRef.current = null;
        screencastRef.current = null;
        activeRef.current = null;
        setActive(null);
        const next = rest[0];
        if (next) attach(next.targetId).catch(() => {});
        else
          client
            .send<{ targetId: string }>("Target.createTarget", { url: "about:blank" })
            .then((r) => attach(r.targetId))
            .catch(() => {});
      }
    });

    client.onClose = (code, closeReason) => {
      if (disposed) return;
      clientRef.current = null;
      sessionRef.current = null;
      screencastRef.current = null;
      if (code === 4002) {
        setPhase("stopped");
      } else if (code === CLOSE_REVOKED) {
        setPhase("revoked");
      } else if (code === 4004) {
        setReason(closeReason || "the browser could not be started");
        setPhase("unavailable");
      } else if (!opened && autoRetries.current > 0) {
        // Handshake failed (503 while the server attached to the bridge, or
        // a blip): one automatic retry after a second, then it is a
        // "disconnected" the user resolves.
        autoRetries.current -= 1;
        retryTimer = setTimeout(() => {
          if (!disposed) setEpoch((e) => e + 1);
        }, 1000);
        setPhase("connecting");
      } else {
        setReason(closeReason);
        setPhase("disconnected");
      }
    };

    ws.onopen = () => {
      opened = true;
      // A successful open refills the one automatic retry for the NEXT
      // drop; it used to stay spent after the first handshake failure.
      autoRetries.current = 1;
      (async () => {
        await client.send("Target.setDiscoverTargets", { discover: true });
        const { targetInfos } = await client.send<{ targetInfos: TargetInfo[] }>("Target.getTargets");
        const pages = targetInfos.filter((t) => t.type === "page");
        setTabList(pages.map((t) => ({ targetId: t.targetId, url: t.url, title: t.title })));
        let first = pages[0]?.targetId;
        if (!first) {
          first = (await client.send<{ targetId: string }>("Target.createTarget", { url: "about:blank" }))
            .targetId;
        }
        await attach(first);
        if (!disposed) setPhase("ready");
      })().catch((e: Error) => {
        // A socket the server accepted and closed at once (4002 "project
        // stopped") fires open, then close: the close handler has already
        // chosen the phase, and this rejection — a microtask later — must
        // not overwrite it with "disconnected".
        if (disposed || client.closed) return;
        // Close FIRST: closing runs onClose, which sets its own reason
        // ("closed by client"); ours must land after it, not under it.
        client.close();
        setReason(e.message);
        setPhase("disconnected");
      });
    };

    return () => {
      disposed = true;
      clearTimeout(retryTimer);
      client.close();
      clientRef.current = null;
    };
    // `attach`/`say`/`setTabList` are stable callbacks; `epoch` is the
    // reconnect trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project, epoch]);

  // ── Hidden panes stream nothing ────────────────────────────────────────────
  useEffect(() => {
    visibleRef.current = visible;
    if (!sessionRef.current) return;
    if (visible) void applyViewport().then(() => setScreencast(true));
    else void setScreencast(false);
  }, [visible, applyViewport, setScreencast]);

  // ── Viewport follows the box ───────────────────────────────────────────────
  useEffect(() => {
    const box = boxRef.current;
    if (!box) return;
    let t: ReturnType<typeof setTimeout> | undefined;
    const ro = new ResizeObserver(() => {
      refreshRect();
      clearTimeout(t);
      t = setTimeout(() => void applyViewport(), RESIZE_MS);
    });
    ro.observe(box);
    return () => {
      clearTimeout(t);
      ro.disconnect();
    };
  }, [applyViewport, refreshRect]);

  // ── The URL bar reflects the active tab unless the user is typing ─────────
  const activeTab = tabs.find((t) => t.targetId === active) ?? null;
  useEffect(() => {
    if (!editingUrl) setUrlInput(activeTab?.url === "about:blank" ? "" : (activeTab?.url ?? ""));
  }, [activeTab?.url, editingUrl]);

  const navigate = useCallback(
    (raw: string) => {
      const url = normalizeUrl(raw);
      const c = clientRef.current;
      const sid = sessionRef.current;
      if (!url) {
        if (raw.trim() !== "") say("Only http, https and about:blank can be typed here.");
        return;
      }
      if (!c || !sid) return;
      setEditingUrl(false);
      c.send("Page.navigate", { url }, sid).catch((e: Error) => say(e.message));
    },
    [say],
  );

  // ── "Open in Browser" from the Ports panel ────────────────────────────────
  useEffect(() => {
    if (!request || phase !== "ready" || !sessionRef.current) return;
    navigate(request.url);
    clearBrowserRequest(project, term);
  }, [request, phase, navigate, project, term]);

  /**
   * Retry after a failed launch: the REST start first, whose reply carries
   * the launcher's full log tail (a close frame holds 123 bytes of it), then
   * the relay. 409 = the project stopped meanwhile; 503 = a fresh bridge
   * attach, which the socket path retries once on its own; anything else
   * stays here with the message.
   */
  const retry = useCallback(async () => {
    setRetrying(true);
    try {
      await api.browserStart(project);
      reconnect();
    } catch (e) {
      const status = e instanceof ApiError ? e.status : 0;
      if (status === 409) setPhase("stopped");
      else if (status === 503) reconnect();
      else setReason((e as Error).message);
    } finally {
      setRetrying(false);
    }
  }, [project, reconnect]);

  const history = useCallback(
    async (delta: -1 | 1) => {
      const c = clientRef.current;
      const sid = sessionRef.current;
      if (!c || !sid) return;
      try {
        const h = await c.send<{ currentIndex: number; entries: Array<{ id: number }> }>(
          "Page.getNavigationHistory",
          {},
          sid,
        );
        const entry = h.entries[h.currentIndex + delta];
        if (entry) await c.send("Page.navigateToHistoryEntry", { entryId: entry.id }, sid);
      } catch (e) {
        say((e as Error).message);
      }
    },
    [say],
  );

  const reload = useCallback(() => {
    const c = clientRef.current;
    const sid = sessionRef.current;
    if (c && sid) c.send("Page.reload", {}, sid).catch((e: Error) => say(e.message));
  }, [say]);

  const newTab = useCallback(() => {
    const c = clientRef.current;
    if (!c) return;
    c.send<{ targetId: string }>("Target.createTarget", { url: "about:blank" })
      .then((r) => attach(r.targetId))
      .then(() => {
        setEditingUrl(true);
        setUrlInput("");
        urlRef.current?.focus();
      })
      .catch((e: Error) => say(e.message));
  }, [attach, say]);

  const closeTab = useCallback(
    (targetId: string) => {
      clientRef.current?.send("Target.closeTarget", { targetId }).catch((e: Error) => say(e.message));
    },
    [say],
  );

  // ── Input forwarding ──────────────────────────────────────────────────────
  const sendInput = (method: string, params: object) => {
    const c = clientRef.current;
    const sid = sessionRef.current;
    if (c && sid) c.send(method, params, sid).catch(() => {});
  };
  // Mouse moves are coalesced to one per animation frame (the newest wins)
  // and flushed before any press or release so the guest sees them in order.
  const pendingMove = useRef<InputMouseParams | null>(null);
  const moveRaf = useRef(0);
  const flushMove = () => {
    if (moveRaf.current) {
      cancelAnimationFrame(moveRaf.current);
      moveRaf.current = 0;
    }
    const m = pendingMove.current;
    if (m) {
      pendingMove.current = null;
      sendInput("Input.dispatchMouseEvent", m);
    }
  };
  useEffect(() => () => cancelAnimationFrame(moveRaf.current), []);
  const onMouse = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const params = mouseParams(
      {
        type: e.type as "mousedown" | "mouseup" | "mousemove",
        button: e.button,
        buttons: e.buttons,
        clientX: e.clientX,
        clientY: e.clientY,
        detail: e.detail,
        altKey: e.altKey,
        ctrlKey: e.ctrlKey,
        metaKey: e.metaKey,
        shiftKey: e.shiftKey,
      },
      rectRef.current,
      frameRef.current,
    );
    if (e.type === "mousemove") {
      pendingMove.current = params;
      if (!moveRaf.current) {
        moveRaf.current = requestAnimationFrame(() => {
          moveRaf.current = 0;
          flushMove();
        });
      }
      return;
    }
    if (e.type === "mousedown") canvasRef.current?.focus();
    flushMove();
    sendInput("Input.dispatchMouseEvent", params);
  };
  const onWheel = (e: React.WheelEvent<HTMLCanvasElement>) => {
    flushMove();
    sendInput("Input.dispatchMouseEvent", wheelParams(e, rectRef.current, frameRef.current));
  };
  const onKey = (e: React.KeyboardEvent<HTMLCanvasElement>) => {
    const chord = isChord(e);
    // The pane's own shortcuts, never forwarded: focus the URL bar, reload.
    if (e.type === "keydown" && chord && (e.key === "l" || e.key === "L")) {
      e.preventDefault();
      setEditingUrl(true);
      urlRef.current?.focus();
      urlRef.current?.select();
      return;
    }
    if (e.type === "keydown" && chord && (e.key === "r" || e.key === "R")) {
      e.preventDefault();
      reload();
      return;
    }
    e.preventDefault();
    sendInput(
      "Input.dispatchKeyEvent",
      keyParams({
        type: e.type as "keydown" | "keyup",
        key: e.key,
        code: e.code,
        keyCode: e.keyCode,
        location: e.location,
        altKey: e.altKey,
        ctrlKey: e.ctrlKey,
        metaKey: e.metaKey,
        shiftKey: e.shiftKey,
      }),
    );
  };

  const busy = phase !== "ready";

  return (
    <div className="flex h-full min-h-0 flex-col" data-phase={phase}>
      {/* Toolbar: history, URL bar, reload, new tab, back to the terminal. */}
      <div className="flex shrink-0 items-center gap-1 border-b border-border bg-bg-raised px-2 py-1 text-xs">
        <IconButton label="Page back" onClick={() => void history(-1)} disabled={busy}>
          <ChevronLeftIcon className="h-3.5 w-3.5" />
        </IconButton>
        <IconButton label="Page forward" onClick={() => void history(1)} disabled={busy}>
          <ChevronRightIcon className="h-3.5 w-3.5" />
        </IconButton>
        <IconButton label="Reload page" onClick={reload} disabled={busy}>
          <RefreshIcon />
        </IconButton>
        <input
          ref={urlRef}
          type="text"
          aria-label="Address"
          placeholder="localhost:3000 — the project's own localhost"
          className="min-w-0 flex-1 rounded border border-border bg-bg px-2 py-0.5 font-mono text-[11px] text-text outline-none focus:border-accent"
          value={urlInput}
          disabled={busy}
          onFocus={() => setEditingUrl(true)}
          onBlur={() => setEditingUrl(false)}
          onChange={(e) => setUrlInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              navigate(urlInput);
              canvasRef.current?.focus();
            } else if (e.key === "Escape") {
              setEditingUrl(false);
              canvasRef.current?.focus();
            }
          }}
        />
        <IconButton label="New tab" onClick={newTab} disabled={busy}>
          <PlusIcon />
        </IconButton>
        <IconButton label="Close browser view and show the terminal" onClick={onShowTerminal}>
          <XIcon className="h-3.5 w-3.5" />
        </IconButton>
      </div>
      {/* Tab strip: every page in the browser, the agent's included. */}
      {tabs.length > 0 && (
        <div className="flex shrink-0 gap-1 overflow-x-auto border-b border-border bg-bg-raised px-2 py-1" role="tablist">
          {tabs.map((t) => (
            <span
              key={t.targetId}
              role="tab"
              aria-selected={t.targetId === active}
              className={`inline-flex max-w-48 shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-[11px] ${
                t.targetId === active ? "bg-accent-soft text-on-accent-soft" : "text-text-dim hover:text-text"
              }`}
            >
              <button
                className="min-w-0 truncate"
                title={t.url}
                onClick={() => attach(t.targetId).catch((e: Error) => say(e.message))}
              >
                {t.title || t.url || "New tab"}
              </button>
              {tabs.length > 1 && (
                <button aria-label={`Close tab ${t.title || t.url}`} onClick={() => closeTab(t.targetId)}>
                  <XIcon className="h-3 w-3" />
                </button>
              )}
            </span>
          ))}
        </div>
      )}
      {notice && (
        <div className="shrink-0 border-b border-border bg-bg px-3 py-1 text-[11px] text-text-dim" role="status">
          {notice}
        </div>
      )}
      <div ref={boxRef} className="relative min-h-0 flex-1 bg-black">
        {/* data-frames starts at 0 in the markup and is then counted on the
            element itself (see the connection effect); the e2e gate reads it. */}
        <canvas
          ref={canvasRef}
          tabIndex={0}
          aria-label="Page"
          data-frames="0"
          className={`h-full w-full outline-none ${busy ? "hidden" : ""}`}
          // Pointer capture keeps a drag that leaves the canvas ours, so its
          // release still reaches the guest — no stuck button. Mouse events
          // follow the capture; it lifts itself on pointerup.
          onPointerDown={(e) => e.currentTarget.setPointerCapture(e.pointerId)}
          onPointerEnter={refreshRect}
          onMouseDown={onMouse}
          onMouseUp={onMouse}
          onMouseMove={onMouse}
          onWheel={onWheel}
          onKeyDown={onKey}
          onKeyUp={onKey}
          onContextMenu={(e) => e.preventDefault()}
        />
        {busy && (
          <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-xs">
            {phase === "connecting" && <p className="text-text-dim">Starting the browser…</p>}
            {phase === "stopped" && (
              <>
                <p className="text-text-dim">Project is stopped.</p>
                <div className="flex gap-2">
                  <Button variant="tonal" onClick={onShowTerminal}>
                    Show terminal
                  </Button>
                  {/* Once the project runs again (the terminal boots it, or
                      the sidebar's Start), this is the way back in — a 4002
                      never auto-reconnects. */}
                  <Button variant="text" onClick={reconnect}>
                    Reconnect
                  </Button>
                </div>
              </>
            )}
            {phase === "unavailable" && (
              <>
                <p className="text-danger">Browser unavailable.</p>
                <pre className="max-w-full overflow-x-auto whitespace-pre-wrap text-left font-mono text-[11px] text-text-dim">{reason}</pre>
                <Button variant="tonal" onClick={() => void retry()} disabled={retrying}>
                  {retrying ? "Starting…" : "Retry"}
                </Button>
              </>
            )}
            {phase === "disconnected" && (
              <>
                <p className="text-text-dim">Browser disconnected.{reason ? ` ${reason}` : ""}</p>
                <Button variant="tonal" onClick={reconnect}>
                  Reconnect
                </Button>
              </>
            )}
            {phase === "revoked" && (
              <p className="text-danger">Access revoked. Sign in again if this is unexpected.</p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
