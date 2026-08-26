import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
  useParams,
} from '@tanstack/react-router'
import './index.css'
import { api } from './lib/api'
import { usePolled } from './lib/hooks'
import { Sidebar } from './components/Sidebar'
import { RightRail } from './components/RightRail'
import { TerminalPane } from './components/TerminalPane'

function Shell() {
  return (
    <div className="flex h-full">
      <Sidebar />
      <main className="flex min-w-0 flex-1">
        <Outlet />
      </main>
    </div>
  )
}

function Home() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 text-text-dim">
      <div className="text-4xl">⛵</div>
      <p className="text-sm">Pick a project, or create one.</p>
      <p className="max-w-sm text-center text-xs">
        Every project runs in its own microVM with its own Docker daemon. The agent CLI in the
        terminal is the conversation.
      </p>
    </div>
  )
}

function ConversationPage() {
  const { project, conv } = useParams({ from: '/p/$project/$conv' })
  const { data, refresh } = usePolled(() => api.listConversations(project), 10000)
  const record = data?.conversations.find((c) => c.id === conv)
  const [editing, setEditing] = useState(false)
  const [title, setTitle] = useState('')

  const saveTitle = async () => {
    setEditing(false)
    if (title.trim() && title.trim() !== record?.title) {
      await api.renameConversation(project, conv, title.trim())
      refresh()
    }
  }

  return (
    <>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-bg-raised px-3 text-xs">
          <span className="text-text-dim">{project}</span>
          <span className="text-text-dim">/</span>
          {editing ? (
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onBlur={saveTitle}
              onKeyDown={(e) => e.key === 'Enter' && saveTitle()}
              className="rounded border border-border bg-bg px-1.5 py-0.5 outline-none focus:border-accent"
            />
          ) : (
            <button
              className="font-medium hover:underline"
              title="Rename conversation"
              onClick={() => {
                setTitle(record?.title ?? '')
                setEditing(true)
              }}
            >
              {record?.title ?? `Conversation ${conv}`}
            </button>
          )}
          {record?.live && <span className="h-1.5 w-1.5 rounded-full bg-ok" title="live session" />}
        </header>
        <div className="min-h-0 flex-1">
          <TerminalPane
            key={`${project}/${conv}`}
            target={{ kind: 'conv', project, id: conv }}
            onExited={refresh}
          />
        </div>
      </div>
      <RightRail project={project} />
    </>
  )
}

function SetupPage() {
  const [ready, setReady] = useState(false)
  const [error, setError] = useState<string>()
  useEffect(() => {
    api
      .setupStart()
      .then(() => setReady(true))
      .catch((e: Error) => setError(e.message))
  }, [])
  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <header className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-bg-raised px-3 text-xs">
        <span className="font-medium">First-time setup</span>
        <span className="text-text-dim">
          — sign in to git, GitHub, claude and agy once; everything lands in the shared home
        </span>
      </header>
      <div className="min-h-0 flex-1">
        {error && <p className="p-4 text-xs text-danger">{error}</p>}
        {!error && !ready && (
          <p className="p-4 text-xs text-text-dim">Booting the hidden setup VM…</p>
        )}
        {ready && <TerminalPane target={{ kind: 'setup' }} />}
      </div>
    </div>
  )
}

const rootRoute = createRootRoute({ component: Shell })
const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', component: Home })
const convRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/p/$project/$conv',
  component: ConversationPage,
})
const setupRoute = createRoute({ getParentRoute: () => rootRoute, path: '/setup', component: SetupPage })

const router = createRouter({
  routeTree: rootRoute.addChildren([indexRoute, convRoute, setupRoute]),
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
)
