---
owner: security
date: 2026-09-13
---
# Security review — close-out fixes (firewall warn, entrypoint stamp, bump-pins)   (2026-09-13)

Range reviewed: 3d50c27..336a5d1 (one commit, 336a5d1; 6 files, +107/-3)

## Verdict: FINDINGS

Two things are worth acting on. First, `image/bump-pins.sh` is new code whose
whole job is to take strings off the public internet and put them into a
program: five publisher-supplied values (Go version + sum, Node version + sum,
the npm `latest` version) and every registry-supplied image digest are
interpolated, unvalidated, into `sed -i` expressions whose delimiter is `/`
(and `#` for the digests). A value carrying the delimiter does not merely
break the edit — it ends the `s` command and the remainder of the string is
parsed as further *sed program*, and GNU sed's program language can write
arbitrary files (`w`) and execute the pattern space as a shell command (`e`).
The precondition is a hostile or corrupted publisher/registry response, so
this is not reachable by an outside attacker at will, but the script runs on
the operator's workstation under `--apply` with the operator's credentials,
and the fix is one validation regex per value. Second, the firewall change is
correct in itself — it installs exactly the same rules, and the new `warn()`
reaches the exit path identically to the other three call sites — but it turns
a previously-tolerated condition into a non-zero unit exit, and one caller,
`deploy/firewall/install.sh`, runs under `set -euo pipefail` with
`systemctl start` *before* `systemctl enable`: on a host where that one rule
cannot be installed, the installer now aborts before ever enabling the timer
or the boot unit, which is the fail-open shape BUG-035 is about.

## Findings

- **[medium / confidence: high]** `image/bump-pins.sh:76` and `:79` — publisher
  and registry strings steer a `sed` program. `GO_VERSION`, `GO_SHA256`,
  `NODE_VERSION`, `NODE_SHA256`, `CLAUDE_CODE_VERSION` (lines 33-42) come from
  go.dev's JSON, nodejs.org's `SHASUMS256.txt` and `npm view`; `DIGEST[tag]`
  (line 49-55) comes from whatever `docker manifest inspect` was served. None is
  checked against a shape before it is spliced into
  `sed -i "s/^ARG GO_VERSION=.*/ARG GO_VERSION=${GO_VERSION}/; …"` and
  `sed -i "s#^FROM ${tag}@sha256:[0-9a-f]*#FROM ${tag}@${DIGEST[${tag}]}#"`.
  A value containing the delimiter closes the replacement and the rest of the
  string is read as sed commands and flags — GNU sed's `w` writes files and
  its `e` flag runs the pattern space through a shell, so a compromised
  publisher, a poisoned mirror, or a TLS-terminating middlebox turns a routine
  `--apply` into writes outside the two Dockerfiles on the operator's machine.
  Even without malice, a version string containing `/`, `#` or a newline
  aborts mid-script and can leave `image/Dockerfile` rewritten while
  `deploy/Dockerfile.server` is not (the ARG `sed` and the digest `sed`s are
  separate invocations under `set -e`).
  *Remediation:* validate every value before it goes anywhere near sed and
  abort otherwise — `[[ ${GO_VERSION} =~ ^[0-9]+(\.[0-9]+){1,2}$ ]]`,
  `[[ ${GO_SHA256} =~ ^[0-9a-f]{64}$ ]]`, `[[ ${DIGEST} =~ ^sha256:[0-9a-f]{64}$ ]]`
  — and prefer building the replacement line with `printf` into a temp file
  over `sed -i` with interpolated replacement text. The tag is also spliced in
  as an unescaped *regex* (`.` matches anything); it comes from the repo's own
  Dockerfiles, so that one is cosmetic, but `\Q`-style quoting or a literal
  match would remove the whole class.

- **[medium / confidence: medium]** `deploy/firewall/install.sh:109-114` (blast
  radius of `deploy/firewall/styrman-firewall.sh:74-75`) — the new `warn()`
  makes `styrman-firewall.service` exit 1 whenever the physdev RETURN cannot
  be installed (no `xt_physdev`, or any `iptables` failure on that rule, e.g.
  lock contention past `-w 5`). `install.sh` is `set -euo pipefail`, its
  `run()` does not tolerate failure, and it runs
  `run systemctl start styrman-firewall.service` *before*
  `run systemctl enable styrman-firewall.service` and
  `run systemctl enable --now styrman-firewall.timer`. A `Type=oneshot` unit
  whose ExecStart exits non-zero makes `systemctl start` return non-zero, so
  on such a host the installer aborts with neither the boot unit nor the 60 s
  self-heal timer enabled — the rules from that single manual run are all
  there is, and after the next reboot the `sdb-+` jumps are absent entirely
  (projects reach the host, the LAN, the tailnet and metadata). `host-setup.sh`
  catches the failure and prints a WARN, but the WARN says "projects stay
  isolated only if a previous script is installed", which does not name the
  missing `enable`. The updater's `host_converge`
  (`deploy/update/styrman-update:577,588`) is already immune: it logs a WARN
  instead of aborting and enables unconditionally — that is the shape to copy.
  *Remediation:* in `install.sh`, enable before starting, and tolerate the
  start's status (`run systemctl start … || log "WARN: …"`). `systemctl enable`
  is idempotent, which is exactly why BUG-035's rule says the enable must not
  sit behind anything that can fail.

