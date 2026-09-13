---
owner: security
date: 2026-09-13
---
# Security review — close-out round 2 (bump-pins shape checks, firewall installer tolerance)   (2026-09-13)

Range reviewed: 336a5d1..48d930b

## Verdict: FINDINGS

Nothing here is exploitable by a project guest or a remote user, and the two
findings that mattered in round 1 are genuinely closed: every publisher value
is shape-checked before it can reach `sed` (the regexes exclude `|`, `&`,
`\`, `/`, whitespace and newline — verified by running them, in both `C` and
`en_US.UTF-8`), and `deploy/firewall/install.sh` now tolerates the degraded
oneshot so `systemctl enable` always runs. What remains is one real
regression in test coverage and two doc/robustness gaps. The regression:
`deploy/firewall/test.sh`'s `run() { bash "${SCRIPT}" || true; }` discards the
firewall script's exit status at every one of its eight call sites, and no
assertion replaces it, so the suite can no longer tell a healthy run (exit 0)
from a warning run (exit 1). Since `deploy/styrman-ctl` in this very same
commit starts treating `enabled/failed` as DEGRADED, a future change that
makes the healthy path emit a spurious WARN would turn every production host
red while `go test ./internal/config` stays green — the exact "silence from
your own instrumentation" shape of BUG-034.

## Closed by this range

- **Round 1 finding 1 (medium, sed injection) — CLOSED.** `image/bump-pins.sh:40-42`
  adds `is_version`/`is_sha256`/`is_digest` and every one of the five publisher
  values plus every registry digest passes one before it is used
  (`:48`, `:56`, `:59`, `:69`). I ran the three predicates against the hostile
  shapes the finding named — `1.27|w /etc/x`, `1.27&x`, `1.27\x`, `1.27/w`,
  an embedded newline, leading/trailing space, `<63 hex>|w`,
  `sha256:<64hex>|w /tmp/x`, `sha256:<64hex>\ns/x/y/` — **all rejected**, in
  `LC_ALL=C` and in `LC_ALL=en_US.UTF-8` (the collation-range concern for
  `[0-9a-f]` does not materialise: glibc rejected even uppercase hex). `&`
  and `\` are excluded structurally: the only character classes are `[0-9]`,
  `[0-9a-f]` and the literal `sha256:`, and both anchors are string anchors
  (bash calls `regexec` without `REG_NEWLINE`, so `^…$` cannot be escaped by
  an embedded newline — confirmed by the `$'1.27\nevil'` case).
- **Round 1 finding 1's second half (half-rewritten tree) — CLOSED.** `:95-111`
  rewrites `mktemp` copies, verifies all five ARG lines with `grep -qxF --`
  (fixed-string, whole-line, `--` end-of-options) and every digest with
  `grep -qsF --`, and only then `mv`s both files. A failed rewrite aborts with
  both Dockerfiles untouched. The delimiter also moved from `/` and `#` to
  `|`, which the shape checks now guarantee cannot appear in any value.
- **Round 1 finding 2 (medium, installer aborts before enable) — CLOSED.**
  `deploy/firewall/install.sh:112-114` is `run systemctl start … || log "WARN: …"`.
  `run()` (`:32`) is `if [ "${DRY}" = 1 ]; then echo "+ $*"; else "$@"; fi`, so it
  propagates the command's status and the `||` catches it; `log` returns 0.
  `run systemctl enable styrman-firewall.service` and
  `run systemctl enable --now styrman-firewall.timer` therefore always run.
  This matches the updater's `host_converge` shape that BUG-035's rule names.
- **Round 1 finding 5 (low, npm not in the tool check) — CLOSED.** `:32` is now
  `for tool in curl jq docker sed npm awk`, and `awk` (also used, also
  previously undeclared) went in with it. All five fetches are additionally
  wrapped so a failure cannot kill the script before the guard: `|| true` on
  the `curl`/`npm` calls, `2>/dev/null` on the `jq`/`awk` filters,
  `// empty` and `[]?` in the jq programs, and a single `fail_read` flag
  checked once at `:73` — so every bad source is *reported* rather than the
  first one aborting silently.
- **Round 1 finding 3 (low, trust model overstated) — PARTIALLY closed.** The
  script header `:17-22` now says it plainly ("TLS to each publisher plus the
  publisher's own published sums. Nothing here verifies a signature (Node's
  SHASUMS256.txt.asc is not checked)"). The mirror into
  `.agents/docs/specs/styrman/runtime-image.md` did not happen — see findings.
- **Round 1 finding 4 (low, npm version with no sum) — documented, not fixed.**
  The header now states "npm publishes no separate sum; the version alone is
  pinned", which is the honest version of the gap; ADR-070 records the
  deferral posture for the close-out's lows generally. No `dist.integrity` is
  printed. Accepting the documentation is a defensible call; noting it here so
  it is not mistaken for fixed.

## Findings

- **[medium / confidence: high]** `deploy/firewall/test.sh:58` —
  `run() { bash "${SCRIPT}" || true; }` throws away the firewall script's exit
  status at all eight invocations (`:64`, `:82`, `:85`, `:89`, `:102`, `:106`,
  `:112`, `:118`, `:123`) and **nothing asserts it anywhere else**. The suite's
  verdict is `[ "${FAIL}" -eq 0 ]` over rule-level assertions only. Before this
  change, `set -e` plus the bare `bash "${SCRIPT}"` meant a script that exited
  non-zero on a healthy host stopped the suite immediately. The `|| true` was
  the right call for the *abort* problem (a host without `xt_physdev` should
  fail at the `physdev RETURN present` assertion, not blow up the runner), but
  it removed the only signal for the opposite defect: a regression that makes
  the clean path emit a WARN. That defect is now expensive, because
  `deploy/styrman-ctl:332` in this same commit makes `enabled/failed` a
  DEGRADED verdict — a spurious WARN puts `styrman-firewall.service` in
  `failed` forever, every 60 s timer tick re-fails it, `styrman-ctl status`
  goes red on every host, and `deploy/firewall/test.sh` (and therefore
  `go test ./internal/config`) stays green throughout. `:106`'s
  `run … && [ $(tagged_count) -eq 0 ]` is now a tautology on its left side for
  the same reason.
  *Demonstrated, not inferred* (both directions, in a scratch copy under
  `unshare -Urnm`, no source touched): a stub firewall script that installs
  **nothing** still fails the suite loudly (6 FAILs, suite exit 1 — the
  `|| true` does not hide that); the **real** script wrapped so it always
  exits 1 while installing every rule correctly yields `PASS=24 FAIL=0`,
  suite exit 0, and therefore a green `TestFirewallScriptInNetns`
  (`internal/config/firewall_script_test.go:105-112` takes test.sh's exit
  status as its whole verdict).
  *Remediation:* keep the tolerance but assert the status. Capture it —
  `run() { local rc=0; bash "${SCRIPT}" >/dev/null 2>&1 || rc=$?; return "${rc}"; }`
  — then add one check in section 1 (`check "clean run exits 0" "run"`, guarded
  by the physdev availability the suite already probes) and one in section 5
  (`check "a warning run exits non-zero" "! run"`). Section 5 already proves
  the *message*; proving the *status* is the piece `styrman-ctl` now depends on.

- **[low / confidence: high]** `.agents/docs/specs/styrman/runtime-image.md:27-38`
  — the doc half of round 1 finding 3 is still open, and it is the half an
  operator reads. It still says every checksum "was cross-checked against the
  publisher's own published sums", that `bump-pins.sh` reads "publisher
  sources only", and that after `--apply` "`docker build image/` proves the
  new sums". What the build proves is that the tarball matches a sum fetched
  from the same origin over the same channel minutes earlier — transport
  integrity, not authenticity — and `--apply` adopts whatever the publisher
  and the registry serve at that instant with no human in the loop. The script
  header now says this; the spec contradicts it.
  *Remediation:* paste the script's `Trust model, stated plainly` paragraph
  into `runtime-image.md` next to the bump procedure, and drop "proves the new
  sums" in favour of "proves the archive matches the sum the publisher listed
  at bump time".

- **[low / confidence: high]** `image/bump-pins.sh:69,73` — a `FROM` whose tag
  has no amd64 manifest now aborts the entire run instead of being skipped.
  The removed line (`report`'s `digest unavailable … continue`) let one
  unresolvable base image pass while the ARG pins still got bumped; now
  `fail_read=1` and `:73` exits. Fail-closed is the right direction, but the
  trigger is broader than "registry down": the tag is extracted by
  `sed -n 's/^FROM \([^ ]*\).*$/\1/p'`, which takes the **first whitespace-
  delimited word** after `FROM`. A future `FROM --platform=linux/amd64 …`
  yields the tag `--platform=linux/amd64`, a `FROM scratch` yields `scratch`,
  and a `FROM ${SOME_ARG}` yields the literal `${SOME_ARG}` — each makes
  `docker manifest inspect` fail and each now bricks the whole bumper with a
  message that blames the registry. Neither Dockerfile uses those forms today
  (4 `FROM` lines, all `name:tag@sha256:…`, two with `AS`), so this is latent.
  *Remediation:* skip a tag that starts with `-`, equals `scratch`, or
  contains `$`, with a distinct message; keep `fail_read=1` for the genuine
  registry failure.

- **[low / confidence: medium]** `image/bump-pins.sh:108` — the digest
  verification is `grep -qsF -- "FROM ${tag}@${DIGEST[${tag}]}" "${tmp_img}" "${tmp_srv}"`.
  `grep -q` over two files succeeds if **either** contains the string, so a tag
  present in both Dockerfiles whose rewrite landed in only one still passes the
  guard and both files are `mv`d into place. `golang:1.25.13-trixie` is exactly
  such a tag (`image/Dockerfile:8` and `deploy/Dockerfile.server:9`). No current
  input produces the split — the two `s|…|` expressions cover both the
  digest-bearing and the bare `FROM` forms — but the guard was added precisely
  to catch the case where a `sed` silently matches nothing, and for the shared
  tag it does not.
  *Remediation:* verify per file, only for the files that actually mention the
  tag: `for f in "${tmp_img}" "${tmp_srv}"; do grep -q "^FROM ${tag}" "$f" || continue; grep -qF -- "FROM ${tag}@${DIGEST[${tag}]}" "$f" || fail; done`.

- **[low / confidence: high]** `image/bump-pins.sh:95,110` — a successful
  `--apply` silently tightens both Dockerfiles' permissions. `mktemp` creates
  at mode 0600 and `mv -f` gives the destination the **source inode's** mode
  — by `rename(2)` on the same filesystem, and by mode-preserving copy across
  devices — so `image/Dockerfile` and `deploy/Dockerfile.server` (both **0664**
  in the tree today) come out 0600 either way. Verified by running both cases.
  Git tracks only the executable bit, so the change survives the commit
  invisibly and surfaces later as a build or read failure for another user,
  or for a container build that reads the tree as non-root. The cross-device
  `mv` is also copy-then-unlink, so the atomicity the comment claims ("never a
  half-rewritten tree") holds for the content *check* but not for the two-file
  move itself.
  *Remediation:* create the temps beside their targets
  (`mktemp "${IMG}.XXXXXX"`) — same filesystem, a real `rename(2)`, and
  `chmod --reference="${IMG}"` (or `cp` the original onto the temp first) so
  the mode is carried over. Project convention also puts scratch in
  `.agents/scratch/`, never `/tmp` (`AGENTS.md`), which the same change
  satisfies.

## Cleared on inspection

- **The `${tag}` interpolation into `sed` (`:86`, `:102`) is not a new hole.**
  `tag` comes from the repo's own tracked Dockerfiles, i.e. an attacker who
  can set it can already edit the build. It is spliced as a BRE, so `.` and
  `-` are the only metacharacters present in the four real tags and both are
  harmless there; `&` and `\` in the *replacement* would matter but Docker's
  reference grammar (`[a-zA-Z0-9]` plus `._-/:`) admits neither, and `|` (the
  new delimiter) is likewise not a legal reference character. Prefix collisions
  are safe too: `debian:trixie` vs `debian:trixie-slim` cannot cross-match,
  because expression 1 requires `@sha256:` immediately after the tag and
  expression 2 requires end-of-line or a literal ` AS `.
- **Double-appending a digest is impossible.** Both `s` commands run on the
  same line in one `sed` pass; once expression 1 has rewritten
  `FROM tag@sha256:old` to `FROM tag@sha256:new`, expression 2's
  `^FROM ${tag}\( AS .*\)\?$` no longer matches (the line does not end at the
  tag and ` AS ` is literal). Verified by reading all four `FROM` lines.
- **`digest_of` parsing a registry's JSON with `jq` is fine.** The registry is
  the untrusted party, but `jq` is a parser, not an evaluator; its output goes
  through `is_digest` before it is used anywhere, and `.manifests[]?` plus
  `2>/dev/null` plus `head -1` mean a malformed or hostile document yields an
  empty string, which fails the shape check and sets `fail_read=1`. A registry
  that serves a *valid-shaped but wrong* digest is a real risk, but it is the
  trust-on-first-use property the header now states, not a parsing bug.
- **`NODE_VERSION` reaching a URL is gated.** `:53-55` only builds
  `https://nodejs.org/dist/v${NODE_VERSION}/SHASUMS256.txt` inside
  `if is_version "${NODE_VERSION}"`, so a hostile `index.json` cannot steer
  `curl` at another host or inject a path. `NODE_MAJOR` (`:50`) comes from the
  repo's own Dockerfile and only reaches `jq --arg`, which is data.
- **`set -e` interactions.** `[ "${1:-}" = "--apply" ] && APPLY=1` (`:30`),
  `is_version … && is_sha256 … || { …; fail_read=1; }` (`:48,56`), and
  `[[ … ]] && [[ … ]] || rc=1` (`styrman-ctl:332`) all end in a command that
  returns 0 on the failure branch, so none of them exits the script — the
  failing left-hand operand of an `&&`/`||` list is exempt from `errexit`.
  `styrman-ctl:19` is `set -euo pipefail` and its own comment at `:283` warns
  about exactly this shape; the new line obeys it.
- **`deploy/firewall/test.sh`'s `|| true` does not hide a script that installs
  nothing.** If the script dies before creating the chains, `rules()` and
  `in_rules()` fail (`iptables -S STYRMAN-FWD` on a missing chain), so
  `physdev RETURN present`, `six private-range DROPs`, `STYRMAN-IN is a bare
  DROP`, `INPUT jump` and `DOCKER-USER jump` all FAIL and the suite exits 1.
  `tagged_count()`'s own `|| true` makes `all three are removed` pass
  vacuously in that scenario, but its neighbours (`the six DROPs are intact`,
  `STYRMAN-IN is a bare DROP again`) do not. The gap is the *status*, recorded
  as the finding above — not the rules.
- **`deploy/styrman-ctl:332`'s new condition.** `unit_state` (`:249-253`)
  emits `"${is-enabled}/${is-active}"`, so `enabled/failed` is the shape being
  excluded and `enabled/inactive` (a healthy oneshot) still passes.
  `enabled-runtime/*` and `absent/*` already fail the `enabled/*` glob. The
  residual is cosmetic: `enabled/activating` and `enabled/deactivating` are
  reported OK, which for a 60 s-tick oneshot is a momentary window, not a
  state a host sits in.
- **`internal/config/styrman_ctl_test.go:190-202` pins the new behaviour.** The
  fake `systemctl` returns `is-active styrman-firewall.service → failed` with
  everything else healthy, and the test requires exit 1 plus `DEGRADED` plus
  the literal `enabled/failed` in the output — so a silent revert to the old
  `[[ == enabled/* ]]`-only check fails `go test ./internal/config`. Ran
  bare: see Checked.
- **`.agents/docs/testing/styrman/integration.sh:131` is tighter than before
  and still not an injection.** `v="$(docker exec … cat …)" && [ -n "$v" ] && [ "$v" = "$(…)" ]`
  — the assignment's status is the `docker exec`'s, so a missing stamp now
  fails instead of comparing `"" = ""`, and `-n` additionally rejects an empty
  file. Guest-controlled bytes arrive through command substitution inside a
  single `eval`, which bash does not re-parse. `v` is used nowhere else in the
  script (grep: this line only), so the global it leaves behind clobbers
  nothing.
- **No dependency manifest changed in the range.** `go.mod`, `go.sum`,
  `web/package.json`, `web/package-lock.json` are untouched; `bump-pins.sh`
  adds no library and shells out only to tools the project already requires.
- **No secret, credential or token appears anywhere in the range**, and the
  new log line (`install.sh:114`) names a unit and a `journalctl` command, no
  values. `bump-pins.sh` sends no credential (`curl` unauthenticated,
  `npm view`, `docker manifest inspect` on public tags).

## Not examined

- No live-host verification of anything: no `systemctl start` against the real
  `styrman-firewall.service`, no root run of `deploy/firewall/install.sh`, no
  `--apply` run of `bump-pins.sh` (it reaches the network and rewrites tracked
  files). Every claim about systemd behaviour is read from the units and the
  scripts.
- `deploy/firewall/styrman-firewall.sh` itself is unchanged in this range and
  was reviewed in round 1; I re-read only `warn()`'s exit path to confirm the
  installer's tolerance is aimed at the right condition.
- `deploy/styrman-ctl` beyond `cmd_status`'s verdict block and `unit_state`.
- The other 81 checks in `.agents/docs/testing/styrman/integration.sh`, and
  `internal/config/firewall_script_test.go` beyond confirming that its verdict
  is `deploy/firewall/test.sh`'s own exit status (which is what makes the
  finding above matter).
- `image/Dockerfile` and `deploy/Dockerfile.server` beyond their `FROM` and
  pinned `ARG` lines.
- Go and web code paths generally — this range touches one Go *test* file and
  no production Go or TypeScript.

## Checked

- **Secrets** — none in the range: no credential, token or key added, and no
  new value reaches a log. `git diff 336a5d1..48d930b` over `deploy image
  internal` greps clean for `password|secret|token|api_key|BEGIN .*PRIVATE`;
  no `.env*`, `*.pem`, `*.key` or credential JSON in the changed paths.
- **Injection** — the round-1 `sed` injection is the whole subject and it is
  closed; I re-tested the three shape predicates against delimiter, `&`, `\`,
  newline and whitespace payloads in two locales and all were rejected. The
  remaining interpolations (`${tag}` into `sed`, `${NODE_MAJOR}` into `jq
  --arg`) are repo-controlled or passed as data. The integration check's
  `eval` is a single evaluation whose command-substitution results are not
  re-parsed. `install.sh`'s new `||` arm passes a fixed literal to `log`.
  `shellcheck -s bash` on all four changed scripts is clean apart from two
  SC2015 *info* notes (`bump-pins.sh:48,56`, "A && B || C is not if-then-else")
  — benign here, because the `C` arm is meant to run whenever either predicate
  fails — and two pre-existing SC2012 notes in `test.sh:31-32`.
  `git diff --numstat` over the range shows real add/delete counts on all six
  files (no `-` columns), so nothing in this diff is binary-classified
  (BUG-007's check).
- **Authz/authn** — no authentication, authorization or identity code in the
  range. `styrman-ctl status` runs `need_root` before anything, unchanged.
- **Crypto** — nothing hand-rolled and nothing cryptographic implemented.
  `bump-pins.sh` only *transports* sha256 strings (and now validates their
  shape); verification stays in the Dockerfile's `sha256sum -c`. The honesty
  of what that proves is the second finding.
- **Dependencies** — no manifest changed, so no new dependency to justify.
  `go test ./internal/config/ -run 'StyrmanCtl' -count=1` was run bare:
  `ok github.com/kesokaj/styrman-crew/internal/config 3.880s`, exit 0. `govulncheck`/`npm audit` were **not** re-run for this range:
  round 1 ran them hours earlier against the identical `go.mod`/`go.sum`/
  `package-lock.json` (its findings stand unchanged — two "Fixed in: N/A" hits
  in `github.com/docker/docker@v28.5.2+incompatible`, one high `js-yaml`
  advisory in `web/`, all pre-existing), and the authoritative run is
  `bash deploy/vulncheck.sh` inside the pinned builder image, which this
  review did not execute.
- **GCP surface** — none. Self-hosted project (`docs/deploy/gcp.md`: not
  applicable); no IAM binding, service account, key file or Cloud Run flag in
  the range. `bump-pins.sh` talks to go.dev, nodejs.org, the npm registry and
  Docker Hub only.
