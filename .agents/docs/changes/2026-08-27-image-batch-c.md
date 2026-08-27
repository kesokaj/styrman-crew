# 2026-08-27 — Image / entrypoint reliability (Batch C)

Third remediation batch — the runtime image + entrypoint findings.

- **I9 — graceful shutdown.** The entrypoint no longer `exec sleep infinity`s
  (which made a trap impossible); it installs a SIGTERM/SIGINT trap that stops
  dockerd cleanly (nested containers get their grace period; overlay2 isn't
  torn down mid-write) and unmounts the loop-mounted ext4 so its journal is
  clean next boot. Verified the trap runs on signal. Kata's `docker stop`
  powers the VM off WITHOUT delivering SIGTERM to the in-guest init, so the
  orchestrator's `Stop` now signals PID 1 inside the guest first (`kill -TERM
  1`), waits for a clean exit, then stops — verified end to end.
- **I10 — ~/.tmux.conf never left root-owned.** A failed seed `cp` used to let
  a later `echo >>` create the file as root (losing `mouse off` and the
  login-shell default → broke keyring/agy, unfixable without sudo). Now the
  source-include + mouse migration run only if the file exists and as the
  user, and any pre-existing root-owned config is chowned back to the guest.
- **I11 — reserved usernames rejected.** `root`/`daemon`/… would skip
  `useradd` and run the guest at the wrong uid on a home it can't write. The
  entrypoint now asserts the account's uid matches and FATAL-exits otherwise;
  the server (`setGuestUser`) and the setup UI both reject a reserved list.
- **I12 — vfs fallback surfaced.** The loop-mount fallback to the slow `vfs`
  driver (a different data-root, so prior overlay2 images vanish — looks like
  data loss) now logs loudly and writes a `storage-warning.txt` marker; the
  image is `fsck.ext4 -p`'d before mounting.
- **N30** — free-space check before allocating the sparse docker image.
- **N32** — `styrman-setup` git-identity step loops until non-empty and bails
  under a non-interactive exec (no more empty identity saved as "success");
  the SSH key-comment parse handles multi-word/empty comments.
- **N33** — the transcript log dir creation is guarded (warns instead of
  silently dropping history).

Tests: reserved-name unit test; go test/vet green; integration 56/56, e2e
32/32; runtime image rebuilt; graceful-shutdown verified live; prod
redeployed.
