---
owner: reviewer
date: 2026-09-22
---
# Review — project-docs   (2026-09-22)

Range reviewed: the whole repository at HEAD (c05219c), no diff

## Status: COMPLETE

## Verdict: REQUEST CHANGES

Docs lens over the living docs only (`README.md`, `AGENTS.md`,
`REQUIREMENTS.md`, `.agents/rules/`, `.agents/skills/`, `.agents/agents/`,
`.agents/commands/`, `.agents/docs/specs/`, `.agents/docs/testing/guide.md`
and `.agents/docs/testing/styrman/`, `.agents/docs/deploy/`, plus the test
suites). The worst problem is a documented command that cannot work: since
ADR-074 the runtime image's `docker build` REQUIRES a GitHub token as a build
secret (`image/Dockerfile:150`, `--mount=type=secret,id=gh_token,required=true`),
and five living-doc copies of that command still give the pre-ADR-074 form
without it — including `README.md`'s Quick start and `.agents/docs/deploy/local.md`'s
Build block, i.e. the first command a new operator runs. Next worst:
`README.md` still advertises **v1.34.1 deployed** with a release list stopping
at v1.35.0 (v1.36.0 and v1.37.0 are live), and its "Hard isolation" paragraph
still describes the ADR-065/066 root-mediated LAN model that ADR-067 deleted —
including the SSH-key withholding whose removal has a test asserting the
inverse. Nothing here is a code defect; the code is consistent with ADR-067
and ADR-074, and the docs are what lag.

## Findings

### Commands that no longer work (a changed interface the docs do not mention)

- **[high / confidence: high]** `README.md:271` — `docker build -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/` fails at `image/Dockerfile:150`, which mounts `id=gh_token` with `required=true` (ADR-074). Fix: the form `AGENTS.md:45` already carries — `GH_TOKEN=$(gh auth token) docker build --secret id=gh_token,env=GH_TOKEN -t styrman-runtime:dev --build-arg STYRMAN_VERSION=dev-$(date +%s) image/`.
- **[high / confidence: high]** `.agents/docs/deploy/local.md:21` — same stale build line in the local deploy guide's Build block. Same fix.
- **[medium / confidence: high]** `.agents/docs/specs/styrman/guide.md:463` — same stale build line in the living full guide's dev-mode walkthrough.
- **[medium / confidence: high]** `.agents/docs/specs/styrman/runtime-image.md:7` — the runtime-image spec's own "Built locally" block omits `--secret id=gh_token` while the same file at line 232 explains the ADR-074 download. The doc contradicts itself.
- **[medium / confidence: high]** `.agents/docs/testing/styrman/README.md:128` — the standalone test-image build line omits the secret. The suites themselves are correct (`ensure-test-image.sh:59` passes `--secret id=gh_token,env=GH_TOKEN` and fails closed without a token); only the hand-run line is wrong.

### Statements whose behaviour is gone

