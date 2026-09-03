import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The OAuth ingress answers an expired session on /api/* with a redirect to
// https://auth.<domain>/oauth2/google/ and on to accounts.google.com — a
// cross-origin chain (verified against the live ingress, 2026-09-03). A
// browser fetch() that FOLLOWS that chain rejects with
// TypeError("Failed to fetch") before any Response exists; one told not to
// follow gets an opaqueredirect Response. `browser()` is that browser, with
// the session expired for the paths `expired` says (BUG-001).
const opaqueRedirect = () =>
  ({ type: "opaqueredirect", status: 0, ok: false, redirected: false, url: "", json: async () => ({}) }) as unknown as Response;
const okJSON = (body: unknown) =>
  ({ type: "basic", status: 200, ok: true, redirected: false, url: "", json: async () => body }) as unknown as Response;

function browser(expired: (path: string) => boolean) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (expired(String(input))) {
      if (init?.redirect === "manual") return opaqueRedirect();
      throw new TypeError("Failed to fetch");
    }
    return okJSON({ projects: [] });
  });
}

// sessionStorage that survives a "reload" (module reset) like the real one.
function memoryStorage() {
  const m = new Map<string, string>();
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, String(v)),
    removeItem: (k: string) => void m.delete(k),
  };
}

const KEEPS_FAILING = "Session expired and sign-in keeps failing — reload manually or check the ingress";
const SIGNING_IN = "Session expired — signing you back in…";

describe("request() when the ingress session has expired", () => {
  const reload = vi.fn();

  // A page load: fresh module instances, since reAuthOnce's "only once" flag
  // and the deferred counter clear are module state.
  async function load() {
    vi.resetModules();
    const { api } = await import("./api");
    const { getToasts } = await import("./toast");
    return { api, texts: () => getToasts().map((t) => t.text) };
  }

  beforeEach(() => {
    vi.useFakeTimers();
    reload.mockReset();
    vi.stubGlobal("window", { location: { reload } });
    vi.stubGlobal("sessionStorage", memoryStorage());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
    vi.resetModules();
  });

  it("re-authenticates once instead of surfacing 'Failed to fetch' until F5", async () => {
    vi.stubGlobal("fetch", browser(() => true));
    const { api, texts } = await load();

    await expect(api.listProjects()).rejects.toThrow(/session expired/i);
    expect(texts()).toContain(SIGNING_IN);
    vi.advanceTimersByTime(1200);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("bounds the reload loop when one endpoint redirects while another stays healthy", async () => {
    // A same-origin 3xx that is NOT an expiry (the mux's path-cleaning 301
    // from a crafted route id) next to a poll that succeeds: the counter must
    // count reloads, not be reset by the healthy response on every load
    // (security review 2026-09-03).
    vi.stubGlobal("fetch", browser((p) => p.includes("//")));
    for (const n of [1, 2]) {
      const { api } = await load();
      await api.listProjects();
      await expect(api.getProject("x//y")).rejects.toThrow(/session expired/i);
      vi.advanceTimersByTime(1200);
      expect(reload).toHaveBeenCalledTimes(n);
    }
    const { api, texts } = await load();
    await api.listProjects();
    await expect(api.getProject("x//y")).rejects.toThrow(/session expired/i);
    expect(texts()).toContain(KEEPS_FAILING);
    vi.advanceTimersByTime(20_000); // past the reload delay AND the deferred clear
    expect(reload).toHaveBeenCalledTimes(2);
    expect(sessionStorage.getItem("reauth")).not.toBeNull();
  });

  it("clears the counter only after the page has stayed healthy", async () => {
    sessionStorage.setItem("reauth", JSON.stringify({ n: 1, at: Date.now() }));
    vi.stubGlobal("fetch", browser(() => false));
    const { api } = await load();
    await api.listProjects();
    vi.advanceTimersByTime(9_000);
    expect(sessionStorage.getItem("reauth")).not.toBeNull();
    vi.advanceTimersByTime(1_000);
    expect(sessionStorage.getItem("reauth")).toBeNull();
  });

  it("ignores a malformed counter instead of letting it disable the cap", async () => {
    sessionStorage.setItem("reauth", JSON.stringify({ n: -1000, at: Date.now() }));
    vi.stubGlobal("fetch", browser(() => true));
    const { api } = await load();
    await expect(api.listProjects()).rejects.toThrow(/session expired/i);
    expect(JSON.parse(sessionStorage.getItem("reauth") ?? "{}").n).toBe(1);
  });

  it("overwrites a corrupt counter instead of leaving the cap off forever", async () => {
    sessionStorage.setItem("reauth", "not json");
    vi.stubGlobal("fetch", browser(() => true));
    const { api } = await load();
    await expect(api.listProjects()).rejects.toThrow(/session expired/i);
    expect(JSON.parse(sessionStorage.getItem("reauth") ?? "{}").n).toBe(1);
  });

  it("uploadFiles re-authenticates too, instead of a bare 'upload failed'", async () => {
    vi.stubGlobal("fetch", browser(() => true));
    const { api, texts } = await load();
    const r = await api.uploadFiles({ kind: "project", project: "p" }, "dir", []);
    expect(r.error?.message).toMatch(/session expired/i);
    expect(texts()).toContain(SIGNING_IN);
    vi.advanceTimersByTime(1200);
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
