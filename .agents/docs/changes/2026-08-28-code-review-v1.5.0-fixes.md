# Code review of v1.5.0 (e20f79f..HEAD) — findings + fixes

Date: 2026-08-28. Formal `/code-review` pass over the per-project-homes +
credential-broker changeset (ADR-031/032), including the pre-commit
adversarial-review fixes which had not themselves been re-reviewed.

## Findings

### 🔴 blocking — image upgrades never reached existing containers
`EnsureRunning` recreated a container only for the pre-ADR-027 workspace-bind
migration — never for image drift. A container built from an older
`styrman-runtime` build kept its old rootfs across stop/start **forever**, so
new `/etc/profile.d` snippets (the v1.5.0 credential materialization!), tools,
and entrypoint fixes silently never reached existing projects. The operator's
live project was on the pre-v1.5.0 image; the docs (ADR-028) already assumed
an "image upgrade" recreate path that didn't exist in code.
**Fix** (`orchestrator.go`): when a container is **stopped** and its image ID
differs from the current `cfg.Image` ID, remove + recreate it on the next
boot. Running containers are never touched (recreating one would kill live
sessions) — they upgrade on their next stop/start cycle. Safe because home +
workspace are mounts and the apt manifest (ADR-028) replays.
**Proven live**: no-drift restart reuses the container (no churn); with drift
the container is recreated in both directions (old→new, new→old) with log
evidence, and the upgraded rootfs carries the credential snippet.

### 🟢 nit — corrupt store file rendered a ghost credential row
`credstore.List` ignored `json.Unmarshal` errors, so a corrupt/empty store
file yielded a zero-value item (empty ID) the UI rendered but could never
delete. **Fix**: skip entries that fail to parse or have an empty ID; test
added.

### 🟢 nit — no length bound on a pasted env value
`PutEnv` accepted values up to the global 1 MiB body cap; a paste mistake (a
whole file as the "value") would be exported into every login shell. **Fix**:
4096-byte cap (real tokens are well under 1 KiB); test added.

## Reviewed and found sound (no action)
- `Exec` is context-bounded with the hijacked-conn unblock pattern — a hung
  in-guest command during credential import cannot leak the goroutine.
- Request bodies are capped (1 MiB default) including the new credential
  routes; path values are validated per kind (host regex / env identifier /
  no separators) so no traversal via `DELETE .../{kind}/{name}`.
- The cache-hit re-verify (`containerHoldsIP`) is correct for DinD: nested
  containers NAT out behind the VM's bridge IP, so the source IP is always
  the project container's own address (live-verified).
- The login snippet's env delivery is protected end-to-end against
  control-character injection (rejected at `PutEnv`, re-checked in `AllEnv`,
  key re-validated in-shell).
- Settings adds two polls (credentials 20 s, projects 15 s) on the Settings
  page only — noted against the existing poll-amplification TODO; acceptable.

## Verification
go test -race clean; Kata integration 62/62 after the orchestrator change;
live drift-upgrade proof (above); throwaway projects cleaned up; the
operator's project untouched (it auto-upgrades on its next stop/start).
