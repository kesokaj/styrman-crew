---
owner: main
date: 2026-09-09
---
# ADR-067 Phase 2 — the server-side machinery is deleted

**293 insertions, 2,398 deletions across 20 files.** Everything the server did
to ask root for a firewall rule is gone; what remains is one boolean on the
project record and the bridge name it chooses.

## Deleted

- **`internal/netpolicy/`** — the whole package (6 files, 1,049 lines): the
  marker directory, its lock and Sync, the status-file parser, `Derive`,
  `Availability`, and the `MinScriptVersion` handshake.
- **`internal/httpapi/network.go`** — 345 → 131 lines. Gone with it:
  `hostFirewallView`, `hostFirewall`, `ingressBridgeMismatch`, `lanFor`,
  `LanExposed`, `kickLAN`, `kickThrottle`, and `pruneHostReachingCreds` —
  the second, independent path that deleted the SSH key out of a project
  home on every enable.
- **`internal/httpapi/lanprune_test.go`** — 347 lines testing only the above.
- **`credbroker`** — `LanChecker`, `Broker.lan`, `lanExposed`,
  `isHostReachingFile`, the withholding branch, and the `lan` parameter of
  `New`. **`credstore.IsHostReaching`** followed once its last caller went.
- **`project.Project`** — `LanChangedAt` and `LanEverEnabled` (the latch),
  `RestoreLanAccess`, `setLanLocked`; `SetLanAccess` loses its `now`.
- **`Server`** — `netpol`, `NetPolicy()`, `lanFields`, `ingressBr`,
  `ingressBrMu`, `ingressBrAt`; `projectView.LanState`/`LanMessage` become one
  `LanAccess` boolean.
- **`cmd/styrman/main.go`** — the boot marker sync, the "feature disabled"
  branch, the 2-minute re-sync ticker, both `Kick` calls, and the marker-dir
  creation.
- **`config`** — `FirewallRunDir` and `NetPolicyDir()`.
- **`deploy/docker-compose.yml`** — `STYRMAN_FIREWALL_DIR` and the
  `/run/styrman-firewall:rslave` bind (BUG-016's entire subject).

`netMu` was **kept**, with its job narrowed to serializing a mode change
against a project deletion. Deleting it with the markers would have reopened a
race a security review closed — the cold reader flagged that specifically.

## The sweep earned its place

The Appendix B sweep — one grep per deleted identifier — was run as the
completion check and **caught two surfaces the work had missed**:
`cfg.NetPolicyDir()` with its boot-time `MkdirAll` in `main.go`, and
`Orchestrator.BridgeNameForNetwork`, whose only caller was the deleted
`ingressBridgeMismatch`. Neither would have failed the build. It now returns
zero hits on the Go tree.

## Test accounting (BUG-005)

**216 → 192, and every one of the 24 is accounted for:**

| Source | Lost | Why |
|---|---|---|
| `netpolicy/dir_test.go` | 6 | marker dir Sync/SetMarker/Kick — package deleted |
| `netpolicy/status_test.go` | 4 | status parsing, Derive, Availability — no status file |
| `netpolicy/script_test.go` | 2 | Go↔shell version parity — no shell version to match |
| `httpapi/lanprune_test.go` | 9 | the credential prune and ingress cross-check |
| `httpapi/network_test.go` | 1 | 7 → 6; enable-gates/throttle/host-view replaced by mode round-trip |
| `project/store_lan_test.go` | 1 | 4 → 3; RestoreLanAccess and the undated clock |
| `credbroker/broker_test.go` | 1 | 9 → 8; IsHostReachingFile + withholding → "serves everything" |

No test of surviving behaviour was dropped. Three new ones replace what they
could: `TestNetworkModeRoundTrips` (the record, the single view and the list
all agree), `TestLoadDropsPreADR067Fields` (a v1.23/v1.24 record keeps its LAN
mode across the upgrade and loses the latch), and
`TestFilesServesEveryCredential` (the exact inverse of the withholding test,
so the operator's decision is asserted rather than assumed).

15 packages ok (was 16 — `netpolicy` is gone), `go vet` clean, `gofmt` clean,
compose parses, no binary files in the diff.

Still open: Phase 3 (firewall script), Phase 4 (UI — the web sweep still
reports `lanState`/`lanMessage`/`LanBadge`/`lib/lan`), Phase 5, Phase 6.

## One self-inflicted defect, caught

A comment I wrote in `store.go` contained two Cyrillic characters
("ди verge"), from a typing slip. Caught by a non-ASCII scan of every file
this phase touched, run because the writing-files rule's BUG-007 section is
about exactly this class — bytes that compile fine and read wrong. The scan is
cheap and worth keeping in the loop; the two pre-existing `×` and `─`
characters it also surfaced are deliberate and were left alone.
