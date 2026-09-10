import { describe, expect, it } from "vitest";
import { projectBadge } from "./projectBadge";

describe("projectBadge", () => {
  it("renders nothing for a plain running or stopped project", () => {
    expect(projectBadge({ state: "running" })).toBeNull();
    expect(projectBadge({ state: "stopped" })).toBeNull();
    expect(projectBadge({ state: "absent" })).toBeNull();
  });

  it("shows a failed delete with the error as the title", () => {
    const b = projectBadge({ state: "stopped", deleteError: "docker rm: boom" });
    expect(b?.text).toBe("delete failed");
    expect(b?.title).toBe("docker rm: boom");
    expect(b?.className).toContain("danger");
  });

  it("shows a failed repo setup", () => {
    const b = projectBadge({ state: "running", provisionError: "clone failed" });
    expect(b?.text).toBe("repo setup failed");
    expect(b?.title).toBe("clone failed");
  });

  it("shows provisioning while the clone runs, even with no container yet", () => {
    expect(projectBadge({ state: "provisioning" })?.text).toBe("provisioning…");
  });

  // Precedence: deleting hides every error; a failed delete outranks a failed
  // repo setup; an error outranks the provisioning transient.
  it("keeps the precedence deleting > deleteError > provisionError > provisioning", () => {
    expect(projectBadge({ state: "deleting", deleteError: "x", provisionError: "y" })?.text).toBe("deleting…");
    expect(projectBadge({ state: "running", deleteError: "x", provisionError: "y" })?.text).toBe("delete failed");
    expect(projectBadge({ state: "provisioning", provisionError: "y" })?.text).toBe("repo setup failed");
  });
});
