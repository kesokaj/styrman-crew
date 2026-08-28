# Plan — credential service (share ONLY credentials; image carries the rest)

Date: 2026-08-28 · Status: PLAN (operator: "plan then do"). Refines and
supersedes `2026-08-28-per-project-home-shared-credentials.md`. §4 matrix
filled from a researched per-tool pass (sources in that research). DO NOT
implement until the operator approves the R3/R5 wording + a phase.

## Operator's refined direction

- **The base image already gives every project the same defaults + harness.**
  So per-project homes need NO config/harness syncing — the image is the
  single source of identical defaults for all projects.
- **The only thing shared across projects is credentials.**
- **A credential SERVICE handles that** — not a shared filesystem mount.

This is cleaner and more secure than the earlier "mount a shared credential
store into every home": no cross-VM file races, and (for tools that support
it) the raw secret never lands readable in a project at all.

## Target architecture

Three clean layers, no shared home:

1. **Base image = identical defaults + harness for every project.** Move the
   agent harness (claude/agy skills, subagents, commands, settings) and the
   shell dotfiles from "deployed into a shared home" (home-init.sh, R5) to
   **baked into the image** (a skeleton the entrypoint seeds into each fresh
   per-project home, or `/etc/skel`). Every project boots with the same
   config; updating it = rebuild/redeploy the image. **This changes R5's
   mechanism** (harness in image, not shared home) — needs the operator's
   REQUIREMENTS approval.
2. **Per-project home = truly individual.** Each project gets its own home
   volume, seeded from the image skeleton, holding only that project's own
   generated state (caches, installs, history, build output). Nothing leaks
   between projects because nothing is shared at the FS layer.
3. **Credential service = the one shared thing, brokered not mounted.** A
   central service owns the single live copy of each credential and delivers
   it to projects on demand, refreshing centrally. Authenticate once; every
   project uses the current credential; a refresh anywhere propagates.

## The `~/.claude` / `~/.gemini` split (load-bearing)

These dirs hold BOTH harness (skills/subagents/commands/settings → image,
identical for all) AND auth (→ credential service). The service must deliver
ONLY the auth portion into an otherwise image-provided dir. Exact auth file
paths per tool come from §4 research; the split is: image bakes the harness
subdirs, the service supplies the credential file(s).

## Credential service — design

### Where it runs
The **styrman server** is the natural home for it: it already brokers every
project, holds the ingress trust boundary, and is already attached to each
project's bridge for the port-preview proxy. A separate sidecar is possible
but adds moving parts; start in-server behind a clear internal interface.

### Delivery channel (must respect ADR-029: projects can't reach the host)
Two viable channels; likely a mix per tool:
- **Push by exec (server → project).** The server already runs `docker exec`
  into every project (that's how terminals work). It can inject/refresh
  credential material or run credential-helper responses the same way — NO
  project→service network path needed, fully compatible with ADR-029. Best
  for file-injection and for seeding.
- **Metadata-server-style endpoint on the project's OWN bridge.** The server
  is on each `sdb-<uuid>` bridge; it could expose a link-local credential
  endpoint (the cloud "169.254.169.254" pattern) that ONLY that project can
  reach, authenticated by a per-project token injected at boot. Best for
  pull-on-demand helpers (git/docker credential helper, gcloud credential
  process) that call out per operation. This is a NARROW, authenticated
  channel — not the general host access ADR-029 blocks — but it does mean
  the project can reach the server on its bridge, so the endpoint needs its
  own per-project auth and a tight, credential-only surface.

### Per-tool delivery tiers (fill from §4 research)
Preference order, best first:
1. **Agent socket** — project signs, can't read the key (ssh-agent).
2. **Credential helper / credential process** — secret fetched per op from
   the service, never stored (git, docker, gcloud, possibly gh).
3. **Env-injected token** — short-lived token in the session env (gh
   GH_TOKEN, npm NODE_AUTH_TOKEN, if honored).
4. **Ephemeral file injection** — for tools that only read a file
   (claude/agy auth?): the service writes the current auth file at session
   start and collects a refreshed one back to the store. Least pure; the
   fallback tier.

### The store + refresh
- Master credentials live in the service's store (host-side, service-owned,
  0600, ideally encrypted at rest / via the keyring). Projects never see the
  store directly.
- **Onboarding** (setup VM): each tool is authenticated ONCE; the resulting
  credential lands in the service store, not a project home.
- **Refresh/rotation**: for helper/agent tiers the service owns the live
  token and refreshes centrally (all projects get the new one on next use).
  For the file-injection tier, an in-project refresh must be written BACK to
  the store (the awkward case — minimize how many tools land here).

## §4 — Per-tool credential matrix (researched 2026-08-28)

7 of 8 tools support a broker mechanism where the raw secret never lands
readable in the project. agy is the one exception.

