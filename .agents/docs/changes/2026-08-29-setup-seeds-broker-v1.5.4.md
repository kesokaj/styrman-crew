# 2026-08-29 — Setup completion seeds the credential broker; setup VM auto-closes (v1.5.4)

## Summary

Two changes, same deploy, both live-verified on the operator's real account
(not synthetic). This closes the onboarding gap the 2026-08-28 handoff left
open ("agy real-token check") one layer down: the actual bug wasn't agy, it
was that finishing setup never touched the broker at all. `go test -race
./...` clean; live end-to-end against a real project confirmed zero
re-sign-in for git/gh/claude/ssh. Image + server built and deployed as
**styrman-server 1.5.4** (`docker compose up -d styrman`, in-place restart,
state preserved — not a virgin reset). **Everything below is uncommitted —
prod (1.5.4) is ahead of git as of this writing.**

## 1. Setup completion seeds the credential broker (ADR-034)

**Reported problem:** after finishing first-run `styrman-setup` and creating
a new project, the operator had to sign in to every tool again — "not even
ssh keys exist" in the new project.

**Root cause:** `styrman-setup` runs in the hidden setup VM, which mounts
the user's SHARED base home (`orchestrator.EnsureSetup` → `cfg.UserHome(user)`),
so credentials landed there correctly. But projects get their OWN per-project
homes seeded from the image (ADR-031) and don't inherit the shared home. The
broker's credstore is only seeded by Settings → Credentials → Import from a
project, which reads the PROJECT's guest home — empty on a fresh project. So
importing from a brand-new project stored nothing (two "credential import
complete" log lines against an empty broker store), the broker had nothing
to push, and every new project needed manual sign-in. Nothing bridged shared
home → broker.

**Fix:** `finishSetup` (`POST /api/setup/done`,
`internal/httpapi/setup.go`) now seeds the broker from the setup VM before
tearing it down. The extraction logic previously inlined in
`importCredentials` was pulled into a shared helper,
`(*Server).harvestCredentials(ctx, user, containerID, guest) map[string]string`
(`internal/httpapi/credentials.go`); `guestOut` and `importGuestFile` now
take a `context.Context` instead of reading off `*http.Request`, so they can
be called from `finishSetup`'s own background context. `finishSetup` calls
`harvestCredentials` against `config.SetupContainer(u.Username)`:
best-effort — a disabled broker (`s.creds == nil`) or an empty harvest never
blocks completing setup, only logs a report. Net effect: finishing setup now
pulls every credential the user just signed into straight into the broker,
and every project created afterward pulls them on login — the broker's
"every project signed in automatically" promise now holds from the very
first project, no separate in-project Import step required.

**Verified live (1.5.4):** booted the setup VM via the API against the
operator's real shared home, `POST /api/setup/done` → broker store went from
empty to holding git (github.com), gh (`GH_TOKEN`), claude
(`credentials.json`), ssh (`id_ed25519`). Restarted the existing "test"
project → its login shell pulled from the broker → ssh key, claude creds,
and gh auth all materialized in the new project home; the
`styrman-git-credential` helper wired correctly, `gh api user` returned
`kesokaj`, an authenticated `git ls-remote https://github.com/...` succeeded
(git push works). Zero re-sign-in.

**Known-open — agy did NOT harvest** (carried to `TODO.md`): the
minimal-mode setup VM (`STYRMAN_MINIMAL=1`) doesn't run
`gnome-keyring-daemon`, so `secret-tool search service antigravity` returns
nothing there even though `~/.local/share/keyrings/login.keyring` is present
on disk. agy's keyring tier (ADR-032a) still needs a running/unlocked Secret
Service to extract from — this is the same "agy real-token check" the prior
handoff flagged as the one open verification, now pinned to a specific
cause: the keyring daemon is absent in the setup VM's minimal mode.
gcloud/npm reported "not found" during the live check simply because the
operator hasn't signed into them yet (expected, not a bug). Also: the
`styrman-setup` completion message still tells the operator to manually go
to Settings → Credentials → Import and pick this project — that instruction
is now obsolete (setup auto-seeds; manual Import remains only as a
supplementary path for signing into a tool that was missed at setup time).
Updating that message requires a runtime-image rebuild, so it's deferred
rather than done in this patch deploy.

## 2. Setup VM auto-closes on completion

Previously the setup terminal ran
`bash -lc 'styrman-setup; exec bash -l'`, so after the script finished the
operator was dropped into an idle interactive shell and the VM stayed up
until they clicked "Done". Now `ServeSetup` (`internal/terminal/bridge.go`)
runs just `bash -lc 'styrman-setup'` — when the script exits, the pane ends
and the client gets the existing close-4000 signal (no protocol change).
`web/src/routes/setup.tsx`'s `<TerminalPane>` now passes an `onExited`
handler that calls `api.setupDone()` — which, with change 1 above, seeds
the broker and tears the VM down — then `useNavigate()`s to the dashboard.
Finishing setup now auto-closes the VM and returns to the dashboard with no
manual "Done" click.

## Also in this deploy

The ENOENT first-login fix (v1.5.3, ADR-033) and its review refinements
(`slog.Warn` on directory recreate, project/conversation regression tests)
ship in the same working tree and the same 1.5.4 image — see
`.agents/docs/changes/2026-08-29-first-login-profile-write-enoent-v1.5.3.md`
for that change's own writeup; not duplicated here.

## Files changed

- `internal/httpapi/credentials.go` — `importCredentials` extraction logic
  refactored into shared `harvestCredentials`; `guestOut`/`importGuestFile`
  take `context.Context` instead of `*http.Request`.
- `internal/httpapi/setup.go` — `finishSetup` seeds the broker via
  `harvestCredentials` against the setup container before teardown; adds
  the `internal/config` import.
- `internal/terminal/bridge.go` — `ServeSetup`'s `paneCmd` drops the
  trailing `exec bash -l`, so the pane ends when `styrman-setup` exits.
- `web/src/routes/setup.tsx` — `<TerminalPane onExited>` calls
  `api.setupDone()` then navigates to `/`.
- `web/dist/**` — client/server bundles rebuilt and regenerated as part of
  this change set (tracked in-repo).

## Verification

- Live, against the operator's real account and a real project (not
  synthetic): broker seeded from setup, existing "test" project restarted
  and pulled git/gh/claude/ssh with zero manual sign-in; `gh api user` →
  `kesokaj`; authenticated `git ls-remote` succeeded.
- Setup VM auto-close observed live: finishing `styrman-setup` returned to
  the dashboard with no manual "Done" click and no idle shell left behind.
- Built + deployed as styrman-server **1.5.4** via `docker compose up -d
  styrman` (in-place restart; state preserved, not a virgin reset).
- **Not yet committed** — the working tree (including the rebuilt
  `web/dist/**`) holds all four files above ahead of git as of this
  writing.
