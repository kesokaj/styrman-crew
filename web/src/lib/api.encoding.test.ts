import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, termSocketUrl } from "./api";

// Every value the client interpolates into a path is percent-encoded by the
// `u` tag in api.ts. The weakness it closes is path RETARGETING: browsers
// resolve `..` and `?` BEFORE sending, so an id carrying either can aim a
// clicked action at a different /api/projects/... path of the same user.
// csrfGuard passes it (genuinely same-origin) and picks its body-size class
// by path suffix (security review 2026-09-03, round 2).
//
// This file is the assertion the reviewers asked for, and it is written
// against the CALL SITES rather than against `u` itself, because the failure
// mode is not "the tag is wrong" — it is "a new call site forgot to use it".
// A test of the tag alone would stay green through exactly that regression.

// Two shapes of hostile value: one that climbs out of the intended path, and
// one that ends it early and appends a query the caller never wrote.
const CLIMB = "../../../api/settings/credentials";
const QUERY = "x?wipe=true";
const ORIGIN = "https://crew.styrman.xyz";

describe("api paths encode every interpolated value", () => {
  const seen: string[] = [];
  // noUncheckedIndexedAccess is on: read the captured URL through a check so
  // a method that issued NO request fails loudly instead of typing as
  // undefined and quietly skipping the assertion.
  const sent = (): string => {
    const first = seen[0];
    if (first === undefined) throw new Error("no request was issued");
    return first;
  };

  beforeEach(() => {
    seen.length = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        seen.push(String(input));
        return {
          type: "basic", status: 200, ok: true, redirected: false, url: "",
          json: async () => ({}),
        } as unknown as Response;
      }),
    );
  });

  afterEach(() => vi.unstubAllGlobals());

  // Each entry calls one method with a hostile value in every position that
  // reaches the path, and names the path prefix the request must not leave.
  const calls: Array<[string, () => unknown, string]> = [
    ["getProject", () => api.getProject(CLIMB), "/api/projects/"],
    ["projectStats", () => api.projectStats(CLIMB), "/api/projects/"],
    ["projectLogs", () => api.projectLogs(CLIMB, 10), "/api/projects/"],
    ["renameProject", () => api.renameProject(CLIMB, "n"), "/api/projects/"],
    ["deleteProject", () => api.deleteProject(QUERY, false), "/api/projects/"],
    ["startProject", () => api.startProject(CLIMB), "/api/projects/"],
    ["stopProject", () => api.stopProject(CLIMB), "/api/projects/"],
    ["projectNetwork", () => api.projectNetwork(CLIMB), "/api/projects/"],
    ["setProjectLan", () => api.setProjectLan(CLIMB, true), "/api/projects/"],
    ["listTerminals", () => api.listTerminals(CLIMB), "/api/projects/"],
    ["createTerminal", () => api.createTerminal(CLIMB, "t"), "/api/projects/"],
    ["updateTerminal", () => api.updateTerminal("p", CLIMB, { title: "t" }), "/api/projects/p/terminals/"],
    ["deleteTerminal", () => api.deleteTerminal("p", CLIMB), "/api/projects/p/terminals/"],
    ["deleteAllTerminals", () => api.deleteAllTerminals(CLIMB), "/api/projects/"],
    ["pauseTerminal", () => api.pauseTerminal("p", CLIMB), "/api/projects/p/terminals/"],
    ["listPorts", () => api.listPorts(CLIMB), "/api/projects/"],
    ["getPortShares", () => api.getPortShares(CLIMB, 8080), "/api/projects/"],
    ["setPortShares", () => api.setPortShares(CLIMB, 8080, []), "/api/projects/"],
    ["listLinks", () => api.listLinks(CLIMB), "/api/projects/"],
    ["createLink", () => api.createLink(CLIMB, 8080, "1h"), "/api/projects/"],
    // The link TOKEN is the second value the 2026-09-03 review named.
    ["revokeLink", () => api.revokeLink("p", CLIMB), "/api/projects/p/links/"],
    ["listHistory", () => api.listHistory(CLIMB), "/api/projects/"],
    ["searchHistory", () => api.searchHistory(CLIMB, "q"), "/api/projects/"],
    ["readHistory", () => api.readHistory("p", CLIMB, 10), "/api/projects/p/history/"],
    ["gitStatus", () => api.gitStatus(CLIMB), "/api/projects/"],
    ["gitDiff", () => api.gitDiff(CLIMB, {}), "/api/projects/"],
    ["gitLog", () => api.gitLog(CLIMB, 5), "/api/projects/"],
    ["gitStage", () => api.gitStage(CLIMB, { all: true }), "/api/projects/"],
    ["gitCheckout", () => api.gitCheckout(CLIMB, "b"), "/api/projects/"],
    ["formatFile", () => api.formatFile(CLIMB, "f.go", ""), "/api/projects/"],
    ["checkFile", () => api.checkFile(CLIMB, "f.go", ""), "/api/projects/"],
    ["projectEnv", () => api.projectEnv(CLIMB), "/api/projects/"],
    ["putProjectEnv", () => api.putProjectEnv(CLIMB, "K", "v"), "/api/projects/"],
    ["projectEnvValue", () => api.projectEnvValue("p", CLIMB), "/api/projects/p/env/"],
    ["deleteProjectEnv", () => api.deleteProjectEnv("p", CLIMB), "/api/projects/p/env/"],
    ["deleteCredential", () => api.deleteCredential("env", CLIMB), "/api/settings/credentials/env/"],
    ["credentialContent", () => api.credentialContent("env", CLIMB), "/api/settings/credentials/env/"],
    // scopeBase feeds every file-explorer path; both scopes carry a project.
    ["listFiles project", () => api.listFiles({ kind: "project", project: CLIMB }, "/"), "/api/projects/"],
    ["listFiles home", () => api.listFiles({ kind: "home", project: CLIMB }, "/"), "/api/projects/"],
    ["readFile", () => api.readFile({ kind: "project", project: CLIMB }, "/a"), "/api/projects/"],
    ["saveFile", () => api.saveFile({ kind: "project", project: CLIMB }, "/a", ""), "/api/projects/"],
    ["createFile", () => api.createFile({ kind: "project", project: CLIMB }, "/a", ""), "/api/projects/"],
    ["deleteFile", () => api.deleteFile({ kind: "project", project: CLIMB }, "/a"), "/api/projects/"],
    ["renameFile", () => api.renameFile({ kind: "project", project: CLIMB }, "/a", "b"), "/api/projects/"],
    ["makeDir", () => api.makeDir({ kind: "project", project: CLIMB }, "/a"), "/api/projects/"],
  ];

  for (const [name, call, prefix] of calls) {
    it(`${name} cannot be aimed out of ${prefix}`, async () => {
      await call();
      expect(seen, `${name} issued no request`).toHaveLength(1);
      // Resolve the way the browser does before it puts the path on the wire.
      // Surviving this is the whole claim: `..` and `?` in the value must not
      // move the request, so the RESOLVED path still starts where it should.
      const url = new URL(sent(), ORIGIN);
      expect(url.pathname, `${name} -> ${sent()}`).toMatch(new RegExp("^" + prefix));
      // The test on the SEGMENT, not the substring: encodeURIComponent escapes
      // the separator, not the dots, so the resolved path still READS
      // "..%2F..%2F.." — as one opaque segment the resolver never walks. A
      // substring check would fail here on a correct result, which is how the
      // first draft of this file failed 46 of its own cases.
      expect(url.pathname.split("/"), `${name} -> ${sent()}`).not.toContain("..");
    });
  }

  it("deleteProject's own query survives, and the id cannot forge one", async () => {
    // `u` must encode the value's `?` while leaving the query the caller
    // appended outside the tag intact — the two are not interchangeable, and
    // a tag that swallowed the real `?` would silently drop ?wipe=true.
    await api.deleteProject(QUERY, true);
    const url = new URL(sent(), ORIGIN);
    expect(url.searchParams.get("wipe")).toBe("true");
    expect(url.pathname).toBe(`/api/projects/${encodeURIComponent(QUERY)}`);
  });

  it("keeps caller-built query strings working", async () => {
    // The `?` and `=` of a real query must stay literal. `u` encodes values
    // only; every query in api.ts is concatenated outside the tag.
    await api.searchHistory("p", "needle words");
    expect(new URL(sent(), ORIGIN).searchParams.get("q")).toBe("needle words");
    seen.length = 0;
    await api.projectLogs("p", 300);
    expect(new URL(sent(), ORIGIN).searchParams.get("tail")).toBe("300");
    seen.length = 0;
    await api.gitLog("p", 5);
    expect(new URL(sent(), ORIGIN).searchParams.get("limit")).toBe("5");
  });

  it("downloadUrl is encoded even though it never reaches fetch()", () => {
    // It becomes an href, so the browser resolves it exactly the same way.
    const href = api.downloadUrl({ kind: "project", project: CLIMB }, "/a.txt");
    expect(new URL(href, ORIGIN).pathname).toMatch(/^\/api\/projects\//);
    expect(new URL(href, ORIGIN).pathname.split("/")).not.toContain("..");
  });
});

