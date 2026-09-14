---
owner: bugs
date: 2026-09-14
id: BUG-058
status: open
area: image/harness (vendored styrman-harness mirror; upstream ~/workspace/styrman-harness)
---
# BUG-058: vendored harness's own `go test ./...` fails — test assumes a git-checkout tree shape the plain-copy mirror doesn't have

## Symptom
`(cd image/harness && go test ./...)` prints `FAIL
github.com/kesokaj/styrman-harness/cmd/sh-agent` — `TestDryRunNeverBuilds`
(image/harness/cmd/sh-agent/main_test.go:211) expects `[dry-run] the binary
would be rebuilt from` and gets `[dry-run] plain copy, not its own git
checkout: the running binary would be installed as is`; `internal/harness`
passes. The same test passes in `~/workspace/styrman-harness`
(`go test ./cmd/sh-agent/` → ok).

## Repro
`(cd image/harness && go test ./...)` in this repo → FAIL as above.
`(cd ~/workspace/styrman-harness && go test ./cmd/sh-agent/)` → ok.

## Root cause
`TestDryRunNeverBuilds` runs `install --dry-run` against the package's own
repo root and assumes that root is a git checkout (a `.git` entry beside
it). The vendored mirror at `image/harness` is a plain copy with no
`.git`, so since the harness's BUG-015 fix (harness commit 2c8990e, gating
the rebuild on `harness.IsCheckout`) the dry-run legitimately takes the
plain-copy branch there — the test encodes the tree shape it happened to
be written in rather than constructing it.

Not a crew defect and not in the crew gate: `deploy/verify-commit.sh` runs
`sha256sum -c` and `go vet ./...` against the mirror, never its test
suite; the image builds the binary without running tests; the mirror is
refreshed verbatim and never edited here (AGENTS.md § Project-Specific
Standards).

Self-inflicted note on the same record: this result was first read through
`go test ./... 2>&1 | tail -3` followed by `echo rc=$?`, which printed
`rc=0` — the pipeline's status was `tail`'s, not `go test`'s (project rule
`.agents/rules/verify-bare.md`, BUG-033/041). The FAIL line was visible in
the tail output, which is the only reason it was caught rather than missed.

## Fix
None in this repo. Carried to the operator for the upstream harness repo:
the test should construct its own shape (a temp dir with a `.git` entry,
or a case that mocks `IsCheckout`), or skip explicitly when run against a
tree where `IsCheckout` is false.

## Prevention
A vendored dependency's own test suite is not this project's gate — its
result is reported as an observation, not folded into a PASS/FAIL of the
change under test. A test asserting behaviour that depends on tree shape
(checkout vs. plain copy) must construct that shape itself, not inherit it
from wherever the package happens to be checked out.

## Related
`.agents/rules/verify-bare.md` (BUG-033/041) — the same session's read of
this failure went through the exact anti-pattern that rule exists to stop;
worth noting if a third instance turns up, since the rule already exists
and was still nearly missed.
