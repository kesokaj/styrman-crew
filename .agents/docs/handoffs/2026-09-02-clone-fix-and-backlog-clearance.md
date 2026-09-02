# Handoff — 2026-09-02 — clone provisioning fix, then the whole backlog

Session ID: 62decb39-3f5f-4fe7-96c8-e69a9afb3581
Project: /home/kesokaj/workspace/styrman-crew
Branch: main

## Context

Operator reported that "Clone a repo" in the New Project dialog did not work. Root-causing
that turned into four releases: the clone fix, a docs audit, the entire 🟡/🟢 backlog, and
one ingress hardening item shipped on its own. **v1.19.1 is live in production.**

## Completed

Live at crew.styrman.xyz, all four tags signed and pushed:

- **v1.18.1** — the clone fix. `EnsureSelfAttached` (which joins the server to the project's
  bridge, the ONLY route to the credential broker) ran AFTER provisioning, so every clone and
  every `gh repo create` ran with no ssh key, no git credential and no GH_TOKEN, then silently
  degraded to an empty repo. Now: attach → `waitForBroker` → provision in a login shell.
  Extracted to `internal/httpapi/provision.go` (`provisionSpec`/`provisionRepo`/`bootFailed`,
  attach+run injected so the ORDER is unit-tested). Plus `provisionError` on the project API,
  a "repo setup failed" sidebar badge, clone auto-naming from the repo URL, and the microVM
  hostname losing its `styrman-` prefix.
- **v1.18.2** — clone URLs embedding a password refused at the boundary (ADR-059); docs audit
  remediation, including a `versioning.md` release procedure that no longer takes prod down.
- **v1.19.0** — 23 backlog items. Cross-tenant deletion via symlink in `sweepMaterializedFile`
  (now `os.Root`-confined `files.Remove`); `STYRMAN_RUNTIME` allowlisted (ADR-060); egress
  deny-list gained 100.64.0.0/10 + 198.18.0.0/15; `usernameFromEmail` duplicate deleted;
  non-wipe delete no longer strands ssh keys; `vulncheck.sh` made runnable on a clean checkout;
  helm REMOVED and OpenTofu/yq/caddy pinned; unsaved editor edits survive a terminal switch;
  `uploadFiles` got 401+timeout handling; vitest + 11 frontend tests (was zero across 8,254
  LOC); O_EXCL create-only endpoint; dead code removed (`LastActivity`, `Project.Agent`,
  `Project.GitInit` — hence the minor bump).
- **v1.19.1** — ingress admin loopback guard verifies an IP literal instead of trusting the
  name "localhost"; `DefaultCaddyAdmin` → `127.0.0.1:2019`.

New tooling: **`deploy/verify-commit.sh`** — builds a commit in a detached worktree (Go
build/vet/race, the separate vendored ingress module, gofmt, frontend check/lint/test, a
`web/dist` freshness rebuild-and-compare, and the real server image build).

## Open / In Progress

**Operator-owned, stated this session:**

1. **Fresh Google sign-out/sign-in** on crew.styrman.xyz — the one release-verification step the
   agent cannot do. Only the 302 to `auth.styrman.xyz` was confirmed; caddy-security surviving
   four version bumps needs a real browser. Operator said they would do this.
2. **Stop and start the `styrman-harness` project once.** Its container is still the original
   from 13:42 on `styrman-runtime:1.17.3`, so it keeps hostname `styrman-styrman-harness` and
   none of today's image changes. `EnsureRunning` recreates a STOPPED container on image
   mismatch; running ones are deliberately left alone. Operator said they would do this.

**Still needs an operator decision:**

3. **Settings harness roster is permanently empty** (`internal/httpapi/setup.go:230-252`). It
   reports Skills/Agents/Commands from the base/setup home, which the entrypoint never seeds
   (setup VM runs `STYRMAN_MINIMAL=1`). Two options: point the lookup at a project home, or
   delete the three dead fields. Asked twice this session; not resolved.

**Genuinely blocked / accepted:**

4. Two CALLED `docker/docker` CVEs (GO-2026-4887/4883), both **Fixed in: N/A** — recheck only.
5. ADR-056 bearer-token possession trade — accepted risk, recorded.
6. A handful of 🟢 carried-over lows in `TODO.md`.

## Key Decisions

- **Credential-bearing clone URLs are refused, not redacted** (ADR-059). The broker already
  delivers the ssh key AND answers git's credential helper per host, so a token in the URL was
  a second, worse path that git wrote verbatim into `.git/config`. Operator's reasoning:
  "can't you use the ssh key the user already provided and gh login?"
