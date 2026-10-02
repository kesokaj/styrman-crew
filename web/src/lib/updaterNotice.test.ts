import { describe, expect, it } from "vitest";
import { updaterNotice } from "./updaterNotice";

// The Settings page's sentence for the auto-updater's status (ADR-081).
describe("updaterNotice", () => {
  it("says nothing without a status, and nothing for a healthy one", () => {
    expect(updaterNotice(undefined)).toBeNull();
    expect(updaterNotice({ state: "ok", code: "ok", checkedAt: "2026-10-02T17:58:00Z" })).toBeNull();
  });

  it("names the rejected token, since when, and what the host stays on", () => {
    expect(
      updaterNotice({
        state: "failing",
        code: "fetch-auth",
        since: "2026-09-22T09:14:02Z",
        checkedAt: "2026-10-02T17:16:19Z",
        deployed: "v1.38.0",
      }),
    ).toEqual({
      tone: "danger",
      text: "Auto-update has been failing since 2026-09-22T09:14:02Z: GitHub rejected this host's token (/etc/styrman/gh-token). This host stays on v1.38.0.",
    });
  });

  it("has a sentence for every failing code", () => {
    const text = (code: string, latest?: string) =>
      updaterNotice({ state: "failing", code, since: "2026-10-01T00:00:00Z", latest })?.text;
    expect(text("fetch")).toContain("cannot reach the repository");
    expect(text("no-tags")).toContain("no release tag");
    expect(text("signature", "v1.41.0")).toContain("v1.41.0 is not signed by a key this host trusts");
    expect(text("rolled-back", "v1.41.0")).toContain("v1.41.0 did not come up and was rolled back");
    expect(text("deploy-failed", "v1.41.0")).toContain("v1.41.0 did not come up and could not be rolled back");
    expect(text("latched", "v1.41.0")).toContain("v1.41.0 failed earlier and is not retried");
    expect(text("error")).toContain("stopped with an error");
  });

  it("an empty latest reads as the newest release, an empty deployed drops its sentence", () => {
    const n = updaterNotice({ state: "failing", code: "signature", since: "2026-10-01T00:00:00Z" });
    expect(n?.text).toBe(
      "Auto-update has been failing since 2026-10-01T00:00:00Z: the newest release is not signed by a key this host trusts.",
    );
  });

  it("a failed rollback never claims what the host runs", () => {
    // The updater's record still names the previous tag after a failed
    // rollback; the host runs whatever the failed deploy left.
    const n = updaterNotice({
      state: "failing",
      code: "deploy-failed",
      since: "2026-10-01T00:00:00Z",
      deployed: "v1.40.0",
      latest: "v1.41.0",
    });
    expect(n?.text).toBe(
      "Auto-update has been failing since 2026-10-01T00:00:00Z: v1.41.0 did not come up and could not be rolled back.",
    );
    expect(n?.text).not.toContain("stays on");
  });

  it("a code it does not know gets the generic failing sentence", () => {
    const n = updaterNotice({ state: "failing", code: "<b>x</b>", since: "2026-10-01T00:00:00Z" });
    expect(n?.tone).toBe("danger");
    expect(n?.text).toContain("stopped with an error");
    expect(n?.text).not.toContain("<b>");
  });

  it("stale and unknown are warnings of their own", () => {
    expect(updaterNotice({ state: "stale", code: "ok", checkedAt: "2026-10-01T08:00:00Z" })).toEqual({
      tone: "warn",
      text: "Auto-update has not run since 2026-10-01T08:00:00Z.",
    });
    expect(updaterNotice({ state: "unknown" })).toEqual({
      tone: "warn",
      text: "Auto-update status cannot be read.",
    });
  });
});
