# Security review — redfix + no-mutable-tags working tree   (2026-09-01)

Scope: the whole uncommitted working tree at HEAD `00d1c18` — `deploy/.env.example`,
`deploy/Dockerfile.server`, `deploy/caddy/Dockerfile`, `deploy/caddy/ingress/{cmd,generator,watcher}.go`
+ new `loopback.go`/`loopback_test.go`/`watcher_test.go`, `deploy/caddy/ingress/go.{mod,sum}`,
`deploy/docker-compose.yml`, `deploy/host-setup.sh`, `deploy/update/{styrman-update,styrman-update.service}`,
`internal/config/config.go`, `internal/orchestrator/{exec,orchestrator}.go` + `tailbuffer_test.go`.
Blast radius followed: the Caddy admin API end-to-end (caddy 2.11.3 source in the module cache),
the container-logs path from `projectLogs` down to `StdCopy`, compose interpolation precedence
(resolved with `docker compose config` against a scratch env file), and the root-privileged
updater's guard ordering.

## Verdict: FINDINGS

The two audit reds are genuinely closed, and one of them is closed better than its own comment
claims — but the diff introduces a **root-run destructive filesystem mutation on the path its own
error message recommends**. `styrman-update` now *requires* `STYRMAN_UPDATE_STATE_DIR`, and its
failure text tells a tester to "pass a scratch prefix, e.g. `$(mktemp -d)`". Line 95 then runs
`install -d -m 700 -o root -g root "$(dirname "${STATE_DIR}")"` as root — and GNU `install -d`
applies mode and ownership to an **already existing** directory (measured in a scratch dir, see
Checked). Following the documented invocation therefore chmods `/tmp` to `0700 root:root`,
clearing the sticky bit and breaking every non-root process on the host. That is exactly the
class of privileged-test-run state mutation ADR-052 was written after. Second: the new
`golang:1.25.3-trixie` pin is stale by ten patch releases — `govulncheck` reports 31 CALLED
stdlib vulnerabilities in the main module, ~23 of which are fixed only in go1.25.4…1.25.13
(h2c `ReadHeaderTimeout`, TLS post-handshake message flooding, `net/textproto`, `crypto/x509`),
so the Dockerfile comment "fixed by 1.25.2/1.25.3" is wrong on the record and ships a
knowingly-vulnerable internet-facing binary.

## Findings

- **[high / confidence: high]** `deploy/update/styrman-update:95` (line unchanged, but newly
  reachable for every invocation because of `:42`/`:77`) —
  `install -d -m 700 -o root -g root "$(dirname "${STATE_DIR}")" 2>/dev/null || true`, executed
  after the `id -u` root check, **chmods and chowns an existing parent directory**. GNU `install
  -d` is not "create if missing": on an existing dir it applies `-m`/`-o`/`-g` (verified in
  scratch: a 0755 dir became 0700; the `-o root` variant failed only because the test ran
  unprivileged — the script always runs as root). Before this diff the default `/etc/styrman/update`
  made the parent always `/etc/styrman`; now every caller must supply a path, and the new fail
  message names `$(mktemp -d)` → parent `/tmp` → `/tmp` becomes `0700 root:root` with the sticky
  bit gone. Host-wide denial of service against every non-root process (the operator's shell,
  builds, sockets), and a silent one: `2>/dev/null || true` swallows nothing here because it
  succeeds. The same shape hits `/opt`, `/var/lib`, or an operator scratch root. Remediation:
  create only what is missing and never re-mode an existing dir —
  `[ -d "$(dirname "$STATE_DIR")" ] || install -d -m 700 -o root -g root "$(dirname "$STATE_DIR")"`,
  and likewise for the leaf; the existing `stat -c %u` / `-perm /022` assertions already cover the
  "it existed and is wrong" case, so forcing the mode buys nothing.

