import { useState } from 'react'
import { api, type Agent } from '../lib/api'

export function NewProjectDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void
  onCreated: (project: string, convId: string) => void
}) {
  const [name, setName] = useState('')
  const [agent, setAgent] = useState<Agent>('claude')
  const [gitInit, setGitInit] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()

  const valid = /^[a-z][a-z0-9-]{0,30}$/.test(name)

  const create = async () => {
    setBusy(true)
    setError(undefined)
    try {
      const p = await api.createProject(name, agent, gitInit)
      const conv = await api.createConversation(p.name)
      onCreated(p.name, conv.id)
    } catch (e) {
      setError((e as Error).message)
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="w-80 rounded-lg border border-border bg-bg-raised p-4 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-3 text-sm font-semibold">New project</h2>
        <label className="mb-1 block text-xs text-text-dim" htmlFor="np-name">
          Name (lowercase, digits, dashes)
        </label>
        <input
          id="np-name"
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && valid && !busy && create()}
          placeholder="my-project"
          className="mb-3 w-full rounded-md border border-border bg-bg px-2 py-1.5 outline-none focus:border-accent"
        />
        <div className="mb-1 block text-xs text-text-dim">Agent</div>
        <div className="mb-3 flex gap-2">
          {(['claude', 'agy'] as const).map((a) => (
            <button
              key={a}
              onClick={() => setAgent(a)}
              className={`flex-1 rounded-md border px-2 py-1.5 text-xs ${
                agent === a
                  ? 'border-accent bg-accent-soft text-accent'
                  : 'border-border hover:bg-bg-hover'
              }`}
            >
              {a === 'claude' ? 'Claude Code' : 'Antigravity (agy)'}
            </button>
          ))}
        </div>
        <label className="mb-4 flex items-center gap-2 text-xs">
          <input type="checkbox" checked={gitInit} onChange={(e) => setGitInit(e.target.checked)} />
          Initialize a git repository
        </label>
        {error && <p className="mb-3 text-xs text-danger">{error}</p>}
        <div className="flex justify-end gap-2 text-xs">
          <button onClick={onClose} className="rounded-md px-3 py-1.5 hover:bg-bg-hover">
            Cancel
          </button>
          <button
            onClick={create}
            disabled={!valid || busy}
            className="rounded-md bg-accent px-3 py-1.5 font-medium text-white disabled:opacity-40"
          >
            {busy ? 'Booting microVM…' : 'Create'}
          </button>
        </div>
      </div>
    </div>
  )
}
