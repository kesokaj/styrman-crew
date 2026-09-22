---
owner: bugs
date: 2026-09-22
id: BUG-072
status: fixed
area: deploy/gcp/install.sh
---
# BUG-072: install.sh verified the wrong hostname after a healthy bring-up

## Symptom
2026-09-22, the first successful GCP bring-up (v1.37.0): the tag's signature
verified on the VM, `host-setup.sh` installed Docker and Kata on a fresh
host, a Kata microVM booted (guest kernel 6.18.28), the runtime image built,
the stack came up, and `styrman-ctl` logged `install: styrman is up at
https://styrman.gcp.styrman.xyz`. `install.sh` then failed its own
verification: `ERROR https://crew.gcp.styrman.xyz/ did not redirect to
auth.gcp.styrman.xyz after 3 minutes (last: '000 ')` and exited 1, after
removing the staged secrets as designed. The host was healthy the whole
time.

## Repro
Run `deploy/gcp/install.sh` with a secrets file (`~/.config/styrman/gcp/env`)
setting `STYRMAN_HOSTNAME=crew` against a fresh GCP host.

## Root cause
`deploy/styrman-ctl`'s `cmd_install` copies only the six `MUST_FILL` keys
(`BASE_DOMAIN GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET CLOUDFLARE_API_TOKEN
ADMIN_EMAIL STYRMAN_ALLOWED_EMAILS`) from `--env-file` into the host's
`deploy/.env`. `STYRMAN_HOSTNAME` is optional and not in that list, so it
never reached the host, which used the default `styrman`. The host served
`styrman.gcp.styrman.xyz` (verified live: `302` to
`https://auth.gcp.styrman.xyz/oauth2/google/?redirect_url=...`), while
`install.sh` probed `crew.gcp.styrman.xyz`, a name that exists only as a
Cloudflare wildcard pointing at the production NUC (`dig` returned the NUC's
address 83.249.78.58; the TLS handshake failed there).
`.agents/docs/deploy/gcp.md` also documented the app URL as
`crew.gcp.styrman.xyz`, so the deploy guide described a hostname no host
had.

## Fix
Same day, `deploy/gcp/install.sh`: a new step 5b delivers each optional key
present in the secrets env (`STYRMAN_HOSTNAME`, `STYRMAN_PUBLIC_DOMAIN`,
`STYRMAN_PRIVATE_DOMAIN`) to the host with `sudo styrman-ctl edit
KEY=VALUE`, each value validated against `^[a-z0-9.-]+$` before it reaches
the remote command. Step 6 then reads the hostname back from the host's own
`/opt/styrman/deploy/.env` and verifies THAT URL, warning when it differs
from the secrets file's request, rather than assuming the edit landed.
`TestGCPInstallPlan` asserts the dry-run plan contains `styrman-ctl edit
STYRMAN_HOSTNAME=crew`; must-fail verified by deleting the delivery loop
(fails with "the plan does not deliver STYRMAN_HOSTNAME to the host") and
restored from a byte copy (`cmp` identical); the suite is green.

## Prevention
When one tool hands configuration to another, the handing tool's contract
is the list of keys it actually carries — a key outside that list is a
default on the far side, not the value you passed, so either deliver it
explicitly or read the far side's effective value back before acting on
it. A check that probes a NAME (a URL, a host, a record) must probe the
name the thing under test actually claims, read from it, not the name the
caller intended — otherwise a healthy system reads as broken, and a
wildcard DNS record can make that failure look like a network problem.

## Related
BUG-069 (a check that diagnosed a condition through a transport it had not
verified). The describing-text rule in the global context: the deploy guide
named a hostname no host had.
