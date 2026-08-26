import { useState } from 'react'
import { api, tryUI } from '../lib/api'
import { usePolled } from '../lib/hooks'
import { TerminalPane } from './TerminalPane'

/**
 * Collapsible right rail: the project's plain-shell terminals (ephemeral,
 * tmux `term-*` — not conversations). The selected terminal renders in the
 * lower half of the rail.
 */
export function RightRail({ project }: { project: string }) {
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState<string | null>(null)
  const { data, refresh } = usePolled(() => api.listTerminals(project), 5000)
  const terminals = data?.terminals ?? []
  const { data: portData } = usePolled(() => api.listPorts(project), 7000)
  const ports = portData?.ports ?? []

  const create = () =>
    tryUI(async () => {
      const { id } = await api.createTerminal(project)
      setActive(id)
      setOpen(true)
      refresh()
    })

  const kill = (id: string) =>
    tryUI(async () => {
      await api.deleteTerminal(project, id)
      if (active === id) setActive(null)
      refresh()
    })

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="flex h-full w-8 shrink-0 items-center justify-center border-l border-border bg-bg-raised text-text-dim hover:text-text"
        title="Terminals"
      >
        <span className="rotate-90 whitespace-nowrap text-[11px] tracking-wider">TERMINALS</span>
      </button>
    )
  }

  return (
    <aside className="flex h-full w-80 shrink-0 flex-col border-l border-border bg-bg-raised">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="text-[11px] font-medium uppercase tracking-wider text-text-dim">Terminals</span>
        <div className="flex gap-2 text-xs">
          <button onClick={create} className="text-accent hover:brightness-110">
            + New
          </button>
          <button onClick={() => setOpen(false)} className="text-text-dim hover:text-text">
            ⨯
          </button>
        </div>
      </div>
      <div className="max-h-40 overflow-y-auto p-1.5">
        {terminals.length === 0 && (
          <p className="px-1.5 py-2 text-xs text-text-dim">No terminals. Open one for a plain shell in this project's VM.</p>
        )}
        {terminals.map((t) => (
          <div
            key={t.id}
            className={`group flex cursor-pointer items-center justify-between rounded px-2 py-1 text-xs hover:bg-bg-hover ${
              active === t.id ? 'bg-bg-active' : 'text-text-dim'
            }`}
            onClick={() => setActive(t.id)}
          >
            <span>shell {t.id}</span>
            <button
              className="invisible hover:text-danger group-hover:visible"
              onClick={(e) => {
                e.stopPropagation()
                kill(t.id)
              }}
              aria-label={`Kill shell ${t.id}`}
            >
              ✕
            </button>
          </div>
        ))}
      </div>
      {ports.length > 0 && (
        <div className="border-t border-border p-1.5">
          <div className="px-1.5 pb-1 text-[11px] font-medium uppercase tracking-wider text-text-dim">
            Ports
          </div>
          <div className="flex flex-wrap gap-1 px-1.5">
            {ports.map((p) => (
              <a
                key={p}
                href={`/proxy/${project}/${p}/`}
                target="_blank"
                rel="noreferrer"
                className="rounded bg-accent-soft px-2 py-0.5 text-xs text-accent hover:brightness-110"
                title={`Open port ${p} from inside the microVM`}
              >
                :{p}
              </a>
            ))}
          </div>
        </div>
      )}
      <div className="min-h-0 flex-1 border-t border-border">
        {active ? (
          <TerminalPane
            key={`${project}/${active}`}
            target={{ kind: 'shell', project, id: active }}
            onExited={() => {
              setActive(null)
              refresh()
            }}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-text-dim">
            Select or open a terminal
          </div>
        )}
      </div>
    </aside>
  )
}
