// The user's ssh PUBLIC key as the settings page shows it (operator request,
// 2026-09-10: "I like the ssh.pub key to be visible in the settings menu").
// Kept free of React so vitest covers the parsing (sshKey.test.ts).
//
// The server already echoes the key: GET /api/settings/tools returns the
// "ssh" tool with its Details = [the .pub's first line], read from the
// user's base home without following a symlink and only if it has a
// public-key shape (internal/httpapi/setup.go) — the file is the user's own,
// so the CONTENT check is the guard, not the file's kind. The same shape
// check is repeated here so the copy button never offers anything else.

import type { ToolInfo } from "./api";

/**
 * Public-key line shapes OpenSSH writes: `<type> <base64> [comment]`, on ONE
 * line. Spaces and tabs only between the fields — `\s` would admit a newline,
 * and a second line hidden after a comment-less key would ride along onto
 * the clipboard and into an authorized_keys file (security review
 * 2026-09-10). The comment may not contain a line break either.
 */
const PUBLIC_KEY =
  /^(ssh-(ed25519|rsa|dss)|ecdsa-sha2-[^\s]+|sk-[^\s]+)[ \t]+[A-Za-z0-9+/=]+([ \t]+[^\r\n]*[^\s])?$/;

/** Longer than any real key line (a 16 kbit RSA key is under 3000). */
const MAX_KEY_LEN = 4096;

/**
 * sshPublicKey — the user's public key, or "" when the server has not
 * reported one. Only a line that LOOKS like a public key is returned: a
 * detail that is anything else (a path, an error, a private-key header) is
 * not something to offer for copying into an authorized_keys file.
 */
export function sshPublicKey(tools: ToolInfo[] | undefined): string {
  const ssh = tools?.find((t) => t.name === "ssh");
  for (const d of ssh?.details ?? []) {
    const line = d.trim();
    // Any control or line/paragraph separator character is a reject, not
    // just CR/LF: they reach the title attribute and the clipboard verbatim.
    if (line.length > MAX_KEY_LEN || /[\p{Cc}\u2028\u2029]/u.test(line)) continue;
    if (PUBLIC_KEY.test(line)) return line;
  }
  return "";
}

/**
 * abbreviateKey — `ssh-ed25519 AAAAC3…Hk6s user@host`: the type, the first
 * and last few base64 characters, and the comment, so the row stays one line
 * while the type and the identity are still readable. The full key goes on
 * the clipboard, never this.
 */
export function abbreviateKey(key: string): string {
  const [type, body, ...rest] = key.split(/\s+/);
  if (!type || !body) return key;
  const shortBody =
    body.length > 16 ? `${body.slice(0, 8)}…${body.slice(-6)}` : body;
  return [type, shortBody, ...rest].join(" ");
}
