---
owner: main
date: 2026-09-13
---
# Firewall script suite — v1.30.1 gate

Run by the main thread: `unshare -Urnm bash deploy/firewall/test.sh` (the suite refuses the host namespace). Verbatim output:

```
PASS golden sandbox bridge name
PASS golden lan bridge name
=== 1. base rules
PASS physdev RETURN present
PASS six private-range DROPs
PASS STYRMAN-IN is a bare DROP
PASS INPUT jump to STYRMAN-IN
PASS DOCKER-USER jump to STYRMAN-FWD
=== 2. ADR-067: the jumps match the sandbox prefix and NOT the lan prefix
PASS no rule anywhere names the lan- interface
PASS the sdb- wildcard covers the sandbox iface
PASS no rule names a lan- wildcard
=== 3. idempotence and self-heal
PASS second run changes nothing
PASS flushed chain is rebuilt
PASS physdev RETURN restored
PASS deleted INPUT jump is reinserted
=== 4. ADR-065/066 leftovers are cleaned up
PASS setup: three tagged rules planted
PASS all three are removed
PASS the six DROPs are intact
PASS STYRMAN-IN is a bare DROP again
PASS cleanup is idempotent
=== 5. the cleanup refuses to delete a DROP
PASS the tagged DROP survives
PASS and the run reports a warning
=== 6. IPv6 mirror
PASS STYRMAN-IN6 drops
PASS INPUT6 jump on the sdb- wildcard
PASS no ip6 rule names the lan- interface

PASS=24 FAIL=0
rc=0
```
