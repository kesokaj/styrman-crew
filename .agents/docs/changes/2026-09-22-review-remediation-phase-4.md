---
owner: main
date: 2026-09-22
---
# Review remediation, Phase 4 — the four decisions   (2026-09-22)

Phase 4 of `plans/2026-09-22-project-review-remediation.md`: findings 7, 10, 11
and 15, each of which needed the operator's call rather than mine. The operator
answered "go with your recommendations" (2026-09-22), and the recommendations
are recorded per item in `.agents/scratch/2026-09-22-review-triage.md`.

## Finding 7 — the R12 `ip_forward` assertion no longer fails open

A containerized server asserts `net.ipv4.ip_forward=0` at boot because it
joins every project bridge, and forwarding-on would void R12. Every error path
refused — except `fs.ErrNotExist`, which warned and continued, on the
reasoning that an exotic kernel should not take the service down. That made
the one guard standing behind R12 report "fine" in the single case where it
had measured nothing: BUG-014/028/029/031's shape.

A missing sysctl file now refuses like any other unreadable state. Inside a
Linux container that file is effectively always present, so its absence is a
reason to look rather than to proceed, and `STYRMAN_ALLOW_IP_FORWARD=1`
remains the operator's documented way through — the same escape the
forwarding-on case has always had.

The rule moved into `config.IPForwardMustRefuse(off, err, override)` because
it is a security decision and `main()` is not reachable from a test.
`TestIPForwardMustRefuse` covers all eight combinations, including an error
arriving alongside `off=true` (the bool is meaningless when the read failed).
`cmd/styrman/main.go` keeps its distinct log messages and now asks that
function whether to exit.

**Proof:** restoring the `fs.ErrNotExist` fail-open compiles and fails
`TestIPForwardMustRefuse` by name on the "missing sysctl file: refuse" case;
`internal/config/ipforward.go` was then restored from a checksummed copy and
verified byte-identical.

## Finding 10 — the converge code is kept, and now says so

~125 lines across `deploy/update/styrman-update` (`retire_harness_units`),
`deploy/firewall/install.sh` §2 and `deploy/firewall/styrman-firewall.sh`'s
one-shot cleanup converge away artifacts of ADR-065/066 and ADR-073. The
analysis that they have no reachable consumer is not in doubt: the NUC has
been past them since v1.27.0, a host created by `deploy/gcp/install.sh` never
had the artifacts, and the only host that could still need them is one frozen
between v1.23 and v1.35 that upgrades later.

The recommendation was to keep them now and delete them once the GCP target is
used in anger, because deleting converge code is irreversible for exactly that
host while keeping it costs ~125 lines of correct code and one no-op updater
tick. Each of the three blocks now carries the retirement note naming the
condition for its deletion and where to record it (ADR-067's and ADR-074's
Consequences). The note is in the code, not in `TODO.md`, which stays empty.

## Finding 11 — `go mod tidy`, and a `REQUIREMENTS.md` edit NOT made

`go mod tidy` promoted `github.com/opencontainers/image-spec` from indirect to
direct, which is what it always was in fact. No version changed.

The second half — `REQUIREMENTS.md`'s approved-dependency list omitting
`image-spec` and `golang.org/x/sys` — is **not done**, deliberately.
`REQUIREMENTS.md` is immutable without the operator's explicit approval of the
exact edit, and "go with your recommendations" was an answer about technical
approach; the recommendation for this item was itself *propose the edit and
wait*. Assembling it turned up that the list is stale in both directions: it
still names `creack/pty`, which is no longer a dependency of this module at
all. The exact edit has been put to the operator and is pending. Nothing in
this phase depends on it.

## Finding 15 — the ADR-069 audit note names its residual

`image/Dockerfile:47-56` records the floating Antigravity CLI install as a
reasoned, accepted exception, and that reasoning is sound: Google publishes
`agy` latest-only behind a manifest whose sha512 the installer verifies, and
offers no versioned artifact, so the pin a security review asked for does not
exist to take. That is why the finding was rebutted in the merged record.

What the note did not say is what the exception leaves open, which is why a
reviewer read it as covering more than it does: the fetch of `install.sh`
itself is TLS-only, and the sha512 check happens inside a script nothing
verified — as root, in an image every tenant microVM boots beside their
brokered credentials. Two sentences added. No code change: the exception
stands, and the note now describes it honestly, with the condition that would
let it be closed.

## Verification

Bare, status read:

- `go vet ./...` → 0
- `go test ./...` → 0 (328 pass, 0 fail, 1 skip; 327 before this phase)
- `bash -n` over the three scripts carrying new retirement notes → 0
- `shellcheck -S warning deploy/firewall/*.sh deploy/update/styrman-update` → 0
- `go build ./...` after `go mod tidy` → 0

Not run: Tier 2, Tier 4, container builds — Phase 5.

## Learnings for the local host

The `ip_forward` change alters BOOT behaviour on the production host: a
containerized server whose `/proc/sys/net/ipv4/ip_forward` cannot be found
will now refuse to start instead of warning. The NUC reads that file normally
today, so no action is expected — but it is the one change in this phase that
could stop a deploy, and the release carrying it should be watched rather than
assumed (`styrman-ctl status` after the updater tick).
