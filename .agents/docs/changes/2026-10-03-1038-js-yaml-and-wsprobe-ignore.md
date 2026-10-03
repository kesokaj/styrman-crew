---
owner: main
date: 2026-10-03
---
# js-yaml 4.3.2; the `wsprobe` ignore rule anchored   (2026-10-03)

The operator: "Js yaml and gitignore", then "Yepp". Two small follow-ups
carried since v1.40.0.

- **js-yaml 4.3.1 → 4.3.2** (`web/package-lock.json` only). It came in through
  `@tanstack/react-start` → `@tanstack/start-plugin-core` → `xmlbuilder2` and
  carried GHSA-2883-xcg3-v3hh. It is used at build time only. The update was
  `npm audit fix`, without `--force`: the patched version is within
  xmlbuilder2's own range. The lockfile diff is that one package's version,
  its `resolved` URL (registry.npmjs.org) and its integrity hash. `npm audit`
  now finds 0 vulnerabilities. Web check, lint and 211 tests pass. A fresh
  `npm run build` reproduced `web/dist/client` exactly, so the shipped bundle
  is unchanged and no release is needed.
- **`.gitignore`: `wsprobe` → `/wsprobe`.** The unanchored rule also matched the
  source directory `cmd/wsprobe/` (`git check-ignore` named it for
  `cmd/wsprobe/newfile.go`), so a new file there would have been silently left
  out. It now ignores only the binary at the repository root, and
  `git check-ignore` confirms both.

Review: the main agent read the diff (5 lines across 2 files): below the
reviewer threshold. Dependency check: the lockfile diff above, plus `npm audit`.

## Learnings for the local host
None.
