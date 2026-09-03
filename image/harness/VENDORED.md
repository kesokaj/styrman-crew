# image/harness — a mirror of styrman-harness

This directory is a verbatim copy of the `styrman-harness` repository
(`~/workspace/styrman-harness`, private) at the commit in `COMMIT`. Nothing in
it is styrman's own; every file is edited in that repository and re-mirrored.

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

`SHA256SUMS` is what makes the mirror checkable without the private
checkout: `deploy/verify-commit.sh` runs `sha256sum -c` over it in the
commit's own worktree (a partially staged refresh fails there, not at
release time), builds the module, builds the runtime image, and compares the
skill and subagent counts `sh-agent list` reports inside the image with the
commit's tree. `COMMIT` remains an attestation by whoever refreshed — the
`diff -rq` above, run against the checkout at that SHA, is its only check.

Then bump `STYRMAN_VERSION` — the version stamp is the only thing that makes
a running project re-seed (ADR-046). Record the new `COMMIT` in the release's
entry in `.agents/docs/specs/styrman/versioning.md`.

## Known cosmetic gap

`sh-agent` records the repo commit by running `git rev-parse` in `--repo`; the
mirror has no `.git`, so the manifest's `commit` field is empty and
`sh-agent status` inside a project prints a blank commit. `COMMIT` here is
the record. `sh-agent update` is not used inside projects (D4).
