import { useState } from 'react'
import { Link, useNavigate, useParams } from '@tanstack/react-router'
import { api, type Project } from '../lib/api'
import { usePolled, useTheme } from '../lib/hooks'
import { NewProjectDialog } from './NewProjectDialog'
import { SetupBanner } from './SetupBanner'
import { ConversationList } from './ConversationList'

export function Sidebar() {
  const navigate = useNavigate()
  const params = useParams({ strict: false }) as { project?: string; conv?: string }
  const { data, refresh } = usePolled(() => api.listProjects(), 5000)
  const { theme, toggle } = useTheme()
  const [showNew, setShowNew] = useState(false)
  const [menuFor, setMenuFor] = useState<string | null>(null)

  const projects = data?.projects ?? []

  const openProject = async (p: Project) => {
    const { conversations } = await api.listConversations(p.name)
    const first = conversations[0] ?? (await api.createConversation(p.name))
    navigate({ to: '/p/$project/$conv', params: { project: p.name, conv: first.id } })
  }

  const projectAction = async (p: Project, action: 'stop' | 'start' | 'delete') => {
    setMenuFor(null)
    if (action === 'stop') await api.stopProject(p.name)
    if (action === 'start') await api.startProject(p.name)
    if (action === 'delete') {
      const wipe = confirm(
        `Delete project "${p.name}"?\n\nOK = also wipe its workspace and docker disk (irreversible).\nCancel here, then choose again, keeps files.`,
      )
      if (!confirm(`Really delete "${p.name}"${wipe ? ' AND its files' : ' (files kept)'}?`)) return
      await api.deleteProject(p.name, wipe)
      if (params.project === p.name) navigate({ to: '/' })
    }
    refresh()
  }

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-border bg-bg-raised">
      <div className="flex items-center justify-between px-3 py-3">
        <span className="text-sm font-semibold tracking-wide">⛵ styrman</span>
        <button
          onClick={() => setShowNew(true)}
          className="rounded-md bg-accent-soft px-2.5 py-1 text-xs font-medium text-accent hover:brightness-110"
        >
          + New Project
        </button>
      </div>

      <SetupBanner />

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        <div className="px-1 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wider text-text-dim">
          Projects
        </div>
        {projects.length === 0 && (
          <p className="px-2 py-4 text-xs text-text-dim">
            No projects yet. Create one to get an agent terminal.
          </p>
        )}
        {projects.map((p) => (
          <div key={p.name} className="mb-0.5">
            <div
              className={`group flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 hover:bg-bg-hover ${
                params.project === p.name ? 'bg-bg-hover' : ''
              }`}
              onClick={() => openProject(p)}
            >
              <span
                className={`h-1.5 w-1.5 shrink-0 rounded-full ${
                  p.state === 'running' ? 'bg-ok' : 'bg-border'
                }`}
                title={p.state}
              />
              <span className="min-w-0 flex-1 truncate">{p.name}</span>
              <span className="text-[10px] text-text-dim">{p.agent}</span>
              <button
                className="invisible rounded px-1 text-text-dim hover:text-text group-hover:visible"
                onClick={(e) => {
                  e.stopPropagation()
                  setMenuFor(menuFor === p.name ? null : p.name)
                }}
                aria-label={`Project ${p.name} actions`}
              >
                ⋯
              </button>
            </div>
            {menuFor === p.name && (
              <div className="mx-2 mb-1 flex gap-1 rounded-md border border-border bg-bg p-1 text-xs">
                {p.state === 'running' ? (
                  <button className="flex-1 rounded px-2 py-1 hover:bg-bg-hover" onClick={() => projectAction(p, 'stop')}>
                    Stop
                  </button>
                ) : (
                  <button className="flex-1 rounded px-2 py-1 hover:bg-bg-hover" onClick={() => projectAction(p, 'start')}>
                    Start
                  </button>
                )}
                <button
                  className="flex-1 rounded px-2 py-1 text-danger hover:bg-bg-hover"
                  onClick={() => projectAction(p, 'delete')}
                >
                  Delete
                </button>
              </div>
            )}
            {params.project === p.name && (
              <ConversationList project={p.name} activeConv={params.conv} />
            )}
          </div>
        ))}
      </nav>

      <div className="flex items-center justify-between border-t border-border px-3 py-2 text-xs text-text-dim">
        <Link to="/" className="hover:text-text">
          Home
        </Link>
        <button onClick={toggle} className="hover:text-text">
          {theme === 'dark' ? '☀ Light' : '☾ Dark'}
        </button>
      </div>

      {showNew && (
        <NewProjectDialog
          onClose={() => setShowNew(false)}
          onCreated={(p, convId) => {
            setShowNew(false)
            refresh()
            navigate({ to: '/p/$project/$conv', params: { project: p, conv: convId } })
          }}
        />
      )}
    </aside>
  )
}