- **[low / confidence: high]** `image/bump-pins.sh:1-18` and
  `.agents/docs/specs/styrman/runtime-image.md:34-38` — the trust model is
  stated more strongly than it is. "publisher sources only" and
  "`docker build image/` proves the new sums" read as provenance
  verification; what the build actually proves is that the tarball matches a
  checksum fetched from the *same origin over the same channel minutes
  earlier*, i.e. transport integrity, not authenticity. Nothing verifies a
  signature: Node publishes `SHASUMS256.txt.asc` signed by its release keys
  and the script reads only the unsigned `SHASUMS256.txt`; Go's sum comes from
  the same JSON document as the download URL; `docker manifest inspect`
  re-pins each `FROM` digest to whatever the registry serves at bump time,
  which is a trust-on-first-use refresh rather than a check. The old manual
  procedure had a human reading the publisher's release page, and that human
  is what `--apply` removes.
  *Remediation:* say it in the header in one sentence ("verification is TLS
  plus publisher self-consistency; no signature is checked, and `--apply`
  adopts whatever the publisher serves at that moment"), mirror it in
  `runtime-image.md`, and — cheap and worth it — verify Node's `.asc` against
  a pinned key before trusting `SHASUMS256.txt`.

- **[low / confidence: medium]** `image/bump-pins.sh:42` — `CLAUDE_CODE_VERSION`
  is advanced to npm's `latest` with no checksum of any kind (the Dockerfile
  pins only a version string, `image/Dockerfile:114`), so one `--apply` adopts
  whatever the registry currently calls latest, backed by nothing but the
  registry's own metadata. The version pin is pre-existing design (ADR-069);
  what is new is that moving it is now a single unreviewed command.
  *Remediation:* print `npm view … dist.integrity` in the report line so the
  operator sees what is being adopted, or pin the tarball's sha256 alongside
  the version as the Go/Node pins already are.

- **[low / confidence: high]** `image/bump-pins.sh:25,42` — `npm` is used but is
  not in the required-tools loop (`curl jq docker sed`). With `set -e`, a
  machine without npm dies at the bare assignment on line 42 with no message;
  the "could not read every publisher source" guard on line 44-45 can never fire
  for that case. Robustness rather than a vulnerability, but it defeats the
  script's own error path.

## Cleared on inspection

- `deploy/firewall/styrman-firewall.sh:69-75` — the `warn()` substitution
  changes no rule. `warn` returns 0 (it is an `echo` plus an array append), so
  the `||` still swallows the failure under `set -eu` exactly as the bare
  `echo` did; every subsequent `ensure_rule`/`ensure_jump` (the six
  private-range DROPs, `DOCKER-USER sdb-+ → STYRMAN-FWD`, the v6 mirror, the
  ADR-065/066 cleanup) still executes. The `2>/dev/null` binds to `ensure_rule`
  only, so the warning itself still reaches the journal.
- The exit path treats it like every other WARN: appended to `WARNS`, checked
  once at line 165 *after* all rules are in place, `exit 1`. A non-zero exit
  therefore never means "the script bailed before installing the rules".
- The timer cannot be rate-limited into silence by the new failure:
  `StartLimitIntervalSec=60`/`StartLimitBurst=20` against `OnUnitActiveSec=60s`
  is one start per interval, and systemd re-arms `OnUnitActiveSec` regardless
  of the service's result. (Not tested on a live host; read from the units.)
- `internal/config/firewall_script_test.go:45-49` pins the exact
  `|| warn "xt_physdev unavailable` text, so a silent revert to a bare `echo`
  now fails `go test ./internal/config`.
- `image/entrypoint.sh:168` — no interpolation remains in the changed line.
  `${HARNESS_VER}` and `${HARNESS_STAMP}` are positional arguments to
  `sh -c 'printf "%s" "$1" > "$2"' _`, and `$1`/`$2` inside double quotes are
  not re-parsed, so a quote, `;`, `$(…)` or newline in either value is inert
  data. The `_` placeholder for `$0` is present and correct.
- A hostile `HARNESS_VER` cannot do worse than before. It is read from
  `/styrman/VERSION`, written at image build from the `STYRMAN_VERSION`
  build arg (`image/Dockerfile:166`); the pre-fix injection would have
  executed as `${USERNAME}` via `runuser`, i.e. as the project user, who
  already holds passwordless sudo inside its own microVM
  (`image/entrypoint.sh:55`). The fix removes a real quoting hazard on the
  build-arg path without changing any boundary.