- **[high / confidence: high]** `.agents/docs/deploy/local.md:31` and `:36` — `STYRMAN_FIREWALL_DIR` is documented as a configuration variable (with an ADR-065 spool/status-dir explanation and a production-aliasing warning) and is passed in the documented Run command. The variable exists nowhere in the code: `grep -rn "STYRMAN_FIREWALL_DIR\|FirewallDir" internal/ cmd/` returns nothing — ADR-067 deleted the spool. The promised fallback ("reports every project `unavailable`") is gone too: `web/src/components/LocalNetwork.tsx:11` — "There is no host half to be pending, unavailable or in error". Delete the row and the flag from the run line.
- **[high / confidence: high]** `README.md:50-61` ("Hard isolation") — presents the deleted ADR-065/066 design as current: "the per-project LAN allowance applied by the same root script within about a second of the switch"; "the host itself and host-published ports are inside it only where the operator's root-owned conf opts in (both default closed)"; "a LAN-enabled project stops being served the operator's SSH key". Under ADR-067 the mode is only the Docker bridge interface name, the server never touches iptables, `/etc/styrman/firewall.conf`'s keys are read by nothing (`.agents/docs/deploy/remote.md:33`), and the withholding is deleted (`internal/credbroker/broker.go:36`, with `internal/credbroker/broker_test.go:249` asserting the ssh key IS served). `AGENTS.md`'s Architecture section already holds the correct text; this README paragraph is the last living restatement.
- **[medium / confidence: high]** `.agents/docs/specs/styrman/guide.md:695` — the troubleshooting row `Local network switch disabled: "Not available on this server"` names a UI state that no longer exists (no such string anywhere in `web/src`), an ADR-066 "script below version 4" check, the deleted `/run/styrman-firewall/status` spool file, the deleted `styrman-firewall.path` unit (`deploy/firewall/install.sh:74` removes it on upgrade) and an "Applying… turns into an error after 90 s" state the two-state model cannot produce. The three ADR-067 rows below it are correct; this one should be deleted outright.
- **[medium / confidence: high]** `.agents/docs/testing/styrman/README.md:8` — the Tier 1 inventory still names `internal/config/styrman_harness_test.go` and `internal/config/styrman_ctl_harness_test.go` ("harness channel Phase 1, ADR-073"). Both files were deleted with the channel (ADR-074); neither exists at HEAD. A reader auditing Tier 1 coverage is pointed at tests that cannot run. Same sentence also describes `TestStyrmanHarnessChannel` driving `styrman-update harness`, a verb `deploy/update/styrman-update` no longer has (`specs/styrman/deploy.md:302-303` records it as deleted).
- **[medium / confidence: medium]** `README.md:112` — "**v1.34.1**, deployed" while v1.37.0 has been live on the NUC (and was live on the GCP host) since 2026-09-22 (`changes/2026-09-22-v1.37.0-release.md`). The release list at `README.md:117-129` stops at v1.35.0, so the README's newest entry presents the ADR-073 harness channel as the current delivery mechanism and gives `sudo styrman-ctl update --setup --harness-token-file <file>` as setup advice for a flag that no longer exists (`deploy/README.md:102`, `specs/styrman/deploy.md:303`). Fix: correct the Status version, add the v1.36.0 (ADR-074) and v1.37.0 (GCP target, ADR-075) lines, and mark the v1.35.0 entry's setup command superseded — it reads as an instruction, not as history.
- **[low / confidence: high]** `.agents/docs/testing/styrman/README.md:130-135` — "**Do NOT build or tag `styrman-runtime:dev` on this host until v1.16.1 deploys.** As of v1.16.0, `deploy/.env` on this host still sets `STYRMAN_IMAGE=styrman-runtime:dev`". Twenty-one minor releases stale; compose has refused mutable tags since ADR-053. As written it tells a reader not to run the documented dev build at all.
- **[low / confidence: medium]** `.agents/docs/testing/styrman/adr-067-sweep.sh:56-64` — the completion check for ADR-067's deletions greps `*.go`, `web/src`, `web/e2e`, `*.sh`, `*.yml` and the unit files, but never `*.md`. That is exactly why `STYRMAN_FIREWALL_DIR` still stands in `.agents/docs/deploy/local.md` with the sweep green. Not a stale test (its verdict about code is correct), but its scope does not match the project's describing-text rule; adding `--include=*.md` over the living docs, with the history directories excluded, would have caught the finding above.

### Stale tests

None. Every test suite matches the code it describes: `adr-067-sweep.sh` reports
zero hits by construction, `integration.sh` contains no harness-channel
assertions (`grep -n "harness/current\|channel tag\|styrman-harness-update"`
returns nothing), `deploy/firewall/test.sh` names no spool or `.path` unit, and
`internal/credbroker/broker_test.go:230-249` is the deliberate inverse of the
deleted withholding test.

### Dropped / narrowed

