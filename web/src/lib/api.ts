// Typed client for the styrman REST API (spec: .agents/docs/specs/styrman/api.md).

export interface Project {
  id: string; // UUID — the identity everything keys on
  name: string; // friendly, renamable label
  dir: string; // workspace dir under ~/projects (frozen at creation)
  createdAt: string;
  gitInit: boolean;
  archived: boolean;
  state: "running" | "stopped" | "absent";
}

/** A terminal: a durable record whose live view is a tmux session. */
export interface Terminal {
  id: string;
  title: string;
  createdAt: string;
  lastOpened: string;
  live: boolean;
  attached: boolean;
}

export interface FileEntry {
  name: string;
  path: string;
  dir: boolean;
  size: number;
  modified: string;
  symlink: boolean;
}

export interface FileContent {
  path: string;
  content: string;
  size: number;
  truncated: boolean;
  binary: boolean;
}

export interface Transcript {
  id: string;
  size: number;
  modified: string;
}

export interface Diff {
  repo: boolean;
  branch?: string;
  status?: string;
  diff?: string;
}

export interface PortInfo {
  port: number;
  process?: string;
  image?: string;
}

export interface ToolInfo {
  name: string;
  ready: boolean;
  details?: string[];
  skills?: string[];
  agents?: string[];
  commands?: string[];
  configDir?: string;
}

export interface Settings {
  version: string;
  user: string;
  username: string;
  isAdmin: boolean;
  authMode: string;
  allowedUsers: string[];
  homeDir: string;
  stateDir: string;
  image: string;
  runtime: string;
  dockerDiskGB: number;
  idleTimeout: string;
  terminalUsername: string;
  setupComplete: boolean;
}

export interface SetupStatus {
  gitIdentity: boolean;
  sshKey: boolean;
  github: boolean;
  claude: boolean;
  keyring: boolean;
  done: boolean;
  vmRunning: boolean;
}

/** Run a UI action; surface API failures instead of swallowing the rejection. */
export async function tryUI(action: () => Promise<void>): Promise<void> {
  try {
    await action();
  } catch (e) {
    alert((e as Error).message || "Request failed");
  }
}

export class ApiError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(body?.error?.message ?? res.statusText, res.status);
  }
  return body as T;
}

/** File-explorer scope: a project's workspace, or the shared home. */
export type Scope = { kind: "project"; project: string } | { kind: "home" };

const scopeBase = (s: Scope) =>
  s.kind === "home" ? "/api/home" : `/api/projects/${s.project}`;

export const api = {
  listProjects: () => request<{ projects: Project[] }>("/api/projects"),
  createProject: (name: string, gitInit: boolean) =>
    request<Project>("/api/projects", {
      method: "POST",
      body: JSON.stringify({ name, gitInit }),
    }),
  getProject: (id: string) => request<Project>(`/api/projects/${id}`),
  renameProject: (id: string, name: string) =>
    request<void>(`/api/projects/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name }),
    }),
  deleteProject: (id: string, wipe: boolean) =>
    request<void>(`/api/projects/${id}${wipe ? "?wipe=true" : ""}`, {
      method: "DELETE",
    }),
  startProject: (id: string) =>
    request<void>(`/api/projects/${id}/start`, { method: "POST" }),
  stopProject: (id: string) =>
    request<void>(`/api/projects/${id}/stop`, { method: "POST" }),

  listTerminals: (project: string) =>
    request<{ terminals: Terminal[] }>(`/api/projects/${project}/terminals`),
  createTerminal: (project: string, title?: string) =>
    request<Terminal>(`/api/projects/${project}/terminals`, {
      method: "POST",
      body: JSON.stringify({ title: title ?? "" }),
    }),
  renameTerminal: (project: string, id: string, title: string) =>
    request<void>(`/api/projects/${project}/terminals/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ title }),
    }),
  deleteTerminal: (project: string, id: string) =>
    request<void>(`/api/projects/${project}/terminals/${id}`, {
      method: "DELETE",
    }),
  deleteAllTerminals: (project: string) =>
    request<void>(`/api/projects/${project}/terminals`, { method: "DELETE" }),
  pauseTerminal: (project: string, id: string) =>
    request<void>(`/api/projects/${project}/terminals/${id}/pause`, {
      method: "POST",
    }),

  listPorts: (project: string) =>
    request<{ ports: PortInfo[] }>(`/api/projects/${project}/ports`),

  listFiles: (scope: Scope, path: string) =>
    request<{ entries: FileEntry[] }>(
      `${scopeBase(scope)}/files?path=${encodeURIComponent(path)}`,
    ),
  readFile: (scope: Scope, path: string) =>
    request<FileContent>(
      `${scopeBase(scope)}/file?path=${encodeURIComponent(path)}`,
    ),
  saveFile: (scope: Scope, path: string, content: string) =>
    request<void>(`${scopeBase(scope)}/file?path=${encodeURIComponent(path)}`, {
      method: "PUT",
      body: JSON.stringify({ content }),
    }),
  downloadUrl: (scope: Scope, path: string) =>
    `${scopeBase(scope)}/download?path=${encodeURIComponent(path)}`,
  uploadFiles: async (scope: Scope, dir: string, fileList: File[]) => {
    const form = new FormData()
    for (const f of fileList) form.append('files', f, f.name)
    const res = await fetch(`${scopeBase(scope)}/upload?path=${encodeURIComponent(dir)}`, {
      method: 'POST',
      body: form,
    })
    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      throw new ApiError(body?.error?.message ?? res.statusText, res.status)
    }
  },

  listHistory: (project: string) =>
    request<{ transcripts: Transcript[] }>(`/api/projects/${project}/history`),
  readHistory: (project: string, id: string) =>
    request<{ id: string; text: string }>(
      `/api/projects/${project}/history/${id}`,
    ),
  diff: (project: string) => request<Diff>(`/api/projects/${project}/diff`),

  settings: () => request<Settings>("/api/settings"),
  tools: () => request<{ tools: ToolInfo[] }>("/api/settings/tools"),
  setTerminalUsername: (name: string) =>
    request<{ terminalUsername: string }>("/api/settings/profile", {
      method: "POST",
      body: JSON.stringify({ terminalUsername: name }),
    }),
  addUser: (email: string) =>
    request<{ allowedUsers: string[] }>("/api/settings/users", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  removeUser: (email: string) =>
    request<{ allowedUsers: string[] }>(
      `/api/settings/users/${encodeURIComponent(email)}`,
      {
        method: "DELETE",
      },
    ),

  setupStatus: () => request<SetupStatus>("/api/setup/status"),
  setupStart: () => request<void>("/api/setup/start", { method: "POST" }),
  setupDone: () => request<void>("/api/setup/done", { method: "POST" }),
};

/** Shareable link to a port exposed inside a project's microVM. */
export const portUrl = (project: string, term: string, port: number) =>
  `/p/${project}/${term}/port/${port}/`;

/** Terminal WebSocket URL. */
export function termSocketUrl(
  target: { kind: "term"; project: string; id: string } | { kind: "setup" },
  cols: number,
  rows: number,
): string {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const size = `cols=${cols}&rows=${rows}`;
  if (target.kind === "setup")
    return `${proto}://${location.host}/ws/setup?${size}`;
  return `${proto}://${location.host}/ws/term/${target.project}/${target.id}?${size}`;
}

/** Terminal protocol close codes (spec: terminal-protocol.md). */
export const CLOSE_EXITED = 4000;
export const CLOSE_REPLACED = 4001;