- A symlinked stamp path is likewise not worse. `~/.styrman/.harness-version`
  lives in the guest-writable project home, so the guest can point it
  anywhere — but the write runs as that same user inside its own VM, resolves
  inside the guest filesystem, and reaches no host path (the home and the
  workspace are the only binds, and both are that user's own). Redirect
  failure is swallowed by `2>/dev/null || true`, which only costs a re-seed
  next boot. Denial-of-self at most.
- Residual interpolated `sh -c` strings in the same file
  (`image/entrypoint.sh:206-207`, `:265`) splice `${HOME_DIR}` and
  `${PROJECT}`. Not touched by this diff and not exploitable: `USERNAME` is
  constrained server-side by `^[a-z][a-z0-9_-]{0,31}$`
  (`internal/httpapi/profile.go:28,77`) plus a reserved-name list, and
  `PROJECT` is a server-generated UUID. Worth converting for consistency, not
  a finding.
- `.agents/docs/testing/styrman/integration.sh:126-132` — the new check is
  test-only and safe: `check()` is a single `eval`, and the guest-controlled
  stamp contents arrive through `"$(…)"` command substitution, whose result
  bash does not re-parse. A guest writing shell metacharacters into the stamp
  gets a failed comparison, not execution.
- No dependency manifests changed in the range (`go.mod`, `go.sum`,
  `package.json`, `package-lock.json` all untouched).
- `shellcheck -s bash image/bump-pins.sh` reports only SC2015 (info) on line
  44, which is benign here: the `||` is meant to fire when any of the `[ -n ]`
  tests fails.
- The `[ "${1:-}" = "--apply" ] && APPLY=1` guard does not trip `set -e`
  (verified by running the construct); preview mode reaches the end.

## Not examined

- The rest of `image/entrypoint.sh` beyond the harness block and the two
  neighbouring `sh -c` sites, and all of `image/Dockerfile` apart from the
  `ARG`/`FROM` lines the bumper edits.
- The rest of `.agents/docs/testing/styrman/integration.sh` (81 other checks)
  and `deploy/firewall/test.sh` beyond confirming it asserts the physdev
  RETURN and runs under `set -euo pipefail`.
- No live-host verification of any kind: no `systemctl start` against the real
  unit, no run of `bump-pins.sh` (it reaches the network and, with `--apply`,
  edits tracked files), no root firewall test. Every claim above is read from
  the source and the unit files.
- Go and web code paths generally: this range touches none, so
  `govulncheck ./...` and `npm audit` were run only as a baseline, not as a
  check of changed code — see Checked.

## Checked

- **Secrets** — no credential, token or key appears in the range; nothing new
  is logged. The two log lines near the changed entrypoint code print the
  harness version and a truncated command tail (`tr -cd '[:print:]' | cut -c1-200`),
  neither of which carries a secret. `image/bump-pins.sh` reads only public
  endpoints and sends no credential (`curl` with no auth, `npm view`,
  `docker manifest inspect` against public tags).
- **Injection** — the one real hit is finding 1: unvalidated publisher and
  registry strings interpolated into `sed -i` programs in
  `image/bump-pins.sh:76,79`. The entrypoint's stamp write is now
  argument-passed and clean; the integration check's `eval` is a single
  evaluation whose command-substitution results bash does not re-parse; the
  firewall script's tagged-rule cleanup still rebuilds `iptables` argv from a
  strict whole-line regex rather than re-splitting text.
- **Authz/authn** — no authentication, authorization or identity code in the
  range. The guest-username regex and reserved-name list that bound the
  entrypoint's remaining interpolation were read
  (`internal/httpapi/profile.go:28,77-82`) but not otherwise reviewed.
- **Crypto** — nothing hand-rolled, and nothing cryptographic is implemented
  here. `bump-pins.sh` only *transports* sha256 strings it never computes;
  the verification itself stays where it was, in the Dockerfile's
  `sha256sum -c` step. The honesty gap about what that verification proves is
  finding 3.
- **Dependencies** — no manifest changed in the range (`go.mod`, `go.sum`,
  `package.json`, `package-lock.json` untouched), and `bump-pins.sh` adds no
  library — it shells out to `curl`, `jq`, `docker`, `sed` and `npm`, all
  already required by the project's toolchain (`npm` is used without being
  declared: finding 5). Baseline runs, neither caused by nor affected by this
  range: `govulncheck ./...` exits 3 with 31 findings, but the local toolchain
  here is **go1.25.0** while the artifact is built in the pinned
  `golang:1.25.13-trixie` — every stdlib hit is an artifact of this machine,
  and the authoritative run is `bash deploy/vulncheck.sh` inside the pinned
  builder, which was **not** executed for this review. Two genuine non-stdlib
  hits stand regardless of toolchain: GO-2026-4887 and GO-2026-4883 in
  `github.com/docker/docker@v28.5.2+incompatible`, both "Fixed in: N/A" —
  pre-existing, no upgrade available, recorded here only so they are on the
  record. `npm audit` in `web/` reports 1 high (`js-yaml` 4.0.0-4.3.1,
  GHSA-2883-xcg3-v3hh, fix available) — also pre-existing and untouched by
  this range.
- **GCP surface** — none. This project is self-hosted (`docs/deploy/gcp.md`
  records "not applicable"); the range contains no IAM binding, service
  account, key file or Cloud Run flag. `bump-pins.sh` talks to go.dev,
  nodejs.org, the npm registry and Docker Hub only.
