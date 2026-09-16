---
owner: bugs
date: 2026-09-16
id: BUG-065
status: fixed
area: image/harness (vendored styrman-harness mirror)
---
# BUG-065: `image/harness/VENDORED.md` edited without regenerating `SHA256SUMS`, failing the release gate's `verify-commit.sh`

## Symptom
At the v1.35.0 release gate, `deploy/verify-commit.sh 6e2b0d8` (the required
pre-tag step, versioning.md step 2) exited 1 at its "vendored harness
mirror (image/harness: integrity, build, toolchain pin)" step:
`sha256sum -c --quiet SHA256SUMS` printed `VENDORED.md: FAILED`. Every
earlier gate row (four verifier logs, Tier 1 and Tier 2 four times) was
green — nothing in the test tiers runs `sha256sum -c` on the mirror; only
`verify-commit.sh` does, and it ran last, on the release commit.

## Repro
`git show 6e2b0d8:image/harness/VENDORED.md | sha256sum` differs from the
`VENDORED.md` line in `git show 6e2b0d8:image/harness/SHA256SUMS`;
`deploy/verify-commit.sh 6e2b0d8` exits 1 with `VENDORED.md: FAILED`.

## Root cause
`image/harness/SHA256SUMS` covers every file in the mirror directory
except itself, `VENDORED.md` and `COMMIT` included. The docs sweep commit
3ede401 (the `documentation` agent, dispatched with `VENDORED.md` in its
file list) added a "Superseded as the delivery path (ADR-073)" note to
`VENDORED.md` without regenerating the sums — a checksummed file was
edited as if it were plain prose. `VENDORED.md`'s own Refresh recipe
regenerates the sums, but nothing said the file ITSELF is under them, and
the dispatch that edited it did not know either. Same class as the
"describing text" rule from a different angle: the sums describe the
file's content, and an edit to the content that leaves the description
behind is caught only where the description is checked — here at the last
step of the gate.

## Fix
Commit after 6e2b0d8 (the commit the v1.35.0 tag is placed on):
`image/harness/SHA256SUMS` regenerated with the recipe's own line (`find .
-type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort | xargs
sha256sum > SHA256SUMS`), `sha256sum -c` clean, the list diff empty, no
symlinks; a section "This file is under SHA256SUMS too" added to
`VENDORED.md` above the Refresh recipe naming BUG-065; `verify-commit.sh`
re-run on the fix commit before the tag.

## Prevention
`image/harness/` is a checksummed tree — any edit under it, `VENDORED.md`
and `COMMIT` included, regenerates `SHA256SUMS` with the recipe in
`VENDORED.md` in the same change, and a `documentation` dispatch that
names a file under `image/harness/` says so in its brief;
`deploy/verify-commit.sh` is the only check that catches it, so run it (or
`cd image/harness && sha256sum -c --quiet SHA256SUMS`) before a commit
that touches the mirror, not only at the tag.

## Related
`.agents/rules/changelog-brief.md` (closed-fact-list dispatch rule; this is
the same shape one level down — a brief that names a file must also name
the invariant that file carries). Not yet a third occurrence of any
existing three-record family; if a similar checksummed/generated-file edit
recurs, the rule belongs in `image/harness/VENDORED.md` itself (already
added) or a `.agents/rules/` file.
