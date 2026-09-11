---
owner: main
date: 2026-09-11
---
# Preflight — v1.28.0   (2026-09-11)

Range reviewed: v1.27.1..b021cfc

**VERDICT: SHIP**

Base: tag `v1.27.1` (release commit `4686792`). Head at gate time: `b021cfc`
(28 commits, 124 files, +12161/-69 before the gate artifacts). Version:
**v1.28.0, MINOR** — new behaviour for a user (the Browser view) and for an
operator (`styrman-ctl`); nothing an existing deployment must react to.
Trigger: the operator's own "ship it" (this turn).

## Findings (FAIL-first; none blocking)

| # | Severity / confidence | Finding | Disposition |
|---|---|---|---|
| 1 | low / high | The tagged tree will be `b021cfc` plus this gate's artifacts (docs only). `deploy/verify-commit.sh` ran twice: on `4c0869b` (OK) and on `b021cfc` (OK, `.agents/scratch/verify-commit-v1.28.0-final.log`) — the commit the tag's source state comes from. Docs cannot change the build. | accepted, as in every prior release (tag on the release commit, gated commit named) |
| 2 | low / medium | Two operator-only steps are not part of any automated tier and stay open after the release: the manual agent walk in the Browser view, and a fresh-host `styrman-ctl install` on a Debian 13 VM. | accepted; ROADMAP "Now" and both plans say so |
| 3 | info | The host carries ≈90 `styrman-*` image tags from earlier releases and verification builds (observation from the `uninstall --level 3` dry-run); housekeeping, not this release's. | noted |

## Step 2 — reviewer / security / verification

- **Reviewer**: every source commit in the range is covered by a prior report — browser phases 1–4 (`reviews/2026-09-11-review-browser-phase{1,2,3,4}.md`: REQUEST CHANGES → fixed, APPROVE WITH NITS ×2, APPROVE), styrman-ctl phases 1–4 plus the post-review range (`reviews/2026-09-11-review-styrman-ctl-{phase1,phase2,phase3,phase4,final}.md`: two REQUEST CHANGES, three APPROVE WITH NITS — every finding fixed or rebutted in writing in the phase testing logs). Commit `4c0869b` (a ten-line marker relaxation and its test) was below threshold and read by main. Commit `b021cfc` (the final security round's fixes) was not re-reviewed by an agent: it was tested bare (15 tests, shellcheck, vet) and its real dry-runs on the host were read; its content answers, point by point, the final security report — listed as finding 1's residual.
- **Security**: nine reports (`reviews/2026-09-11-security-browser-phase{1,2,3,4}.md`, `reviews/2026-09-11-security-styrman-ctl-{phase1,phase2,phase3,phase4,final}.md`). Highs found and fixed in-range: the shareable DevTools port, `--remote-allow-origins`, the awk escape bypass in `env_set`, the wrong-rule chain deletes; every medium fixed or accepted with a written reason (`testing/2026-09-11-verify-browser-phase4.md`, `testing/2026-09-11-verify-styrman-ctl-phase4.md`). Surfaces touched: auth (owner-only routes), secrets (token, signers, `.env`), input parsing (CDP payloads, `.env` values, iptables rules), dependencies (chromium, socat, chrome-devtools-mcp pinned).
- **Verification**: verifier logs `testing/2026-09-11-verifier-browser-phase{1-2,1-3,4}.md` and `testing/2026-09-11-verifier-styrman-ctl-{phase1,phase2,phase3,phase4,final}.md` — all PASS; the last browser one at the README's exact counts (integration `passed=90 failed=0 skipped=3`, e2e `passed=91 failed=0 skipped=2`); `deploy/verify-commit.sh b021cfc` OK.

## Step 3 — trail freshness

- Newest `changes/` entries describe this range (browser phases 1–4, styrman-ctl phases 1–4; the release entry is written with this gate): pass.
- No future `date:` under `.agents/docs`: pass.
- `changes/` and `DECISIONS.md` append-only since the base (`git diff` shows zero removed lines in either; the two "modified" commits appended ADR-068 and its amendment): pass.
- `ROADMAP.md` touched five times since the base, `TODO.md` empty by design: pass.

## Step 4 — docs freshness

`README.md`, `.agents/docs/specs/styrman/{guide,api,terminal-protocol,runtime-image,deploy}.md` and `.agents/docs/deploy/{remote,local}.md` were updated by the `documentation` agent for both features and read back against the code; `REQUIREMENTS.md` R4 carries the approved sentence; `deploy/README.md` has the `styrman-ctl` quick start: pass.

## Secret scan

No `gitleaks` on this host; the staged release diff is scanned for `PRIVATE KEY`, `-----BEGIN`, `AKIA`, `AIza`, `ya29.`, `token=`, `password=` before the release commit (result recorded in the handoff). No `.env*`, key or credential path is in the range.

## What was not checked

- The live relay and the agents' MCP path on the containerized deploy: proven on this host with a host-run server against a LAN-mode project (hand-walk and e2e), not against the compose stack — the deploy check after the tag (`styrman-ctl status`, a Browser view opened on production) is the first run there.
- A real `styrman-ctl install`/`uninstall` (every run on the production host was `--dry-run` or `status`).
- `govulncheck` beyond the security agents' runs (two pre-existing `docker/docker` advisories with no fix published); `npm audit` does not cover the global `chrome-devtools-mcp` install.
