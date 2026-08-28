// Typed client for the styrman REST API (spec: .agents/docs/specs/styrman/api.md).

import { toast } from "./toast";

export interface Project {
  id: string; // UUID — the identity everything keys on
  name: string; // friendly, renamable label
  dir: string; // workspace dir under ~/projects (frozen at creation)
  createdAt: string;
  gitInit: boolean;
  state: "running" | "stopped" | "absent" | "creating" | "deleting";
  /** Last failed background destroy — the DELETE answered 202, so this is
   * how the user learns the delete didn't stick. */
  deleteError?: string;
}

/** A terminal: a durable record whose live view is a tmux session. */
export interface Terminal {
  id: string;
  title: string;
  /** Autostart: runs when the tmux session is created (first open, and
   * re-opens after pause/VM restart), then drops to the shell. */
  command?: string;
  createdAt: string;
  lastOpened: string;
  live: boolean;
  attached: boolean;
  /** Rang the bell while unattended — an agent is likely waiting for input. */
  bell: boolean;
}

/** A terminal waiting for attention (bell rung while unattended). */
export interface AppNotification {
  projectId: string;
  projectName: string;
  terminalId: string;
  title: string;
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
  /** The terminal's friendly title, when its record still exists. */
  title?: string;
}

/** One full-text search hit across the project's transcripts. */
export interface HistoryMatch {
  transcript: string;
  line: string;
  title?: string;
}

export interface Diff {
  repo: boolean;
  branch?: string;
  status?: string;
  diff?: string;
}

/** A whole-tree snapshot stored under refs/styrman/* — never on branches. */
export interface Checkpoint {
  id: string;
  time: string;
  subject: string;
}

export interface PortInfo {
  port: number;
  process?: string;
  image?: string;
}

/** A public, expiring capability link to one exposed port (no login needed). */
export interface PublicLink {
  token: string;
  port: number;
  url: string;
  expires: string;
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
  homeDir: string;
  stateDir: string;
  image: string;
  runtime: string;
  dockerDiskGB: number;
  idleTimeout: string;
  terminalUsername: string;
  /** True once the user explicitly picked a name (setup won't re-ask). */
  terminalUsernameChosen: boolean;
  setupComplete: boolean;
}

/** A stored credential, value-free. `id` is the delete key (host | KEY | file
 *  id); `name` is the display label (host | KEY | path). */
export interface CredItem {
  kind: "git" | "env" | "file" | "keyring";
  id: string;
  name: string;
}

/** Per-tool result of an import-from-project pass. */
export type CredImportReport = Record<string, string>;

export interface SetupStatus {
  gitIdentity: boolean;
  sshKey: boolean;
  github: boolean;
  claude: boolean;
  keyring: boolean;
  done: boolean;
  vmRunning: boolean;
}

/** Client-side mirror of the server's project-name rule (project.nameRe) —
 *  shared by create and rename so both fail fast with the same message. */
export function projectNameError(name: string): string | undefined {
  return /^[a-z][a-z0-9-]{0,30}$/.test(name)
    ? undefined
    : "lowercase letters, digits and dashes; starts with a letter (max 31)";
}

