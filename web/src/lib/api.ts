// Typed client for the styrman REST API (spec: .agents/docs/specs/styrman/api.md).

import { toast } from "./toast";

export interface Project {
  id: string; // UUID — the identity everything keys on
  name: string; // friendly, renamable label
  dir: string; // workspace dir under ~/projects (frozen at creation)
  createdAt: string;
  state: "running" | "stopped" | "absent" | "creating" | "deleting";
  /** Last failed background destroy — the DELETE answered 202, so this is
   * how the user learns the delete didn't stick. */
  deleteError?: string;
  /** Last failed background repo provisioning (clone / GitHub repo create).
   * POST /api/projects answers 201 before the clone runs, so this is how the
   * user learns why the project came up as an empty repo. */
  provisionError?: string;
}

/** Live resource snapshot of a project's microVM (terminal status bar). */
export interface ProjectStats {
  state: "running" | "stopped" | "absent" | "creating" | "deleting";
  cpuPercent: number;
  cpuCount: number;
  load1: number;
  memUsedBytes: number;
  memTotalBytes: number;
  diskUsedBytes: number;
  diskTotalBytes: number;
  processes: number;
  ip?: string;
  uptimeSeconds: number;
  host?: string;
}

/** One changed path in the git working tree / index. */
export interface GitFile {
  path: string;
  staged: string; // index code (M A D R C) or ""
  unstaged: string; // worktree code, or "?" for untracked
  untracked: boolean;
  origPath?: string;
}

/** Git working-tree status for the project's repo. */
export interface GitStatus {
  state: "running" | "stopped" | "absent";
  repo: boolean;
  branch?: string;
  upstream?: string;
  ahead: number;
  behind: number;
  clean: boolean;
  files: GitFile[];
}

/** Local and remote-tracking branches (v1.22.0). */
export interface GitBranches {
  state: "running" | "stopped";
  repo: boolean;
  current?: string;
  local: string[];
  remote: string[];
}

/** One commit in the log view. */
export interface GitCommit {
  hash: string;
  shortHash: string;
  subject: string;
  author: string;
  date: string;
}

export interface GitLog {
  state: "running" | "stopped";
  repo: boolean;
  commits: GitCommit[];
}