- **[medium / confidence: high]** `deploy/Dockerfile.server:5` — `FROM golang:1.25.3-trixie` with
  the comment "go1.25.0 also carried 27 stdlib vulns govulncheck flags as CALLED, fixed by
  1.25.2/1.25.3". `govulncheck ./...` (main module, today) reports **33 CALLED** vulns: 31 stdlib
  + 2 unfixable `docker/docker`. Of the stdlib set, roughly 23 are fixed only **after** 1.25.3 —
  e.g. GO-2026-6089 (`net/http`, h2c `ReadHeaderTimeout` bypass, reached from
  `cmd/styrman/main.go:149` `ListenAndServe`), GO-2026-6090 (`crypto/tls` post-handshake message
  flood), GO-2026-5039 (`net/textproto`), GO-2026-5037/4947/4946 (`crypto/x509`), GO-2026-4918
  (`net/http`) — all fixed in 1.25.9…1.25.13. The pin therefore ships an internet-facing server
  with reachable, already-patched DoS/parsing bugs, and the code comment asserts the opposite.
  Remediation: pin the newest 1.25.x patch (1.25.13 at the time of this scan), re-run
  `govulncheck ./...`, and correct the comment to state the exact patch level it claims.

- **[medium / confidence: medium]** `deploy/caddy/Dockerfile:8,18` — the Caddy binary's **Go
  toolchain is neither pinned nor asserted**. `caddy:2.11.4-builder` supplies whatever Go its
  base image carried on build day, so the ingress binary's stdlib patch level is unknown and
  unreproducible — the exact property this diff set out to fix for plugins. `govulncheck` in
  `deploy/caddy/ingress` reports 29 CALLED (27 stdlib + the same 2 docker) against the local
  go1.25.0; whether the shipped binary shares them is unverifiable from the repo. The image
  already asserts its module set (`RUN caddy list-modules | grep …`); add the analogous toolchain
  assertion (`go version -m /usr/bin/caddy` must report ≥ the intended go1.25.x) and pin the base
  images by digest (`caddy:2.11.4-builder@sha256:…`, `golang:1.25.x-trixie@sha256:…`) — a tag pin
  is still mutable upstream, so "tag → binary" remains non-deterministic without it. (Plugin
  version pins *do* get integrity from the Go checksum DB, which is the right mechanism;
  `go mod verify` passes in both modules.)

- **[medium / confidence: high]** `deploy/host-setup.sh:312,475` — the privileged setup script
  still defaults to a mutable tag, `RUNTIME_IMAGE="${STYRMAN_IMAGE:-styrman-runtime:latest}"`,
  and its printed instruction is
  `STYRMAN_VERSION=$(git -C .. describe --tags --always) docker compose up -d --build` — which
  yields a **`v`-prefixed** value (`v1.16.0`). After this diff, compose derives
  `STYRMAN_IMAGE=styrman-runtime:${STYRMAN_VERSION}` and `config.Load` refuses an empty value, so
  a fresh install builds `styrman-runtime:latest` and then tells the server to boot
  `styrman-runtime:v1.16.0`, which was never built: every project start fails. Fail-closed, so
  this is deploy integrity + availability rather than an exposure — but it leaves the one mutable
  `:latest` build in the tree in the change whose stated purpose is removing them, and it makes
  the tag namespace ambiguous (`1.16.0` from the updater vs `v1.16.0` from host-setup).
  Remediation: derive `RUNTIME_IMAGE` from the same `${LATEST#v}` shape the updater uses, require
  the version, and drop the `:latest` fallback.

