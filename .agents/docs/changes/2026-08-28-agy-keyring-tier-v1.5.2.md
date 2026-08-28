# agy joins the broker — keyring tier, v1.5.2 (ADR-032a)

Date: 2026-08-28. The operator asked "agy pull on login?" — re-investigation
killed the "unsupported" verdict: the agy binary vendors **zalando/go-keyring**
(found via `strings`), and our own `styrman-setup` probe already keyed on
`service antigravity` — so the Secret Service item shape was fully known:
`{service: antigravity, username: <account>}`. Nothing installs at login;
agy ships in the image — only the TOKEN is pulled.

## What changed
- `credstore`: fourth kind, `keyring-<id>.json` ({id, service, username,
  label, secret b64}; service/username/label validated single-line for the
  TSV delivery). List shows `service (username)`; Delete by id.
- `credbroker`: `GET /keyring` → `[{service, username, label, secret(b64)}]`.
- `image/configs/styrman-credentials.sh`: after env/files, pulls /keyring and
  `secret-tool store`s each item into the project's unlocked gnome-keyring
  (styrman-keyring.sh sorts first, so the daemon is up). Identical attributes
  replace-or-create → indistinguishable from a native sign-in.
- import (`POST /api/settings/credentials/import`): extracts agy via
  `secret-tool search --all service antigravity` (username) + `secret-tool
  lookup` (raw secret). **Bug found live:** `secret-tool search` prints
  `attribute.*` lines to STDERR (item/label/secret to stdout) — the first
  import attempt's `2>/dev/null` silently dropped exactly those lines; fixed
  by merging streams.
- UI/api: kind union + "keyring" chip; import hint includes agy.
- Docs: ADR-032a; R3 updated (operator gave blanket approval this session);
  README/guide/api/runtime-image — no "agy excepted" language remains.

## Proven live (synthetic token; state scrubbed back to virgin afterwards)
Project A: seeded a fake `{service: antigravity, username: synth@test}` item
→ import reported `agy: stored (keyring)`; store listed
`keyring/agy/antigravity (synth@test)` value-free. Project B (brand new,
fresh home): first login shell held the item — `secret-tool lookup` returned
the exact secret and the styrman-setup probe printed "agy: keyring
credentials present" — zero manual steps. Both test projects deleted, state
scrubbed to byte-virgin, server restarted (v1.5.2): `projects: []`,
credentials `[]`, `/var/lib/styrman` = two empty roots. The operator's
morning first-login test is untouched.

## Remaining assumption (confirm on the operator's first real sign-in)
go-keyring semantics say a seeded item is indistinguishable, but a REAL agy
token round-trip is still pending: after signing agy in once tomorrow, run
Import and open a second project — agy should skip its sign-in URL. If agy
ever stores a second item under another service name, extend the import's
search.

## Verification
go test -race ./... clean (keyring store round-trip/validation/isolation,
broker /keyring + 403, list/delete); gofmt clean; tsc clean; image 1.5.2 +
server 1.5.2 deployed; live synthetic end-to-end above.
