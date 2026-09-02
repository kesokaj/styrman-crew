import { describe, expect, it } from "vitest";
import { cloneURLError, projectNameError, projectNameFromRepoURL } from "./api";

// These three are pure, security- or UX-load-bearing, and were the frontend's
// entire test coverage story: none (8,254 LOC, 2026-09-01 audit).

describe("projectNameFromRepoURL", () => {
  it("names a cloned project after the repo", () => {
    // The operator expectation this exists for: "use the same name of the repo
    // who is cloned".
    expect(projectNameFromRepoURL("git@github.com:kesokaj/styrman-harness.git"))
      .toBe("styrman-harness");
    expect(projectNameFromRepoURL("https://github.com/kesokaj/styrman-harness.git"))
      .toBe("styrman-harness");
    expect(projectNameFromRepoURL("https://github.com/kesokaj/styrman-harness"))
      .toBe("styrman-harness");
    expect(projectNameFromRepoURL("ssh://git@host:22/o/My_Repo.git")).toBe("my-repo");
  });

  it("slugs to the server's project-name rules", () => {
    // Whatever it returns must pass projectNameError, or the dialog offers a
    // name the server will reject.
    for (const url of [
      "https://github.com/o/My_Repo.git",
      "https://github.com/o/repo.with.dots.git",
      "https://github.com/o/UPPER.git",
      "git@h:o/123numeric.git",
      "https://github.com/o/trailing---.git",
      "https://github.com/o/a-very-long-repository-name-that-exceeds-the-limit.git",
    ]) {
      const name = projectNameFromRepoURL(url);
      if (name !== "") expect(projectNameError(name), `${url} -> ${name}`).toBeUndefined();
    }
  });

  it("returns empty when nothing usable is left, so the typed name stands", () => {
    expect(projectNameFromRepoURL("")).toBe("");
    expect(projectNameFromRepoURL("https://github.com/o/---.git")).toBe("");
    expect(projectNameFromRepoURL("https://github.com/o/123.git")).toBe("");
  });

  it("ignores trailing slashes", () => {
    expect(projectNameFromRepoURL("https://github.com/o/repo/")).toBe("repo");
  });
});

describe("cloneURLError", () => {
  it("refuses a URL carrying a password, mirroring the server", () => {
    // The server refuses these with 400 (cloneURLHasSecret / ADR-059); the
    // client must catch it first so the user is not told after the fact.
    for (const url of [
      "https://user:ghp_token@github.com/o/r.git",
      "https://x-access-token:tok@github.com/o/r.git",
      "ssh://git:key@host/o/r",
    ]) {
      expect(cloneURLError(url), url).toBeTruthy();
    }
  });

  it("allows a bare username and ordinary URLs", () => {
    // ssh://git@host is the standard SSH form; "git" is a username, not a
    // secret. An "@" in the PATH is not userinfo either.
    for (const url of [
      "",
      "https://github.com/o/r.git",
      "git@github.com:o/r.git",
      "ssh://git@host/o/r",
      "https://user@github.com/o/r.git",
      "https://github.com/o/r@v1.git",
    ]) {
      expect(cloneURLError(url), url).toBeUndefined();
    }
  });
});

describe("projectNameError", () => {
  it("accepts what the server accepts", () => {
    for (const n of ["a", "my-project", "p1", "a".repeat(31)]) {
      expect(projectNameError(n), n).toBeUndefined();
    }
  });
  it("rejects what the server rejects", () => {
    for (const n of ["", "1abc", "-abc", "Abc", "a_b", "a".repeat(32), "a b"]) {
      expect(projectNameError(n), n).toBeTruthy();
    }
  });
});