- **A VM runtime is a precondition, asserted at load** (ADR-060). `CAP_SYS_ADMIN` +
  `apparmor=unconfined` is containment only behind a microVM.
- **A release tag is verified against the COMMIT, never the working tree** (ADR-061).
- **helm removed from the runtime image** rather than pinned (operator: "No helm").
- **The ingress guard shipped alone**, not inside v1.19.0 — that release already touched the
  ingress with digest pins, and a crash loop would have been hard to attribute.
- Rejected: tightening the ingress guard without also changing `DefaultCaddyAdmin` — the
  ingress would have failed its own check and crash-looped under `restart:unless-stopped`.

## Corrections

- "Read how to deploy. You should push version to git and the app should auto update" —
  feedback log `2026-09-02T19:05:00+02:00`. The agent ran `docker compose … up -d --build` by
  hand from the DEV repo (following `versioning.md`'s then-current Manual release procedure) and
  took production down: compose reads `.env` from the working directory, prod's lives at
  `/opt/styrman/deploy/.env`, the dev clone has none, so caddy started with no `BASE_DOMAIN`.
  `versioning.md` has been rewritten to lead with the tag+updater path.
- "No helm" — feedback log is the auto-hook entry `2026-09-02T18:40:43Z` ("I don't really use
  helm"). Agent was pinning helm/OpenTofu/yq per a pre-existing TODO; operator wanted helm gone
  entirely instead.

## Agent notes (unverified)

- The agent pushed `b321f35` as v1.18.0 and it **did not compile** — `bootFailed` was extracted
  into `provision.go` which was never staged. Every local gate passed because they ran against
  the working tree. The auto-updater built from the tag, failed, and rolled back to v1.17.3.
  v1.18.0 remains in history as a tag that does not build; it was NOT retagged (that would
  rewrite published history). This is the agent's own error, logged as a `gap`, not a user
  correction.
- Three separate documentation/changelog agents wrote claims that did not match the code this
  session — including an invented `kvm` runtime allowlist and Go tests attributed to a vitest
  file that does not exist. All caught and corrected before landing. **Verify subagent doc
  output against source before accepting it.**
- Two audit subagents (reviewer, security) ran 85 minutes and produced one sentence each before
  being killed. If a scan agent has not produced a report file within ~20 minutes, kill it.

## Environment State

- Branch: `main`, clean, synced with `origin/main` (0 unpushed commits).
- Tags v1.18.1, v1.18.2, v1.19.0, v1.19.1 all pushed and matching remote.
- Live: `styrman-server:1.19.1` (healthy), `styrman-caddy:1.19.1`, `STYRMAN_IMAGE=styrman-runtime:1.19.1`.
- Production deploys from **`/opt/styrman`**, NOT this working copy. `/opt/styrman/deploy/.env`
  holds the secrets. Never run `docker compose` from this repo.
- One project container running: `styrman-harness`, still on `styrman-runtime:1.17.3` (see Open #2).

## Artifacts to Read

- `.agents/docs/changes/2026-09-02-v1.19.0-security-hardening-dead-code-toolchain.md` — the big one.
- `.agents/docs/changes/2026-09-02-v1.19.1-ingress-loopback-guard.md` — the ingress fix.
- `.agents/docs/changes/2026-09-02-v1.18.1-project-create-and-secure-clone-urls.md` — the clone fix.
- `.agents/docs/changes/2026-09-02-v1.18.0-project-create-provisioning.md` — carries a CORRECTION
  section explaining that v1.18.0 never shipped.
- `.agents/docs/architecture/DECISIONS.md` — ADR-058 (provisioning order), 059 (clone URLs),
  060 (VM runtime), 061 (verify the commit).
- `.agents/docs/specs/styrman/versioning.md` — **read before any release.** Rewritten this session.
- `TODO.md` — 23 items closed 2026-09-02; the rest carry reasons.

## Suggested Skills

- `review-standards` (via the `reviewer` agent) — v1.19.0/v1.19.1 shipped with reviewer rows
  from earlier in the session; a fresh independent pass over `ae73cdf..86135fb` would be worth
  having before the next feature lands.
- `golang` — most open items are Go.
- `frontend` / `typescript` — the vitest suite is new and thin; extending it is open work.

## Next Steps

1. Confirm the operator completed Open #1 (fresh Google login) and #2 (restart `styrman-harness`).
2. Get the Open #3 product decision on the Settings harness roster, then implement it.
3. Optional: an independent reviewer pass over `ae73cdf..86135fb` (5 commits, 4 releases).
4. Recheck the two `docker/docker` CVEs when upstream publishes a fix.
