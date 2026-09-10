import { describe, expect, it } from "vitest";
import { abbreviateKey, sshPublicKey } from "./sshKey";

const ED = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb8Q7wq1p3E7c9rZk4x2sY6Hk6s user@host";

describe("sshPublicKey", () => {
  it("returns the ssh tool's public-key detail", () => {
    expect(sshPublicKey([{ name: "ssh", ready: true, details: [ED] }])).toBe(ED);
  });

  it("accepts rsa, ecdsa and security-key types, with or without a comment", () => {
    for (const k of [
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC7",
      "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY= me",
      "sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC1lZDI1NTE5QG9wZW5zc2guY29t yubikey",
    ]) {
      expect(sshPublicKey([{ name: "ssh", ready: true, details: [k] }])).toBe(k);
    }
  });

  // The assertion that matters: the row offers a COPY button for whatever
  // this returns, so anything that is not a public key must yield nothing —
  // a path, a message, and above all a private-key header.
  it("returns nothing for details that are not a public key", () => {
    for (const d of [
      "~/.ssh/id_ed25519",
      "-----BEGIN OPENSSH PRIVATE KEY-----",
      "not set up",
      "ssh-ed25519",
      "",
    ]) {
      expect(sshPublicKey([{ name: "ssh", ready: true, details: [d] }])).toBe("");
    }
  });

  it("returns nothing without the ssh tool, without details, or without tools", () => {
    expect(sshPublicKey(undefined)).toBe("");
    expect(sshPublicKey([])).toBe("");
    expect(sshPublicKey([{ name: "git", ready: true, details: [ED] }])).toBe("");
    expect(sshPublicKey([{ name: "ssh", ready: false }])).toBe("");
  });

  it("ignores other tools' details and surrounding whitespace", () => {
    expect(
      sshPublicKey([
        { name: "git", ready: true, details: ["name = x"] },
        { name: "ssh", ready: true, details: [`  ${ED}\n`] },
      ]),
    ).toBe(ED);
  });
});

describe("abbreviateKey", () => {
  it("keeps the type and comment and shortens the body", () => {
    const a = abbreviateKey(ED);
    expect(a.startsWith("ssh-ed25519 AAAAC3Nz…")).toBe(true);
    expect(a.endsWith("Hk6s user@host")).toBe(true);
    expect(a.length).toBeLessThan(ED.length);
  });

  it("leaves a short or malformed value alone", () => {
    expect(abbreviateKey("ssh-rsa AAAA")).toBe("ssh-rsa AAAA");
    expect(abbreviateKey("garbage")).toBe("garbage");
  });
});
