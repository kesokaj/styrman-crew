import { useNavigate, useRouterState } from '@tanstack/react-router'
import { api } from '../lib/api'
import { usePolled } from '../lib/hooks'

export function SetupBanner() {
  const navigate = useNavigate()
  const onSetupPage = useRouterState({ select: (s) => s.location.pathname === '/setup' })
  const { data: st, refresh } = usePolled(() => api.setupStatus(), 10000)

  if (!st || st.done) return null

  const items: Array<[string, boolean]> = [
    ['git', st.gitIdentity],
    ['ssh', st.sshKey],
    ['gh', st.github],
    ['claude', st.claude],
    ['keyring', st.keyring],
  ]

  const markDone = async () => {
    if (!confirm('Mark setup as done? The hidden setup VM is removed; your sign-ins stay in the shared home.')) return
    await api.setupDone()
    refresh()
    if (onSetupPage) navigate({ to: '/' })
  }

  return (
    <div className="mx-2 mb-1 rounded-md border border-warn/30 bg-warn/10 p-2.5 text-xs">
      <div className="mb-1.5 font-medium">First-time setup</div>
      <div className="mb-2 flex flex-wrap gap-x-2 gap-y-0.5 text-text-dim">
        {items.map(([label, ok]) => (
          <span key={label} className={ok ? 'text-ok' : ''}>
            {ok ? '✓' : '○'} {label}
          </span>
        ))}
      </div>
      <div className="flex gap-1.5">
        <button
          onClick={() => navigate({ to: '/setup' })}
          className="flex-1 rounded bg-warn/20 px-2 py-1 font-medium text-warn hover:brightness-110"
        >
          Open setup
        </button>
        <button onClick={markDone} className="rounded px-2 py-1 text-text-dim hover:text-text">
          Done
        </button>
      </div>
    </div>
  )
}
