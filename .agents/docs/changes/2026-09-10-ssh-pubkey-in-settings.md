# SSH public key in Settings → Credentials (date: 2026-09-10)

## Summary

Settings → Credentials now displays the user's SSH public key (abbreviated with full key in hover tooltip) alongside a copy-to-clipboard button, making it easy to retrieve the key when needed. The data was already flowing from `GET /api/settings/tools` (the ssh tool's `details` field, read symlink-refusing from the setup VM's home); the UI now extracts and displays it.

Frontend-only change; no credential handling or storage modified. The button copies the FULL public key to clipboard with a toast confirmation or error message.

## Type

feature

## Files changed

- `web/src/lib/sshKey.ts` (new) — `sshPublicKey()` extracts the OpenSSH public key line from tool details; `abbreviateKey()` truncates for display
- `web/src/lib/sshKey.test.ts` (new) — 7 vitest tests for key extraction and abbreviation edge cases
- `web/src/routes/settings.tsx` — ssh row detail now shows abbreviated key with copy button and hover title
- `web/dist/client` — rebuilt SPA

## Verification

Manual test: open Settings → Credentials, hover over the abbreviated ssh key to confirm full key appears in title, click copy button and confirm toast and clipboard contents. Vitest: 7 new tests; test suite runs green (127 → 134 tests). Operator walk-through confirmed on the live instance 2026-09-10.

## Requirements

n/a — UI polish, no new requirements.

## Notes

Security review was dispatched and confirmed the surface poses no new risk: the public key is non-secret (it is literally the user's `.ssh/id_ed25519.pub` file, readable via browser developer tools by inspecting the API response, and published to GitHub by the user). No credentials, private keys, or tokens are exposed in the UI.
