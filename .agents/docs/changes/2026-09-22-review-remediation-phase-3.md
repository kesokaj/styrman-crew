---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 3 — hardening that did not travel   (2026-09-22)

Phase 3 of `plans/2026-09-22-project-review-remediation.md`: findings 4, 5, 6
and the lock half of 9 from `reviews/2026-09-22-review-project-merged.md`.

## `deploy/gcp/install.sh` stops forking two helpers from `styrman-ctl`

`env_get` and `private_file` now live in `deploy/gcp/lib.sh`, which
`install.sh` already sources.

**`env_get` was unambiguous drift.** The fork was
`grep -E "^$2=" "$1" | tail -n1` — no `is_key` guard, and the key
interpolated into a regex. The hardened version matches the key literally as
an awk VARIABLE, which is what an earlier security review put there after a
key containing a slash changed a `sed` program. Every caller passes a literal
today, so there was no live exploit; the point is that the next caller need
not be careful.

**`private_file`'s two divergences are not drift, and are now parameters.**
The plan required deciding which side is right for each rather than taking the
stricter and calling it merged:

- *Owner.* `styrman-ctl` accepts `root`, `${SUDO_USER}` or the invoking user
  because it runs under `sudo`; `install.sh` accepted only the invoking user
  because it runs as the operator. Both correct for their context →
  `OWNER_POLICY` argument, `self` by default.
- *Parent directory mode.* `styrman-ctl` refuses a parent intersecting `022`,
  `install.sh` refused `077`. **`022` is the security-correct check**: a
  readable directory does not expose a 600 file, whereas a WRITABLE one lets
  the file be replaced. `077` is stricter and right for a directory that
  exists only to hold secrets, which is what `~/.config/styrman/gcp` is, and
  wrong for `/etc/styrman`, which is legitimately 755. → `PARENT_MASK`
  argument, `077` by default.

Both defaults are chosen so `install.sh`'s behaviour is **unchanged**: this
phase fixes `env_get`'s hardening, it does not quietly tighten or loosen the
file checks a working deploy depends on.

**The duplication against `styrman-ctl` remains, deliberately.** `styrman-ctl`
is the general host tool and `deploy/gcp/lib.sh` is GCP-specific; making the
former source the latter inverts the dependency. Two copies that have already
drifted once will drift again, so `TestGCPLibEnvGetMatchesStyrmanCtl` asserts
both still carry the literal-match awk program and the `is_key` guard, and
that `install.sh` has not grown the fork back — the same shape as
`TestFirewallScriptMatchesBridgePrefixes`.

## `X-Caddy-Role` is stripped from the client request

`deploy/caddy/ingress/generator.go`: in role-gated mode the role is set only
inside the three `header_regexp` matchers, so a request matching none of them
carried the client's own header to the upstream — unlike every other identity
header on that path, each stripped or unconditionally overwritten. Nothing
reads it today, which is why it is cheap now and would read as an upstream
authorization bug the day something does. Authenticated mode sets the header
unconditionally and needs no strip; a second test asserts that, so the fix
does not drift into "strip everywhere".

## `credMu`

`internal/httpapi/server.go`: `credharvest.go`'s `credPushing` and
`credPushRerun` had shared `deletingMu` with the project-lifecycle maps — two
unrelated features on one lock, serialising a credential push against a
project delete for no reason. Eight call sites moved. The file split that the
same finding also suggested is deliberately NOT done: no lens has read
`server.go` for cohesion, and refactoring an unread 1,405-line file is how a
defect hides.

## Proof

Both fixes were seen red before being believed, each mutation compiling first
and naming a failing test, each file restored from a checksummed copy and
verified byte-identical:

| Mutation | Named failure |
|---|---|
| `lib.sh`'s `env_get` put back to the fork's `grep -E` form | `TestGCPLibEnvGetMatchesStyrmanCtl` ("the two copies have drifted") and `TestGCPLibEnvGetRefusesANonKey` |
| the `request_header -X-Caddy-Role` line removed | `TestRoleGatedStripsClientSuppliedRole` |

`TestGCPLibEnvGetRefusesANonKey` drives the real `lib.sh` through a probe
script with `a/b`, `BASE_DOMAIN/x`, `lower`, `WITH-DASH`, `$(id)` and `*`, and
asserts a valid key still resolves — so the guard is not passing by refusing
everything. `TestGCPLibPrivateFileDefaultsAndPolicies` pins both defaults, the
`022`/`077` difference, a group-writable parent, a group-readable file, an
unknown owner policy and a symlink.

## Verification

Bare, status read each time:

- `go vet ./...` → 0
- `go test ./...` → 0 (327 pass, 0 fail, 1 skip; 324 before this phase)
- `go test -race ./internal/httpapi/` → 0 — the lock split's own check
- `(cd deploy/caddy/ingress && go test ./...)` → 0
- `bash -n` and `shellcheck -S warning` over `deploy/gcp/*.sh` → 0

Not run: Tier 2, Tier 4, container builds — Phase 5.

## Learnings for the local host

The `env_get` hardening now reaches the GCP scripts, and the drift guard means
a future change to `styrman-ctl`'s copy that forgets the other one fails
`go test ./...` on any machine, not only on a host that runs both.
