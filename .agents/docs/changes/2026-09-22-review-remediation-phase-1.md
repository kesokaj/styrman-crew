---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 1 — docs truth   (2026-09-22)

Phase 1 of `plans/2026-09-22-project-review-remediation.md`, acting on the docs
lens of the whole-project review
(`reviews/2026-09-22-review-project-docs.md`, REQUEST CHANGES) plus two
statements Phase 0b's own behaviour change made stale.

Documentation only. No source, test or build file changed; the one non-doc file
is `deploy/.env.example`, and every changed line in it is a comment
(`host-setup.sh` copies that file to `deploy/.env`, so an added or removed
variable would have reached a real host — checked, none was).

## What was wrong, and what it says now

- **Five copies of the runtime-image build command failed as written.**
  ADR-074 made the GitHub token a required build secret
  (`image/Dockerfile:150`, `required=true`) and `AGENTS.md` was updated; the
  five other copies were not. Fixed in `README.md`,
  `deploy/local.md`, `specs/styrman/guide.md`,
  `specs/styrman/runtime-image.md` and `testing/styrman/README.md`, each
  keeping its own tag and surrounding prose.
- **A deleted variable was still documented and still passed.**
  `deploy/local.md` had a `STYRMAN_FIREWALL_DIR` row in its Configure table
  and passed it in its Run command; ADR-067 deleted that spool. Row and flag
  gone. `deploy/.env.example` claimed the variable "is set by
  docker-compose.yml" — compose has no firewall reference at all, and
  `git grep` outside the docs returned that line alone. It now says the
  variable does not exist and why.
- **`README.md` described a network design that was deleted two releases ago:**
  the ADR-065/066 root-mediated LAN allowance, complete with a per-project
  allowance applied "within about a second" and a LAN project losing the
  operator's SSH key — whose removal `internal/credbroker/broker_test.go:249`
  asserts. Replaced with the ADR-067 model `AGENTS.md` already stated
  correctly: the mode is the Docker bridge interface name, the server never
  touches iptables, and cross-project isolation is Docker's own unconditional
  per-bridge DROP.
- **The Status line said v1.34.1 while v1.37.0 is live**, and the release list
  stopped at v1.35.0. v1.36.0 and v1.37.0 added; the v1.35.0 entry, which
  describes the ADR-073 harness channel, is now marked superseded by ADR-074
  and its `styrman-ctl update --setup --harness-token-file` setup step
  explicitly retired — it had been sitting there as something to run.
- **`testing/styrman/README.md` named two test files that no longer exist**
  (`internal/config/styrman_harness_test.go`,
  `internal/config/styrman_ctl_harness_test.go`, deleted with ADR-073's
  mechanism) and carried a v1.16.x caveat telling the reader not to build
  `styrman-runtime:dev` "until v1.16.1 deploys". Both gone.
- **`specs/styrman/api.md` did not describe today's own behaviour change.**
  `GET .../files` now documents the 5000-entry cap, that it is applied to raw
  directory order before sorting so which entries are cut is arbitrary, and
  the `truncated` field beside `entries`; the delete and rename rows document
  the symlink-traversal refusal added in Phase 0b. Not a review finding — the
  review predates the change; found while assembling the brief, which is what
  the describing-text rule asks for.

## What the re-grep caught that the lens had not

The plan required re-running the docs lens's own eight `git grep` commands
after the fixes rather than trusting them done. Three hits came back that the
lens had not reported:

- **Real:** `specs/styrman/deploy.md:51` described what `styrman-ctl install`
  runs as `docker build -t styrman-runtime:${V} --build-arg
  STYRMAN_VERSION=${V} image/` — without the secret. `deploy/styrman-ctl:490-496`
  assembles `secret_args` from `--gh-token-file`, else `GH_TOKEN`, else
  `/etc/styrman/gh-token`, and `die`s when it has none. The line now says so.
  Fixed by the main agent, not the `documentation` agent, because it was found
  after that dispatch had returned.
- **False positive:** `specs/styrman/versioning.md:199` looks like a bare build
  command to a line-based grep; `--secret id=gh_token,src=/etc/styrman/gh-token`
  is on the next line. Correct as written.
- **False positive:** `testing/styrman/README.md:142` names
  `internal/netpolicy/script_test.go`, which is indeed deleted — in a sentence
  saying it "is gone with `internal/netpolicy/` itself" and naming its
  replacement, `internal/config/firewall_script_test.go`. Describing a
  deletion is not a stale claim.

Greps 2 (`STYRMAN_FIREWALL_DIR`) and 6 (SSH-key withholding) return only
legitimate hits: the Tier 4 paragraph explaining what ADR-067 replaced, the
`adr-067-sweep.sh` script that greps for those names to assert their absence,
and three living-doc sentences that state the withholding was removed.

## One correction to the agent's draft

The `documentation` agent was given a closed fact list
(`.agents/rules/changelog-brief.md`). It followed it, and while deleting the
expired v1.16.x caveat it also repaired two cross-references that pointed at
the deleted block — correct, and within scope, since a pointer to deleted text
is itself stale. But one of the two replacements invented a fact: it wrote
"never `:dev`, the production tag", which restates the very claim the caveat
was deleted for. `:dev` stopped being the production tag when v1.16.1 shipped;
production boots `styrman-runtime:X.Y.Z` and compose refuses to run without
`STYRMAN_VERSION`. Corrected to a bare "(default `styrman-runtime:test`)"
before staging. **Corrections needed to agent drafts: 1.**

This is the tenth such correction in the window the rule records, and the first
where the invented fact was the negation of the sentence being deleted.

## Verification

Docs-only, so the project's suites were not re-run for behaviour; what was run,
bare, with its status read:

- `(cd deploy/caddy/ingress && go test ./...)` → 0 (its `loopback_test.go`
  reasons about `.env.example`)
- `bash -n deploy/styrman-ctl` → 0
- Every changed line of `deploy/.env.example` confirmed to be a comment
- The docs lens's greps 1, 2, 5 and 6, re-run after the fixes, output above

Not run, and not claimed: Tier 2, Tier 4, container builds. No behaviour
changed in this phase.

## Learnings for the local host

None: no deploy path, script or image content changed. The `deploy.md:51` fix
describes `styrman-ctl` more accurately but does not alter it.
