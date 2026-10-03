---
owner: main
date: 2026-10-03
---
# dave's updater works again; the signing keys' protection recorded   (2026-10-03)

The operator asked: "Why can't you do 2 and 4 you have my permission". Those
were Next Steps 2 (replace dave's updater token, BUG-084) and 4 (record how the
release keys are protected) in `handoffs/2026-10-03-*-dave-on-v1.41.0.md`.

## The updater token (BUG-084, fixed)
A fine-grained token can only be created in GitHub's web settings. Asked which
token to install, the operator chose "Use my gh login token now".
- The rejected token was moved aside to
  `/etc/styrman/gh-token.rejected-2026-10-03`, not deleted. It was 94 bytes,
  written 2026-09-02, the length of a fine-grained token, and an expiry fits
  the timing.
- The `gh` login token was installed as `/etc/styrman/gh-token`, root:root
  0600, through a pipe (`umask 077`, then `chown`/`chmod`, then `mv`). It was
  never on a command line or in a log.
- **Known deviation:** the docs ask for a fine-grained, read-only token. This
  one can write to every repository of the operator's and stops working if
  `gh` is logged out or the token refreshed. Replacing it is the operator's
  follow-up.

Verified, in order:
1. `styrman-update --check` (the installed 1.38.0 updater) → "v1.41.0
   signature OK (allowed signer)", "update AVAILABLE: v1.38.0 → v1.41.0".
2. `sudo deploy/styrman-ctl update` → "OK — v1.41.0 is live" and "updater
   self-installed from the verified tree". Every image rebuilt from cache
   identical to what the break-glass deploy had built, so both containers kept
   running. `styrman-ctl status`: verdict OK, `deployed v1.41.0`, the clone at
   `c2b1f61`.
3. The new unit carries
   `Environment=STYRMAN_UPDATE_STATUS_FILE=/etc/styrman/status/updater.json`.
   One more tick (`systemctl start styrman-update.service`) → `Result=success`,
   and `/etc/styrman/status/updater.json` (root 0644) reads
   `{"result":"ok","code":"ok","checkedAt":"2026-10-03T01:44:16Z","since":"","deployed":"v1.41.0","latest":"v1.41.0"}`.
   The same line is read inside `styrman-server` through the read-only mount.

This is the first real root tick to write the status file. The release gate
listed that as not covered: only the gated hooks had run the writer before.
Still not seen: the Settings notice for a failing status on a live host (an
`ok` status shows nothing, by design).

## The signing keys (ADR-082)
Appended to ADR-082:
- The original signer is a plain file on dave, mode 600, with no passphrase
  and no agent. It was tested by opening it with an empty passphrase, which
  printed no key material. Anyone with the operator's account on dave can sign
  a tag that dave and the GCP VM deploy as root. Adding a passphrase would make
  every tag cut on dave interactive; that is the operator's choice, and nothing
  was changed.
- The Mac key's protection was not inspected: the Mac is not reachable from
  dave.

## Learnings for the local host
- dave's `/etc/styrman` holds `gh-token.rejected-2026-10-03`. Delete it once
  the new token has proved itself, or keep it as the record of what expired.
