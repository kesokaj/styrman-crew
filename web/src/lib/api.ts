// Typed client for the styrman REST API (spec: .agents/docs/specs/styrman/api.md).

export type Agent = 'claude' | 'agy'

export interface Project {
  name: string
  agent: Agent
  createdAt: string
  gitInit: boolean
  archived: boolean
  state: 'running' | 'stopped' | 'absent'
}

export interface Conversation {
  id: string
  title: string
  agentSession?: string
  createdAt: string
  lastOpened: string
  live: boolean
  attached: boolean
}

export interface TerminalSession {
  id: string
  attached: boolean
}

export interface SetupStatus {
  gitIdentity: boolean
  sshKey: boolean
  github: boolean
  claude: boolean
  keyring: boolean
  done: boolean
  vmRunning: boolean
}

class ApiError extends Error {
  readonly status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (res.status === 204) return undefined as T
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(body?.error?.message ?? res.statusText, res.status)
  }
  return body as T
}

export const api = {
  listProjects: () => request<{ projects: Project[] }>('/api/projects'),
  createProject: (name: string, agent: Agent, gitInit: boolean) =>
    request<Project>('/api/projects', {
      method: 'POST',
      body: JSON.stringify({ name, agent, gitInit }),
    }),
  deleteProject: (name: string, wipe: boolean) =>
    request<void>(`/api/projects/${name}${wipe ? '?wipe=true' : ''}`, { method: 'DELETE' }),
  startProject: (name: string) => request<void>(`/api/projects/${name}/start`, { method: 'POST' }),
  stopProject: (name: string) => request<void>(`/api/projects/${name}/stop`, { method: 'POST' }),

  listConversations: (project: string) =>
    request<{ conversations: Conversation[] }>(`/api/projects/${project}/conversations`),
  createConversation: (project: string, title?: string) =>
    request<Conversation>(`/api/projects/${project}/conversations`, {
      method: 'POST',
      body: JSON.stringify({ title: title ?? '' }),
    }),
  renameConversation: (project: string, id: string, title: string) =>
    request<void>(`/api/projects/${project}/conversations/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    }),
  deleteConversation: (project: string, id: string) =>
    request<void>(`/api/projects/${project}/conversations/${id}`, { method: 'DELETE' }),

  listTerminals: (project: string) =>
    request<{ terminals: TerminalSession[] }>(`/api/projects/${project}/terminals`),
  createTerminal: (project: string) =>
    request<{ id: string }>(`/api/projects/${project}/terminals`, { method: 'POST' }),
  deleteTerminal: (project: string, id: string) =>
    request<void>(`/api/projects/${project}/terminals/${id}`, { method: 'DELETE' }),

  setupStatus: () => request<SetupStatus>('/api/setup/status'),
  setupStart: () => request<void>('/api/setup/start', { method: 'POST' }),
  setupDone: () => request<void>('/api/setup/done', { method: 'POST' }),
}

/** Terminal WebSocket URL. kind 'conv' attaches the agent, 'shell' a plain shell, 'setup' the onboarding VM. */
export function termSocketUrl(
  target: { kind: 'conv' | 'shell'; project: string; id: string } | { kind: 'setup' },
  cols: number,
  rows: number,
): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  const size = `cols=${cols}&rows=${rows}`
  if (target.kind === 'setup') return `${proto}://${location.host}/ws/setup?${size}`
  const shell = target.kind === 'shell' ? '&kind=shell' : ''
  return `${proto}://${location.host}/ws/term/${target.project}/${target.id}?${size}${shell}`
}

/** Terminal protocol close codes (spec: terminal-protocol.md). */
export const CLOSE_EXITED = 4000
export const CLOSE_REPLACED = 4001