describe("termSocketUrl", () => {
  beforeEach(() => {
    vi.stubGlobal("location", { protocol: "https:", host: "crew.styrman.xyz" });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("encodes the project and the terminal id", () => {
    // The terminal `id` is the first value the 2026-09-03 review named, and
    // the /ws path was still building it raw after the /api paths were fixed.
    const url = new URL(termSocketUrl(
      { kind: "term", project: CLIMB, id: CLIMB }, 80, 24,
    ));
    expect(url.pathname).toMatch(/^\/ws\/term\//);
    expect(url.pathname.split("/")).not.toContain("..");
    // Both values survive as exactly one segment each.
    expect(url.pathname.split("/")).toHaveLength(5);
    expect(url.searchParams.get("cols")).toBe("80");
    expect(url.searchParams.get("rows")).toBe("24");
  });

  it("still carries boot and the setup tool", () => {
    const boot = new URL(termSocketUrl({ kind: "term", project: "p", id: "t" }, 80, 24, true));
    expect(boot.pathname).toBe("/ws/term/p/t");
    expect(boot.searchParams.get("boot")).toBe("1");
    const setup = new URL(termSocketUrl({ kind: "setup", tool: "gcloud" }, 80, 24));
    expect(setup.pathname).toBe("/ws/setup");
    expect(setup.searchParams.get("tool")).toBe("gcloud");
  });
});