| Tool | Secret stays out? | Mechanism | Delivery tier |
|---|:--:|---|---|
| ssh | YES | per-project ssh-agent holds the key; project signs over `SSH_AUTH_SOCK`, never reads it | **agent-socket** |
| git | YES | `credential.helper` = broker exec (inject via `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_COUNT`); per-op token, supports `password_expiry_utc`/`oauth_refresh_token` | **credential-helper** |
| gh | YES | honors `GH_TOKEN`; or `gh auth setup-git` makes it the git helper | **env-token** (or reuse git helper) |
| gcloud | YES | `external_account` config with `credential_source.executable` (needs `GOOGLE_EXTERNAL_ACCOUNT_ALLOW_EXECUTABLES=1`); no refresh token on disk | **credential-process** |
| docker | YES | `credsStore`/`credHelpers` → `docker-credential-broker`; per pull/push token | **credential-helper** |
| npm | PARTIAL | `npm_config_//registry/:_authToken` env (no helper protocol); static, rotate before expiry | **env-token** |
| claude | YES | `apiKeyHelper` script → broker (re-called every 5 min / on 401), or inject `ANTHROPIC_AUTH_TOKEN` / `CLAUDE_CODE_OAUTH_TOKEN` | **helper** (pref) / env-token |
| agy | **NO** | OAuth stored ONLY in the Secret Service (gnome-keyring, already plumbed); file fallback broken (bug #479); no working OAuth env var (issue #632 open) | **keyring-seeding (push)** |

### claude / agy — auth vs harness split (exact)
- **claude**: auth = `~/.claude/.credentials.json` (or skip it entirely with
  `apiKeyHelper`/env); harness+config = the rest of `~/.claude` +
  `~/.config/anthropic`. → image bakes the harness; broker supplies auth via
  `apiKeyHelper` (the file is then unnecessary).
- **agy**: auth = gnome-keyring Secret Service item; config = `~/.gemini/
  antigravity-cli/settings.json` + logs. → image bakes the config; broker
  seeds the keyring item. (Exact keyring service/account name is
  [unverified] — discover once via `secret-tool` after a reference login.)

### Broker shape (recommended): metadata-server pull on the project bridge
Model on GCP/AWS instance metadata: a credential-only HTTP endpoint at a
fixed link-local address on each project's OWN bridge (the styrman server is
already on that bridge). **Identity = network position** — the orchestrator
solely owns the bridge, so the request's source IS the project; no secret
zero. Only short-lived derived tokens leave the broker; refresh in the
master propagates to all automatically (everyone pulls the same live copy).
Keep the metadata-server SSRF defenses: a required non-forwardable header,
endpoint unroutable off the bridge, credential-only surface (nothing else of
the server API). Use **push-by-exec** (the terminal channel) only for
bootstrap and for **seeding agy's keyring**. ADR-029 stays intact: this is a
narrow, authenticated, credential-only bridge endpoint, not host access.

## Phasing (build order once approved)

- **P1 — deliver the operator's actual ask** (individual projects + shared,
  current credentials, no re-auth): base image carries config+harness;
  per-project homes seeded from it; broker holds the master credentials and
  delivers via the cleanest per-tool tier that's low effort — env/helper for
  git/gh/gcloud/docker/npm/claude, ssh-agent socket, keyring-seed for agy.
  This already satisfies "auth once, shared, kept current, never re-auth."
- **P2 — security hardening** (not requested, but the research makes it
  cheap to grow into): short-lived/dynamic tokens, the metadata endpoint
  with full SSRF defenses, per-project scoping, dynamic backend secrets.

The operator asked for *shared + current + no-re-auth* — that is P1. "Secret
never readable in the VM" is a P2 bonus the tool support happens to enable;
worth stating so we build P1 in a way that grows into P2, not against it.

## Migration
- Harness/dotfiles: move into the image; existing shared-home copies become
  redundant (the per-project seed takes over). Low risk (config, regenerable).
- Credentials: extract the current shared-home credentials into the service
  store ONCE (onboarding-equivalent), then per-project homes stop carrying
  them. Guard like the ADR-027 sweep; credentials are the one thing we must
  not lose or leave stale — highest-care step.
- Homes: current single shared home → per-project homes seeded from the
  image. Same guarded, VMs-stopped, idempotent approach as ADR-027.

## Risks / open questions
1. **File-injection refresh loop** for any tool that only reads a file and
   refreshes it in place (candidate: claude/agy) — writing the refreshed
   token back to the store without races across concurrent projects. §4
   determines who's in this tier.
2. **Metadata-endpoint auth** — if we use the bridge endpoint, the
   per-project token must be unguessable, injected out-of-band (env at
   boot), and the endpoint must expose credentials ONLY (no other server
   API), or it widens the ADR-029 surface.
3. **Keyring** — agy's Secret Service currently uses gnome-keyring plumbed
   per-home; how it maps to a service (does agy read a keyring entry the
   service can populate, or a file?). §4/agy.
4. **R5** rewrite (harness in image) + **R3** rewrite (per-project home) —
   both need operator approval (REQUIREMENTS.md is approval-gated).
5. **Scope/size** — this is a real subsystem (a broker + per-tool adapters +
   onboarding + migration). Suggest phasing: (P1) image carries
   harness/config + per-project homes with a SIMPLE credential delivery
   (exec-injection of the current files) to get the model right; (P2)
   upgrade high-value tools to helper/agent tiers so secrets stop landing in
   projects. P1 alone already delivers "individual projects + no re-auth".

## Deliverable this round
This document (architecture + the decisions/research needed). Nothing built.
Next: complete §4 from research, get operator approval on R3/R5 wording and
the P1/P2 phasing, then implement P1 behind an ADR, adversarially reviewed.