/** Run a UI action; surface API failures instead of swallowing the rejection. */
export async function tryUI(action: () => Promise<void>): Promise<void> {
  try {
    await action();
  } catch (e) {
    toast("error", (e as Error).message || "Request failed");
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
    // Don't hang forever if the server is wedged (a stuck VM boot pinning the
    // box); surface a timeout the UI can show instead of a spinner forever.
    signal: init?.signal ?? AbortSignal.timeout(20000),
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
// Homes are per-project (ADR-031), so the home scope carries its project too.
export type Scope =
  | { kind: "project"; project: string }
  | { kind: "home"; project: string };

const scopeBase = (s: Scope) =>
  s.kind === "home"
    ? `/api/projects/${s.project}/home`
    : `/api/projects/${s.project}`;

export const api = {
  listProjects: () => request<{ projects: Project[] }>("/api/projects"),
  createProject: (
    name: string,
    opts: { github?: "none" | "private" | "public"; cloneUrl?: string } = {},
  ) =>
    request<Project>("/api/projects", {
      method: "POST",
      body: JSON.stringify({
        name,
        github: opts.github ?? "none",
        cloneUrl: opts.cloneUrl ?? "",
      }),
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
  // Start/stop legitimately take minutes on a slow Kata boot/teardown — the
  // default 20 s abort made the UI report failures for operations that then
  // completed server-side.
  startProject: (id: string) =>
    request<void>(`/api/projects/${id}/start`, {
      method: "POST",
      signal: AbortSignal.timeout(150000),
    }),
  stopProject: (id: string) =>
    request<void>(`/api/projects/${id}/stop`, {
      method: "POST",
      signal: AbortSignal.timeout(150000),
    }),

  listTerminals: (project: string) =>
    request<{ terminals: Terminal[] }>(`/api/projects/${project}/terminals`),
  createTerminal: (project: string, title?: string) =>
    request<Terminal>(`/api/projects/${project}/terminals`, {
      method: "POST",
      body: JSON.stringify({ title: title ?? "" }),
    }),
  updateTerminal: (
    project: string,
    id: string,
    patch: { title?: string; command?: string },
  ) =>
    request<void>(`/api/projects/${project}/terminals/${id}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
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

  notifications: () =>
    request<{ notifications: AppNotification[] }>("/api/notifications"),

  listPorts: (project: string) =>
    request<{ ports: PortInfo[] }>(`/api/projects/${project}/ports`),

  getPortShares: (project: string, port: number) =>
    request<{ emails: string[]; url: string }>(
      `/api/projects/${project}/ports/${port}/shares`,
    ),
  setPortShares: (project: string, port: number, emails: string[]) =>
    request<{ emails: string[]; url: string }>(
      `/api/projects/${project}/ports/${port}/shares`,
      { method: "PUT", body: JSON.stringify({ emails }) },
    ),

  listLinks: (project: string) =>
    request<{ links: PublicLink[] }>(`/api/projects/${project}/links`),
  createLink: (project: string, port: number, ttl: string) =>
    request<PublicLink>(`/api/projects/${project}/ports/${port}/links`, {
      method: "POST",
      body: JSON.stringify({ ttl }),
    }),
  revokeLink: (project: string, token: string) =>
    request<void>(`/api/projects/${project}/links/${token}`, {
      method: "DELETE",
    }),

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
  deleteFile: (scope: Scope, path: string) =>
    request<void>(`${scopeBase(scope)}/file?path=${encodeURIComponent(path)}`, {
      method: "DELETE",
    }),
  renameFile: (scope: Scope, path: string, to: string) =>
    request<void>(
      `${scopeBase(scope)}/file/rename?path=${encodeURIComponent(path)}`,
      { method: "POST", body: JSON.stringify({ to }) },
    ),
  makeDir: (scope: Scope, path: string) =>
    request<void>(
      `${scopeBase(scope)}/mkdir?path=${encodeURIComponent(path)}`,
      { method: "POST" },
    ),
  downloadUrl: (scope: Scope, path: string) =>
    `${scopeBase(scope)}/download?path=${encodeURIComponent(path)}`,
  uploadFiles: async (scope: Scope, dir: string, fileList: File[]) => {
    const form = new FormData();
    for (const f of fileList) form.append("files", f, f.name);
    const res = await fetch(
      `${scopeBase(scope)}/upload?path=${encodeURIComponent(dir)}`,
      {
        method: "POST",
        body: form,
      },
    );
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new ApiError(body?.error?.message ?? res.statusText, res.status);
    }
  },

  listHistory: (project: string) =>
    request<{ transcripts: Transcript[] }>(`/api/projects/${project}/history`),
  searchHistory: (project: string, q: string) =>
    request<{ matches: HistoryMatch[] }>(
      `/api/projects/${project}/history/search?q=${encodeURIComponent(q)}`,
    ),
  readHistory: (project: string, id: string, bytes?: number) =>
    request<{ id: string; text: string }>(
      `/api/projects/${project}/history/${id}${bytes ? `?bytes=${bytes}` : ""}`,
    ),
  diff: (project: string) => request<Diff>(`/api/projects/${project}/diff`),

  listCheckpoints: (project: string) =>
    request<{ checkpoints: Checkpoint[] }>(
      `/api/projects/${project}/checkpoints`,
    ),
  createCheckpoint: (project: string, label?: string) =>
    request<{ id: string; created: boolean }>(
      `/api/projects/${project}/checkpoints`,
      { method: "POST", body: JSON.stringify({ label: label ?? "" }) },
    ),
  restoreCheckpoint: (project: string, id: string) =>
    request<{ restored: string; safetyId: string }>(
      `/api/projects/${project}/checkpoints/${id}/restore`,
      { method: "POST" },
    ),

  settings: () => request<Settings>("/api/settings"),
  tools: () => request<{ tools: ToolInfo[] }>("/api/settings/tools"),
  updateRuntime: (patch: { dockerDiskGB?: number; idleTimeout?: string }) =>
    request<{ dockerDiskGB: number; idleTimeout: string }>(
      "/api/settings/runtime",
      { method: "PATCH", body: JSON.stringify(patch) },
    ),
  setTerminalUsername: (name: string) =>
    request<{ terminalUsername: string }>("/api/settings/profile", {
      method: "POST",
      body: JSON.stringify({ terminalUsername: name }),
    }),
  // User-access management is API-only since v1.3 (operator Q4): the
  // /api/settings/users endpoints exist for curl, but the UI card is gone.

  // Credential broker (ADR-032): the store holds one live copy of each
  // credential; the broker serves the current copy to every project.
  credentials: () =>
    request<{ items: CredItem[]; brokerEnabled?: boolean }>(
      "/api/settings/credentials",
    ),
  importCredentials: (project: string) =>
    request<{ report: CredImportReport }>(
      "/api/settings/credentials/import",
      { method: "POST", body: JSON.stringify({ project }) },
    ),
  putCredentialToken: (key: string, value: string) =>
    request<{ key: string; status: string }>(
      "/api/settings/credentials/token",
      { method: "POST", body: JSON.stringify({ key, value }) },
    ),
  deleteCredential: (kind: string, name: string) =>
    request<void>(
      `/api/settings/credentials/${kind}/${encodeURIComponent(name)}`,
      { method: "DELETE" },
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
