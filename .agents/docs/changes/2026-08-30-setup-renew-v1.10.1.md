# Setup: keep/renew prompts for existing credentials + no-reconnect hardening (v1.10.1)

Date: 2026-08-30. Follow-up to v1.10.0 from operator feedback: "if there
already is an identity I need to be able to skip… and a way to renew it and
skip to the next."

## Keep / renew (`image/configs/styrman-setup`)

Every step was idempotent but not renewable — an existing credential printed
✔ and moved on, with no way to re-authenticate. Now each step that finds an
existing credential shows it and asks `renew? [y/N]`: Enter keeps it and
skips to the next step, `y` re-runs the sign-in.

- git: `change name/email?` → re-prompts name + email.
- ssh: `replace it with a freshly generated key?` → generates a new
  `id_ed25519`, keeps the old pair as `id_ed25519.replaced` (the broker only
  harvests `id_ed25519` itself), warns that registered copies (GitHub) must
  be replaced.
- gh / gcloud (account + ADC separately) / claude / agy: `sign in again
  (renew / switch account)?` → re-runs the login command.
- The `ask_optional` helper was renamed `ask` (same semantics: default no,
  non-interactive always no) since it now also gates renewals.

**Probe stdin guard:** all status probes (`gh auth status`, `gcloud auth
list`, `gcloud auth application-default print-access-token`, `claude auth
status`) now run with `</dev/null`. A pty test showed `claude auth status`
swallowing typed-ahead input buffered for later prompts, stranding the flow
at its own prompt — a probe must never eat the user's answers.

## Setup pane never auto-reconnects after first open (`web/src/components/TerminalPane.tsx`)

The v1.10.0 SIGINT trap made Ctrl+C a clean exit, but the restart hole
wasn't fully closed: the bridge answers close 4000 only when dockerd
registers the exec's exit within a 1 s poll (`ExitCodeWait`); losing that
race yields a TRANSIENT close, and the client's reconnect (`tmux
new-session -A`) would recreate the ended session — restarting the script
despite the trap. Now a setup pane treats ANY post-open close as exited
(→ `onExited`, which is status-aware); only pre-open failures retry (VM
still booting). Project terminals keep normal transient-reconnect behavior
(their panes idle at a shell; reattach is always safe).

## Files changed

- `image/configs/styrman-setup` — keep/renew prompts, `ask` helper, probe
  `</dev/null` guards, +x mode bit recorded in git
- `web/src/components/TerminalPane.tsx` — `everOpened` no-reconnect guard
  for setup targets (4001 "replaced" still takes precedence)
- Specs: `runtime-image.md`, `terminal-protocol.md`, `guide.md`
- `web/dist/` — rebuilt embedded bundle

## Verification

- `bash -n` + full pty-driven tests in a throwaway `styrman-runtime:dev`
  container: non-interactive run with existing creds ✔-skips everything and
  exits 0; interactive renew updates git identity (Old → New) while Enter
  keeps the ssh key; targeted `styrman-setup ssh` renew generates a new key
  + `.replaced` backup with the git-email comment; targeted `styrman-setup
  git` shows the renew prompt and Enter keeps.
- web: `tsc --noEmit`, oxlint, `npm run build` all clean.
- Deployed: runtime + server rebuilt as 1.10.1, server healthy (log shows
  version 1.10.1), project VMs untouched, setup VM recycled, crew 302 →
  OAuth gate.
