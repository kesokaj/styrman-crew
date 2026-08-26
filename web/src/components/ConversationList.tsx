import { useNavigate } from '@tanstack/react-router'
import { api, tryUI } from '../lib/api'
import { usePolled } from '../lib/hooks'

export function ConversationList({ project, activeConv }: { project: string; activeConv?: string }) {
  const navigate = useNavigate()
  const { data, refresh } = usePolled(() => api.listConversations(project), 5000)
  const conversations = data?.conversations ?? []

  const open = (id: string) =>
    navigate({ to: '/p/$project/$conv', params: { project, conv: id } })

  const create = () =>
    tryUI(async () => {
      const rec = await api.createConversation(project)
      refresh()
      open(rec.id)
    })

  const remove = async (id: string, title: string) => {
    if (!confirm(`Delete "${title}"? Its terminal session ends; the agent's transcript stays in the home.`)) return
    await tryUI(async () => {
      await api.deleteConversation(project, id)
      refresh()
      if (activeConv === id) navigate({ to: '/' })
    })
  }

  return (
    <div className="ml-4 border-l border-border pl-1">
      {conversations.map((c) => (
        <div
          key={c.id}
          className={`group flex cursor-pointer items-center gap-1.5 rounded-md px-2 py-1 text-xs hover:bg-bg-hover ${
            activeConv === c.id ? 'bg-bg-active text-text' : 'text-text-dim'
          }`}
          onClick={() => open(c.id)}
        >
          <span className="min-w-0 flex-1 truncate">{c.title}</span>
          {c.live && <span className="h-1 w-1 rounded-full bg-ok" title="live session" />}
          <button
            className="invisible text-text-dim hover:text-danger group-hover:visible"
            onClick={(e) => {
              e.stopPropagation()
              remove(c.id, c.title)
            }}
            aria-label={`Delete ${c.title}`}
          >
            ✕
          </button>
        </div>
      ))}
      <button
        className="w-full rounded-md px-2 py-1 text-left text-xs text-accent hover:bg-bg-hover"
        onClick={create}
      >
        + New conversation
      </button>
    </div>
  )
}