- **[low / confidence: high]** `deploy/caddy/ingress/loopback.go:11-16` — `isLoopbackAddr`
  validates the host but accepts **any port string**: `net.SplitHostPort` does not check the port,
  so `CADDY_ADMIN='127.0.0.1:2019 }'`, `'127.0.0.1:2019}\nrespond …'` or `'127.0.0.1:2019 origins
  0.0.0.0'` all return `true` (measured in scratch; colon-bearing payloads are rejected only
  incidentally, by `SplitHostPort`'s "too many colons"). The value is then written verbatim into
  the generated global options (`generator.go:214`, `admin %s { origins %s }`), i.e. this guard is
  the only validator standing between an env var and Caddyfile text. Reachability today is
  blocked by ordering luck, not by the guard: `cmd.go:48` hands the same string to `caddy.Run` as
  `tcp/<addr>` first, and Caddy's address parser rejects a non-numeric port, so the process exits
  before the generator runs. `127.0.0.1:0` also passes and binds an ephemeral port (the watcher
  then never reaches the admin → 4 transport failures → exit 1 → restart loop). Input is
  operator-controlled (`deploy/.env`), which caps severity. Remediation: require
  `strconv.ParseUint(port, 10, 16)` in 1..65535 inside `isLoopbackAddr`.

- **[low / confidence: medium]** `deploy/caddy/ingress/loopback.go:19-21` — `localhost` is accepted
  **by name**, and Caddy then binds whatever the container's NSS/`/etc/hosts` resolves it to. In
  the shipped compose service that is the stock loopback entry, but the guard's whole job is to be
  true regardless of resolution state, and `DefaultCaddyAdmin` makes this the default path. Also
  `strings.Trim(host, "[]")` accepts malformed brackets (`"::1]"`) — harmless today because only
  valid loopback literals survive `ParseIP`. Remediation: resolve and require *every* returned
  address to be loopback, or restrict to IP literals and let the default be `127.0.0.1:2019`.

- **[low / confidence: high]** `deploy/update/styrman-update:78-83` — the `/var/lib/styrman`
  refusal is a literal prefix match on the raw string, so it is trivially evaded by shapes an
  operator could plausibly type: `/var/lib//styrman/update`, a relative path (resolved against
  whatever cwd the script was launched from, before the `cd "${DEPLOY_DIR}"` at :106), or a
  symlink at an allowed path pointing into `/var/lib/styrman` (the subsequent `stat -c %u`
  follows symlinks). Remediation: require an absolute path and canonicalise before matching —
  `case "$STATE_DIR" in /*) ;; *) fail …; esac; STATE_DIR="$(realpath -m "$STATE_DIR")"` — then
  run the `case`.

- **[low / confidence: medium]** `deploy/caddy/ingress/generator.go:214` — emitting an explicit
  `origins` **narrows** Caddy's default. With `Origins == nil` and a loopback bind, caddy
  `admin.go:allowedOrigins` allows all three spellings (`localhost:<p>`, `127.0.0.1:<p>`,
  `[::1]:<p>`); pinning one string means any other local admin client — `caddy reload`, an
  operator `curl 127.0.0.1:2019/…` while `CADDY_ADMIN=localhost:2019` — now gets
  `403 host not allowed`. The watcher stays consistent because both values derive from
  `cfg.AdminListen`, so this is a runbook footgun, not a live break; the risk is that a future
  403 gets "fixed" by widening `origins` (which is the one part of the block that is a real
  control against browser-origin requests). Worth one line in the deploy spec rather than a code
  change.

- **[low / confidence: high]** `deploy/docker-compose.yml:15,66` — version-named image tags are
  still **locally mutable**: nothing stops `styrman-caddy:1.16.1` from being rebuilt from a
  different tree, and `up -d` reuses whatever local tag exists. The rule as implemented removes
  the `:latest`/`:dev` ambiguity (a real improvement, and the `:?` fail-closed check verified
  below is the right shape) but does not make the mapping immutable. If the operator rule's intent
  is "what is running is provable", the endpoint is a registry + digest pin
  (`image: …@sha256:…`), which also gives rollback something to verify against.

## Checked

- **Secrets** — no credentials, tokens or keys in the diff. `deploy/.env` is untouched, `0600`,
  and matched by `.gitignore:2`; `deploy/.env.example` carries placeholders only. Grep of the live
  `.env` was limited to non-secret keys (`STYRMAN_IMAGE`/`STYRMAN_VERSION`/`CADDY_ADMIN` — all
  absent, so the new compose defaults govern). No secret reaches a log or an error string: the new
  `config.Load` message names only variable names, `styrman-update` logs versions and paths, and
  the watcher logs `admin_addr` (non-secret). The compose allow-list for the internet-facing
  ingress is unchanged and still explicit rather than `env_file`.
- **Injection** — Caddyfile injection via `CADDY_ADMIN` is the one live parsing gap (low finding
  above), blocked in practice by `caddy.Run` parsing first. `styrman-update`: `ver` is
  `${LATEST#v}` where `LATEST` matched `^v[0-9]+\.[0-9]+\.[0-9]+$`, so `docker build -t
  "styrman-runtime:${ver}"` and the compose env are shell-quoted and shape-constrained; the
  state-file read is still regex-validated against a leading `-`. `Orchestrator.Logs` takes
  `tail` from the query string but clamps to 1..2000 before `strconv.Itoa`. Nested compose
  interpolation resolves as intended (verified: `styrman-runtime:9.9.9`), and a missing
  `STYRMAN_VERSION` fails closed with the `:?` message rather than defaulting.
- **Authz/authn** — no auth code changed. `projectLogs` (`internal/httpapi/stats.go:125`) still
  goes through the owner-scoped `getProjectFor` before reaching the rewritten `Logs`; the new
  truncation banner adds no data and drops no check. Admin-API exposure end-to-end: `loadConfig`
  refuses a non-loopback bind for the initial minimal config, and — read out of the 2.11.3 source
  — `caddy.run()` calls `replaceLocalAdminServer(newCfg)` **before** apps are provisioned, while
  `changeConfig`'s failure path restores only `rawCfg`, never the running admin server. So after a
  rejected `/load` the LIVE admin endpoint belongs to the FAILED config. That is precisely the
  19:28 incident shape (`0.0.0.0:26293`, "host checking disabled" is caddy's own
  `admin.go:454` warning), and what forecloses it now is the new `admin <loopback>` line in every
  generated Caddyfile — the generator's comment undersells it as "defense-in-depth ONLY", which
  is true of `origins` but **not** of the `admin <addr>` bind. Recommend restating that comment so
  nobody deletes the block believing it is decorative. `admin` remains root-equivalent to any
  local process on host loopback, as the comment honestly says.
- **Crypto** — nothing hand-rolled and nothing crypto-adjacent added. The tag-signature path
  (SSH-only, pinned `allowed_signers`, last-armor-header parse) is unchanged by this diff and
  still fails closed; `Environment=STYRMAN_UPDATE_STATE_DIR=/etc/styrman/update` in the unit leaks
  nothing beyond a path already documented in the repo.
- **Dependencies** — `govulncheck ./...` main module: **33 CALLED** (31 stdlib against the local
  go1.25.0 toolchain + the 2 unfixable `docker/docker` findings GO-2026-4887/4883) — no new
  third-party CALLED findings vs the audit baseline. `govulncheck ./...` in
  `deploy/caddy/ingress`: **29 CALLED** (27 stdlib + the same 2 docker); the caddy 2.9.1 → 2.11.3
  bump *removed* the old `smallstep`/`quic-go 0.48`/`certmagic 0.21`/`badger`/`pgx` subtree
  entirely — a real reduction in attack surface, with zero new CALLED module vulns. `go mod verify`
  → "all modules verified" in both modules; no `replace` directives, only a pre-existing
  `exclude`. `npm audit --omit=dev` in `web/` → **found 0 vulnerabilities**. The stdlib numbers
  reflect the *local* toolchain; the shipped ones depend on the base-image pins, which is the
  substance of the two toolchain findings above. `install -d` semantics were measured in a
  scratch dir only (an existing 0755 dir became 0700); nothing was run against `/tmp`,
  `/var/lib/styrman`, `/etc/styrman`, or any production container.