/** One stash entry. Parsed SERVER-side — the frontend never reads porcelain. */
export interface GitStashEntry {
  ref: string;
  subject: string;
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

/** One problem reported by a syntax checker, addressed to a line. */
export interface CheckDiagnostic {
  line: number;
  col?: number;
  severity: "error" | "warning" | "info";
  message: string;
}

export interface CheckResult {
  /** false = no checker exists for this file type (not an error). */
  supported: boolean;
  /** The checker that ran (or would have). */
  tool?: string;
  /** true = the checker is not installed in the project VM. */
  unavailable?: boolean;
  diagnostics: CheckDiagnostic[];
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
  kind: "git" | "env" | "file";
  id: string;
  name: string;
}

/** Per-tool result of an import-from-project pass. */
export type CredImportReport = Record<string, string>;

export interface SetupStatus {
  gitIdentity: boolean; // required
  sshKey: boolean; // required
  github: boolean; // optional
  gcloud: boolean; // optional
  claude: boolean; // optional
  agy: boolean; // optional
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

/**
 * The project name for a "Clone a repo" create: the repository's own name, as
 * the operator expects ("use the same name as the repo being cloned"). Slugged
 * to the server's project-name rules (projectNameError) — a repo called
 * `My_Repo.git` becomes `my-repo`. Returns "" when nothing usable is left, so
 * the caller keeps whatever the user typed.
 */
export function projectNameFromRepoURL(url: string): string {
  const last = url.trim().replace(/\/+$/, "").split(/[/:]/).pop() ?? "";
  return last
    .replace(/\.git$/i, "")
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^[^a-z]+/, "") // must start with a letter
    .replace(/-+$/, "")
    .slice(0, 31)
    .replace(/-+$/, ""); // the slice can leave a trailing dash
}

/**
 * Mirrors the server's cloneURLHasSecret: a clone URL must not embed a
 * password. Nothing needs one — styrman delivers the ssh key and answers git's
 * credential helper per host — and git would write the token verbatim into the
 * project's .git/config. A bare username (ssh://git@host) is not a secret.
 * Returns an error message, or undefined when the URL is fine.
 */
export function cloneURLError(url: string): string | undefined {
  const raw = url.trim();
  if (!raw) return undefined;
  const m = /^[a-z][a-z0-9+.-]*:\/\/([^/]*)@/i.exec(raw);
  if (m?.[1]?.includes(":")) {
    return "Remove the token from the URL — styrman supplies your git credentials automatically.";
  }
  return undefined;
}

/** Run a UI action; surface API failures instead of swallowing the rejection. */
export async function tryUI(action: () => Promise<void>): Promise<void> {
  try {
    await action();
  } catch (e) {
    toast("error", (e as Error).message || "Request failed");
  }
}

class ApiError extends Error {
  readonly status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

// reAuthOnce: session-expiry handling (2026-08-30 audit item). The OAuth
// ingress answers an expired session with a REDIRECT to the auth portal (an
// XHR can't complete it), and the server itself can 401. Either way the only
// fix is a full page load so the OAuth flow can run — do it once, with a
// toast, instead of every poll surfacing cryptic errors forever.
let reAuthTriggered = false;
let reAuthCleared = false;
const REAUTH_CLEAR_AFTER_MS = 10_000;
function reAuthOnce() {
  if (reAuthTriggered) return;
  reAuthTriggered = true;
  // Cross-reload cap (2026-08-30 review): the module flag resets on reload,
  // so a persistently broken auth path would loop forever. Two attempts in
  // a minute → stop reloading and tell the user instead.
  let attempts = 0;
  try {
    const raw = sessionStorage.getItem("reauth");
    // Shape-checked, and a corrupt record is overwritten rather than left to
    // disable the cap forever (security review 2026-09-03, both rounds): a
    // negative n would never reach 2, and a parse failure must not skip the
    // write below.
    let rec: { n?: unknown; at?: unknown } = {};
    try {
      const parsed: unknown = raw ? JSON.parse(raw) : null;
      if (typeof parsed === "object" && parsed !== null) rec = parsed;
    } catch {
      rec = {};
    }
    const { n, at } = rec;
    if (
      Number.isInteger(n) &&
      (n as number) >= 0 &&
      typeof at === "number" &&
      Number.isFinite(at) &&
      Date.now() - at < 60_000
    )
      attempts = n as number;
    sessionStorage.setItem(
      "reauth",
      JSON.stringify({ n: attempts + 1, at: Date.now() }),
    );
  } catch {
    /* storage unavailable — fall through with attempts=0 */
  }
  toast(
    attempts >= 2 ? "error" : "info",
    attempts >= 2
      ? "Session expired and sign-in keeps failing — reload manually or check the ingress"
      : "Session expired — signing you back in…",
  );
  if (attempts < 2) setTimeout(() => window.location.reload(), 1200);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    // Don't hang forever if the server is wedged (a stuck VM boot pinning the
    // box); surface a timeout the UI can show instead of a spinner forever.
    signal: init?.signal ?? AbortSignal.timeout(20000),
    ...init,
    // Never follow a redirect. The API issues none; the one the ingress issues
    // for an expired session (→ https://auth.<domain>/oauth2/google/ →
    // accounts.google.com) is cross-origin, so a fetch that FOLLOWED it
    // rejected with a bare "Failed to fetch" before any Response existed and
    // the expiry handling below never ran — every poll showed the raw error
    // until F5 (BUG-001). With "manual" the browser hands back an
    // opaqueredirect Response instead. After the spread so no caller can
    // unset it by accident (security review 2026-09-03).
    redirect: "manual",
  });
  if (res.status === 401 || res.type === "opaqueredirect") {
    reAuthOnce();
    throw new ApiError("session expired — reloading to sign in", res.status || 401);
  }
  // A healthy page clears the re-auth counter, so a GENUINE later expiry gets
  // its automatic reload again (audit 2026-08-30). Deferred and guarded
  // (security review 2026-09-03): clearing on the first ok response let a page
  // that ALSO gets a same-origin 3xx on one endpoint (the mux's path-cleaning
  // 301 from a crafted route param) reset the counter on every load and reload
  // every 1.2 s forever. The counter counts RELOADS, so it clears only once
  // the page has stayed healthy for a while with no re-auth fired.
  if (res.ok && !reAuthCleared) {
    reAuthCleared = true;
    setTimeout(() => {
      if (reAuthTriggered) return;
      try {
        sessionStorage.removeItem("reauth");
      } catch {
        /* storage unavailable */
      }
    }, REAUTH_CLEAR_AFTER_MS);
  }
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
  { kind: "project"; project: string } | { kind: "home"; project: string };

const scopeBase = (s: Scope) =>
  s.kind === "home"
    ? `/api/projects/${s.project}/home`
    : `/api/projects/${s.project}`;

export const api = {
  listProjects: () => request<{ projects: Project[] }>("/api/projects"),
  createProject: (
    name: string,
    opts: {
      github?: "none" | "private" | "public";
      cloneUrl?: string;
    } = {},
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
  projectStats: (id: string) =>
    request<ProjectStats>(`/api/projects/${id}/stats`),
  // Sidebar git badges (v1.14.0): branch + dirty count for every RUNNING
  // project in one batched call.
  gitSummary: () =>
    request<{
      items: {
        projectId: string;
        branch?: string;
        dirty: number;
        ahead?: number;
        behind?: number;
      }[];
    }>("/api/git/summary"),
  // Ops panel (v1.14.0): the microVM's console log tail (entrypoint, dockerd).
  projectLogs: (id: string, tail = 300) =>
    request<{ logs: string }>(`/api/projects/${id}/logs?tail=${tail}`),
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
  /**
   * Create a NEW file, failing with 409 if the name is taken. The dialog's own
   * existing-name check is TOCTOU against the last-loaded listing, so this is
   * the one that actually holds — the kernel settles it with O_EXCL.
   */
  createFile: (scope: Scope, path: string, content: string) =>
    request<void>(
      `${scopeBase(scope)}/file?path=${encodeURIComponent(path)}&create=true`,
      { method: "PUT", body: JSON.stringify({ content }) },
    ),
  // Format content with the language's real CLI formatter in the project VM.
  formatFile: (project: string, path: string, content: string) =>
    request<{ content: string }>(`/api/projects/${project}/format`, {
      method: "POST",
      body: JSON.stringify({ path, content }),
    }),
  // Syntax-check content with the language's real CLI checker in the project
  // VM (gofmt, shellcheck, hadolint, …). `supported:false` means we have no
  // checker for this file type — a normal answer, not an error.
  checkFile: (project: string, path: string, content: string) =>
    request<CheckResult>(`/api/projects/${project}/check`, {
      method: "POST",
      body: JSON.stringify({ path, content }),
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
    // This call cannot go through request(): it must not throw (see below) and
    // it posts FormData, not JSON. It therefore has to repeat request()'s two
    // safety nets by hand, which it previously did not (2026-09-01 audit):
    // an upload with an expired session followed the ingress redirect to the
    // OAuth portal, came back res.ok with an HTML body, and read as SUCCESS
    // that saved nothing; and it could hang forever against a wedged server.
    let res: Response;
    try {
      res = await fetch(`${scopeBase(scope)}/upload?path=${encodeURIComponent(dir)}`, {
        method: "POST",
        body: form,
        signal: AbortSignal.timeout(120000), // uploads are big; generous, not absent
        redirect: "manual", // as in request(): the ingress chain is cross-origin
      });
    } catch (e) {
      const aborted = e instanceof DOMException && e.name === "TimeoutError";
      return {
        saved: [],
        error: new ApiError(aborted ? "upload timed out" : "upload failed", 0),
      };
    }
    if (res.status === 401 || res.type === "opaqueredirect") {
      reAuthOnce();
      return {
        saved: [],
        error: new ApiError("session expired — reloading to sign in", res.status || 401),
      };
    }
    // Unlike the other calls this does NOT throw on failure: the server
    // reports the BASE NAMES it wrote under `dir` on EVERY exit (it base-names
    // each part; a mid-stream failure lists only what landed), and the caller
    // must act on that list before surfacing the error — a pane holding one of
    // those files would otherwise overwrite the upload on Save.
    const body = (await res.json().catch(() => ({}))) as {
      saved?: string[];
      error?: { message?: string };
    };
    return {
      saved: body.saved ?? [],
      error: res.ok
        ? undefined
        : new ApiError(body.error?.message ?? res.statusText, res.status),
    };
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
  // Git side panel (real git client; runs in the guest VM).
  gitStatus: (project: string) =>
    request<GitStatus>(`/api/projects/${encodeURIComponent(project)}/git/status`),
  gitDiff: (
    project: string,
    opts: {
      path?: string;
      staged?: boolean;
      untracked?: boolean;
      /** A single commit's diff; overrides path/staged server-side. */
      commit?: string;
    } = {},
  ) => {
    const q = new URLSearchParams();
    if (opts.path) q.set("path", opts.path);
    if (opts.staged) q.set("staged", "true");
    if (opts.untracked) q.set("untracked", "true");
    if (opts.commit) q.set("commit", opts.commit);
    const qs = q.toString();
    return request<{ state: string; diff: string }>(
      `/api/projects/${encodeURIComponent(project)}/git/diff${qs ? `?${qs}` : ""}`,
    );
  },
  gitStage: (project: string, body: { path?: string; all?: boolean }) =>
    request<{ output: string }>(`/api/projects/${encodeURIComponent(project)}/git/stage`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  gitUnstage: (project: string, body: { path?: string; all?: boolean }) =>
    request<{ output: string }>(`/api/projects/${encodeURIComponent(project)}/git/unstage`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  gitDiscard: (project: string, path: string, untracked: boolean) =>
    request<{ output: string }>(`/api/projects/${encodeURIComponent(project)}/git/discard`, {
      method: "POST",
      body: JSON.stringify({ path, untracked }),
    }),
  gitCommit: (
    project: string,
    message: string,
    opts: { all?: boolean } = {},
  ) =>
    request<{ output: string }>(
      `/api/projects/${encodeURIComponent(project)}/git/commit`,
      {
        method: "POST",
        body: JSON.stringify({ message, all: opts.all ?? false }),
      },
    ),
  gitPush: (project: string) =>
    request<{ output: string }>(`/api/projects/${encodeURIComponent(project)}/git/push`, {
      method: "POST",
    }),

  // v1.22.0 verbs. READS answer 200 with state/repo flags (a missing repo or a
  // stopped VM is a state, not a failed request); MUTATIONS answer 4xx.
  gitFetch: (project: string) =>
    request<{ output: string }>(
      `/api/projects/${encodeURIComponent(project)}/git/fetch`,
      { method: "POST" },
    ),
  gitPull: (project: string) =>
    request<{ output: string }>(
      `/api/projects/${encodeURIComponent(project)}/git/pull`,
      { method: "POST" },
    ),
  gitBranches: (project: string) =>
    request<GitBranches>(
      `/api/projects/${encodeURIComponent(project)}/git/branches`,
    ),
  gitCheckout: (project: string, branch: string, create = false) =>
    request<{ output: string }>(
      `/api/projects/${encodeURIComponent(project)}/git/checkout`,
      { method: "POST", body: JSON.stringify({ branch, create }) },
    ),
  gitLog: (project: string, limit?: number) =>
    request<GitLog>(
      `/api/projects/${encodeURIComponent(project)}/git/log${limit ? `?limit=${limit}` : ""}`,
    ),
  gitStash: (project: string, op: "push" | "pop") =>
    request<{ output: string }>(
      `/api/projects/${encodeURIComponent(project)}/git/stash`,
      { method: "POST", body: JSON.stringify({ op }) },
    ),
  gitStashList: (project: string) =>
    request<{ entries: GitStashEntry[] }>(
      `/api/projects/${encodeURIComponent(project)}/git/stash`,
      { method: "POST", body: JSON.stringify({ op: "list" }) },
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
    request<{ report: CredImportReport }>("/api/settings/credentials/import", {
      method: "POST",
      body: JSON.stringify({ project }),
    }),
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
  credentialContent: (kind: string, id: string) =>
    request<{ content?: string; path?: string; binary?: boolean }>(
      `/api/settings/credentials/${kind}/${encodeURIComponent(id)}/content`,
    ),
  // Project-scoped env variables (SidePanel Env tab) — delivered only to this
  // project's NEW login shells; a project value wins over a same-named global.
  projectEnv: (project: string) =>
    request<{ brokerEnabled: boolean; keys: string[] }>(
      `/api/projects/${project}/env`,
    ),
  putProjectEnv: (project: string, key: string, value: string) =>
    request<void>(`/api/projects/${project}/env`, {
      method: "PUT",
      body: JSON.stringify({ key, value }),
    }),
  projectEnvValue: (project: string, key: string) =>
    request<{ key: string; value: string }>(
      `/api/projects/${project}/env/${encodeURIComponent(key)}`,
    ),
  deleteProjectEnv: (project: string, key: string) =>
    request<void>(`/api/projects/${project}/env/${encodeURIComponent(key)}`, {
      method: "DELETE",
    }),
  setupStatus: () => request<SetupStatus>("/api/setup/status"),
  setupStart: () => request<void>("/api/setup/start", { method: "POST" }),
  setupDone: () => request<void>("/api/setup/done", { method: "POST" }),
  // Seed the broker from the RUNNING setup VM without marking done or tearing
  // it down — the "add one more credential later" flow (Settings → Credentials).
  setupHarvest: () =>
    request<{ report: CredImportReport }>("/api/setup/harvest", {
      method: "POST",
    }),
  // Tear the setup VM down without marking setup done (leave / cancel).
  setupDiscard: () => request<void>("/api/setup", { method: "DELETE" }),
};

/** Shareable link to a port exposed inside a project's microVM. */
export const portUrl = (project: string, term: string, port: number) =>
  `/p/${project}/${term}/port/${port}/`;

/** Terminal WebSocket URL. */
export function termSocketUrl(
  target:
    | { kind: "term"; project: string; id: string }
    | { kind: "setup"; tool?: string },
  cols: number,
  rows: number,
  // boot: this connect carries USER INTENT to run the project (opening or
  // resuming a pane) and may boot a deliberately-stopped VM. Automatic
  // reconnect retries pass false and get close 4002 on a stopped project
  // instead of resurrecting it.
  boot = false,
): string {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const size = `cols=${cols}&rows=${rows}`;
  if (target.kind === "setup") {
    // Optional targeted tool (?tool=gcloud) → styrman-setup runs just that step.
    const tool = target.tool ? `&tool=${encodeURIComponent(target.tool)}` : "";
    return `${proto}://${location.host}/ws/setup?${size}${tool}`;
  }
  const b = boot ? "&boot=1" : "";
  return `${proto}://${location.host}/ws/term/${target.project}/${target.id}?${size}${b}`;
}

/** Terminal protocol close codes (spec: terminal-protocol.md). */
export const CLOSE_EXITED = 4000;
export const CLOSE_REPLACED = 4001;
export const CLOSE_STOPPED = 4002; // project deliberately stopped — never auto-reconnect