None found. Every rule, flag and caveat retired by ADR-067 and ADR-074 has a
home in a living doc that still states it (`deploy/remote.md:33-44` for the
firewall cleanup, `specs/styrman/deploy.md:301-359` for the channel deletion,
`REQUIREMENTS.md:50-57` for R5's rewrite with the old wording quoted as "was").
The findings above are the inverse case — text kept, behaviour gone.

## `--project`: living-doc classification

| Doc | Verdict | Proof |
|---|---|---|
| `README.md` | **outdated** | `README.md:112` "v1.34.1, deployed" vs `changes/2026-09-22-v1.37.0-release.md:29`; `README.md:52-61` vs `internal/credbroker/broker.go:36`; `README.md:271` vs `image/Dockerfile:150` |
| `AGENTS.md` | accurate | `AGENTS.md:45` carries the `--secret id=gh_token` build; Architecture matches ADR-067/074; every path in the Key Files table exists |
| `REQUIREMENTS.md` | accurate | `REQUIREMENTS.md:50-57` — R5 rewritten for ADR-074, names `HARNESS_VERSION` and the signer, old wording quoted as history |
| `.agents/rules/` (9 files) | accurate | each names a live mechanism: `guest-writable-paths.md` → `internal/files/trust_test.go`, `internal/orchestrator/realbelow_test.go`; `remote-check-names-transport-first.md` → `deploy/gcp/lib.sh` `ssh_reachable` + `TestGCPSSHUnreachableIsNamed` |
| `.agents/skills/`, `.agents/agents/`, `.agents/commands/`, `.agents/workflows/` | empty (nothing to review) | all four directories contain no files |
| `.agents/docs/specs/styrman/guide.md` | **outdated** | `:463` stale build command; `:695` the deleted ADR-065/066 troubleshooting row |
| `.agents/docs/specs/styrman/runtime-image.md` | **outdated** | `:7` build block without the secret, contradicted by `:232` |
| `.agents/docs/specs/styrman/deploy.md` | accurate | `:301-359` records the ADR-073 deletion and the ADR-074 build secret (`:317`) |
| `.agents/docs/specs/styrman/versioning.md` | accurate | `:15` and `:199` both carry `--secret id=gh_token` |
| `.agents/docs/specs/styrman/api.md` | accurate (spot-checked) | every endpoint sampled (`/api/version`, `/api/notifications`, `git/summary`, `git/stash`, `git/discard`, `setup/harvest`, `browser/start`, `links`, `logs`, `diff`, `history`, `stats`) has a handler in `internal/httpapi`; `:15`'s ADR-067 two-state `lanAccess` matches `internal/httpapi/network.go:41` |
| `.agents/docs/specs/styrman/terminal-protocol.md` | unverified | not read this run |
| `.agents/docs/testing/guide.md` | accurate | Tier 1/2 commands match `AGENTS.md` Setup Commands |
| `.agents/docs/testing/styrman/README.md` | **outdated** (counts accurate) | `:8` names two deleted test files; `:128` stale build line; `:130-135` a v1.16.x caveat. Counts verified correct: `:15`'s 115 integration checks = `grep -cE '^[[:space:]]*check ' integration.sh` → 115; `:8`'s "178 tests across 15 files" = `npm test` → `Test Files 15 passed (15)`, `Tests 178 passed (178)` |
| `.agents/docs/deploy/local.md` | **outdated** | `:21` stale build line; `:31`/`:36` `STYRMAN_FIREWALL_DIR`, absent from the code |
| `.agents/docs/deploy/remote.md` | accurate | `:33-44` ADR-067 text with the upgrade cleanup; `:119` the sshd advice stated as advice, matching `AGENTS.md` |
| `.agents/docs/deploy/gcp.md` | accurate | `:85-89` carries the post-tag BUG-072 fix (`styrman-ctl edit` for the optional keys, hostname read back); `:166-168` the teardown of the DNS records added in `5ecc24d` |
| Test suites (`integration.sh`, `adr-067-sweep.sh`, `ensure-test-image.sh`, `web/e2e/`, `*_test.go`) | accurate | see "Stale tests" above; the one scope gap is `adr-067-sweep.sh`'s missing `*.md` |

### `git grep` commands that re-check every stale reference

```bash
# 1. The pre-ADR-074 build command (must be zero hits in living docs)
git grep -n 'docker build -t styrman-runtime' -- README.md AGENTS.md \
  '.agents/docs/specs' '.agents/docs/deploy' '.agents/docs/testing' \
  | grep -v -- '--secret'

# 2. A config variable the code does not have
git grep -n 'STYRMAN_FIREWALL_DIR' -- README.md AGENTS.md REQUIREMENTS.md \
  '.agents/rules' '.agents/docs/specs' '.agents/docs/deploy' '.agents/docs/testing'

# 3. ADR-065/066 machinery presented as current
git grep -nE 'styrman-firewall\.path|/run/styrman-firewall|firewall\.conf|Not available on this server|script below version 4' \
  -- README.md AGENTS.md '.agents/docs/specs' '.agents/docs/deploy' '.agents/docs/testing'
#    (deploy/remote.md's "REMOVES the ADR-065/066 machinery" lines are the
#     legitimate hits; anything else in a living doc is stale)

# 4. ADR-073 harness channel presented as current
git grep -nE 'harness-token-file|harness/current|styrman-harness-update|styrman-update harness|image/harness/' \
  -- README.md AGENTS.md REQUIREMENTS.md '.agents/docs/specs' '.agents/docs/deploy' '.agents/docs/testing'
#    (hits that say "deleted"/"superseded"/"was" are fine)

# 5. Deleted test files still named as part of a suite
for f in $(git grep -ohE 'internal/[a-z]+/[a-z_]+_test\.go' -- '.agents/docs/testing' | sort -u); do
  git ls-files --error-unmatch "$f" >/dev/null 2>&1 || echo "NAMED BUT ABSENT: $f"
done

# 6. The SSH-key withholding, removed by ADR-067
git grep -niE 'withhold|stops being served the operator' -- README.md '.agents/docs/specs' '.agents/docs/deploy'

# 7. The README's deployed version against the newest release entry
git grep -n '^\*\*v1\.' -- README.md; ls -t .agents/docs/changes/*release.md | head -1

# 8. Count claims (run bare, read the status)
grep -cE '^[[:space:]]*check ' .agents/docs/testing/styrman/integration.sh   # must equal the README's 115
(cd web && npm test)                                                        # must equal "178 tests across 15 files"
```

## Criteria
1. Dead code — n/a — docs lens
2. Flow tracing — n/a — docs lens
3. Standards conformance — n/a — docs lens
4. Directory layout — n/a — docs lens
5. Simplicity (DIY-except-security) — n/a — docs lens
6. Decoupling — n/a — docs lens

## Not examined

- `.agents/docs/specs/styrman/terminal-protocol.md` (217 lines) — not read; its
  claims about the WS protocol are unverified either way.
- `.agents/docs/specs/styrman/api.md` — spot-checked only (12 of ~70 endpoints
  confirmed to have a handler); request/response SHAPES were not compared
  against the Go structs at all, except `lanAccess`.
- `.agents/docs/specs/styrman/deploy.md` (877 lines) and `guide.md` (731 lines)
  — read by targeted grep (build commands, ADR-065/066/073 references, the
  troubleshooting table), not end to end. Statements outside those greps are
  unverified.
- `.agents/docs/deploy/gcp.md` and `remote.md` — checked against the newest
  `changes/` entry and the scripts' flags, not by executing anything. No remote
  or GCP command was run.
- The e2e suite (`web/e2e/`) — its check count and its assertions were not run
  or read beyond a grep for stale identifiers; the integration suite was counted
  but not executed (it needs real Kata).
- `image/README.md` and `deploy/README.md` are not in the dispatch's living-doc
  list; they were read only as corroboration and are not classified.
