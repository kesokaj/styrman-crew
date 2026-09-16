# image/harness — a mirror of styrman-harness

This directory is a verbatim copy of the `styrman-harness` repository
(`~/workspace/styrman-harness`, private) at the commit in `COMMIT`. Nothing in
it is styrman's own; every file is edited in that repository and re-mirrored.

## Superseded as the delivery path (ADR-073)

Since ADR-073 this mirror is no longer how a `styrman-harness` release
normally reaches a project: the host's harness channel
(`styrman-update harness`) delivers the newest signed tag directly, and the
server bind-mounts it into every project VM. This mirror is now the
FALLBACK the entrypoint seeds from when the server mounted no channel (no
channel delivered yet, or one it refused). The refresh procedure below is
unchanged and still what a crew release records
(`image/harness/COMMIT`).

What the image does with it (ADR-062, superseding ADR-046's source and
ADR-047's PATH trim):

- `image/Dockerfile` builds `cmd/sh-agent` from this tree in a builder stage
  and ships the binary as `/styrman/bin/sh-agent`; the tree itself lands at
  `/styrman/harness/`.
- `image/entrypoint.sh` seeds every project home with
  `sh-agent install --repo /styrman/harness --home "$HOME"` (write-once, keyed
  on `~/.styrman/.harness-version`), after `image/configs/harness-migrate.sh`
  has moved the artifacts of the previous harness aside. Styrman-side
  workarounds belong there, at the call site, never in this tree.

## This file is under SHA256SUMS too

`SHA256SUMS` lists every file in this directory except itself — `COMMIT`
and this note included — and `deploy/verify-commit.sh` refuses a release
whose mirror does not match it. Editing this file (or `COMMIT`) means
re-running the `sha256sum` line of the recipe below in the same change
(BUG-065: the v1.35.0 docs sweep edited this note and the release's own
verify-commit caught the stale sums).

## Refresh

```sh
H=~/workspace/styrman-harness
test -z "$(git -C "$H" status --porcelain)"   # a dirty checkout is not a commit
rsync -a --delete --exclude .git --exclude .agents --exclude .gitignore \
      --exclude /COMMIT --exclude /VENDORED.md --exclude /SHA256SUMS "$H/" image/harness/
git -C "$H" rev-parse HEAD > image/harness/COMMIT
(cd image/harness && find . -type f ! -name SHA256SUMS | sed 's#^\./##' | LC_ALL=C sort | xargs sha256sum > SHA256SUMS)
diff -rq --exclude .git --exclude .agents --exclude .gitignore "$H" image/harness
# expected: exactly three lines, "Only in image/harness: COMMIT", "... SHA256SUMS", "... VENDORED.md"
```

The mirror's own `go test ./...` is not a check here: `deploy/verify-commit.sh`
vets the mirror (SHA256SUMS, build, image counts) and never runs its tests.
It used to be red by construction — `TestDryRunNeverBuilds` asserted the
checkout-shaped dry-run output and failed in any plain copy (BUG-058 here,
BUG-016 upstream) — until the harness gave that test its own
checkout-shaped fixture (2026-09-14); a failure there now is a real one.

`SHA256SUMS` is what makes the mirror checkable without the private
checkout: `deploy/verify-commit.sh` runs `sha256sum -c` over it in the
commit's own worktree (a partially staged refresh fails there, not at
release time), builds the module, builds the runtime image, and compares the
skill and subagent counts `sh-agent list` reports inside the image with the
commit's tree. `COMMIT` remains an attestation by whoever refreshed — the
`diff -rq` above, run against the checkout at that SHA, is its only check.

Then cut a release — the image version stamp is the only thing that makes
a running project re-seed (ADR-046). The release's entry in
`.agents/docs/changes/` names the new `COMMIT` (full SHA), as the release
procedure in `.agents/docs/specs/styrman/versioning.md` says.

## A plain copy, by design

The mirror has no `.git`, and `sh-agent` decides what a tree is by exactly
that (`harness.IsCheckout`: a `.git` entry directly in `--repo`), never by
asking git. So inside the image `install` takes the plain-copy path: it
installs the running binary as is, never runs `go build`, never runs git,
and prints `plain copy, not its own git checkout: the running binary is
installed as is` (BUG-015 in the harness, fixed in 2c8990e — the first
harness commit that installs cleanly from a read-only, git-less mirror). The
manifest's `commit` field is empty for the same reason, there being no
checkout to read one from, and `sh-agent status` in a project prints
`repo /styrman/harness (plain copy, not its own git checkout; update is
refused)`. `COMMIT` here is the record; `sh-agent update` is not used inside
projects (D4).
