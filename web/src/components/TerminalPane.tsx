import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { CLOSE_EXITED, CLOSE_REPLACED, termSocketUrl } from '../lib/api'

export type TermTarget =
  | { kind: 'conv' | 'shell'; project: string; id: string }
  | { kind: 'setup' }

type ConnState =
  | 'connecting'
  | 'connected'
  | 'exited'
  | 'replaced'
  | 'reconnecting'
  | 'disconnected'

// Only these close codes are transient per the protocol spec; everything
// else (failed upgrade, server error) must not loop forever.
const TRANSIENT_CODES = new Set([1000, 1001, 1006])
const MAX_RETRIES = 8

const THEME = {
  background: '#0d0e10',
  foreground: '#d6d8dd',
  cursor: '#5b9dd9',
  selectionBackground: '#2b3038',
}

/**
 * One xterm.js terminal bound to a styrman WebSocket. Implements the wire
 * protocol: binary frames are PTY bytes, JSON text frames carry resize,
 * close 4000 = session exited (no auto-reconnect), close 4001 = replaced by
 * another tab (NEVER reconnect), anything else = transient → reconnect.
 */
export function TerminalPane({ target, onExited }: { target: TermTarget; onExited?: () => void }) {
  const hostRef = useRef<HTMLDivElement>(null)
  const [state, setState] = useState<ConnState>('connecting')
  // Bumping epoch remounts the connection — the "Reconnect" button for the
  // exited/disconnected terminal states.
  const [epoch, setEpoch] = useState(0)

  // Key the whole effect on the logical target so switching conversations
  // tears down cleanly and reconnects fresh.
  const targetKey =
    (target.kind === 'setup' ? 'setup' : `${target.kind}/${target.project}/${target.id}`) +
    `#${epoch}`

  useEffect(() => {
    const host = hostRef.current
    if (!host) return

    const term = new Terminal({
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      cursorBlink: true,
      scrollback: 10000,
      theme: THEME,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    fit.fit()

    let ws: WebSocket | null = null
    let closedForGood = false
    let retries = 0
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined

    const connect = () => {
      if (closedForGood) return
      setState((prev) => (prev === 'connecting' ? 'connecting' : 'reconnecting'))
      ws = new WebSocket(termSocketUrl(target, term.cols, term.rows))
      ws.binaryType = 'arraybuffer'
      ws.onopen = () => {
        retries = 0
        setState('connected')
        // The URL size only seeds session *creation*; a reattach needs an
        // explicit resize in case the pane changed size while disconnected.
        fit.fit()
        ws?.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
        term.focus()
      }
      ws.onmessage = (ev) => term.write(new Uint8Array(ev.data as ArrayBuffer))
      ws.onclose = (ev) => {
        if (closedForGood) return
        if (ev.code === CLOSE_REPLACED) {
          setState('replaced')
          closedForGood = true // protocol: never reconnect on 4001
          return
        }
        if (ev.code === CLOSE_EXITED) {
          setState('exited')
          closedForGood = true
          onExited?.()
          return
        }
        // Transient drops reattach with exponential backoff; anything else
        // (or exhausted retries) parks in 'disconnected' with a manual retry.
        if (TRANSIENT_CODES.has(ev.code) && retries < MAX_RETRIES) {
          retries++
          reconnectTimer = setTimeout(connect, Math.min(1000 * 2 ** retries, 15000))
        } else {
          setState('disconnected')
          closedForGood = true
        }
      }
    }
    connect()

    const dataSub = term.onData((d) => {
      if (ws?.readyState === WebSocket.OPEN) {
        // Binary frame = raw input bytes (text frames are control messages).
        ws.send(new TextEncoder().encode(d).buffer as ArrayBuffer)
      }
    })

    const onResize = () => {
      fit.fit()
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    const resizeObserver = new ResizeObserver(onResize)
    resizeObserver.observe(host)

    return () => {
      closedForGood = true
      clearTimeout(reconnectTimer)
      dataSub.dispose()
      resizeObserver.disconnect()
      if (ws) {
        ws.onclose = null
        ws.close()
      }
      term.dispose()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetKey])

  return (
    <div className="relative h-full w-full bg-term-bg">
      <div ref={hostRef} className="term-host h-full w-full p-1.5" />
      {state !== 'connected' && (
        <div className="absolute inset-x-0 top-0 z-10 flex justify-center gap-2 pt-2">
          <span className="rounded-full border border-border bg-bg-raised px-3 py-1 text-xs text-text-dim">
            {state === 'connecting' && 'Connecting…'}
            {state === 'reconnecting' && 'Reconnecting…'}
            {state === 'exited' && 'Session ended'}
            {state === 'replaced' && 'Opened in another tab'}
            {state === 'disconnected' && 'Disconnected'}
          </span>
          {(state === 'exited' || state === 'disconnected') && (
            <button
              onClick={() => {
                setState('connecting')
                setEpoch((e) => e + 1)
              }}
              className="rounded-full border border-accent bg-accent-soft px-3 py-1 text-xs text-accent hover:brightness-110"
            >
              {state === 'exited' ? 'Start again' : 'Reconnect'}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
