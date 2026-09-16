---
owner: main
date: 2026-09-16
---
# Verification — release v1.34.1 live: the Stop path on a re-seeded project (2026-09-16)

Range reviewed: the live deploy of tag `v1.34.1` (release commit `f7d1b15`) on the production host, verified on the throwaway project `verify-v1-34-0` (id `ea28256e-0534-482b-aa33-c54464b58a9c`, created for the v1.34.0 check). Commands as run, output as printed. The operator's own projects (`google`, `asdasdasd`) were not touched.

## Status: PASS

Everything in the brief matched; nothing did not.

## Deploy observed

```
$ git push origin main; git push origin v1.34.1        # 2026-09-16T16:05:18+02:00
   976b565..f7d1b15  main -> main
 * [new tag]         v1.34.1 -> v1.34.1
$ sudo -n journalctl -u styrman-update --since '2026-09-16 16:05:00' -o short-iso | grep -E '\[styrman-update\]|Good "git"'
2026-09-16T16:06:40+02:00 Good "git" signature for ostling.simon@gmail.com with ED25519 key SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU
2026-09-16T16:06:40+02:00 [styrman-update] v1.34.1 signature OK (allowed signer)
2026-09-16T16:06:40+02:00 [styrman-update] deployed=v1.34.0 latest-stable=v1.34.1
2026-09-16T16:06:41+02:00 [styrman-update] deploying v1.34.1 as 1.34.1
2026-09-16T16:06:41+02:00 [styrman-update] building runtime image (styrman-runtime:1.34.1)
2026-09-16T16:06:43+02:00 [styrman-update] building + starting the stack (server + ingress) at 1.34.1
2026-09-16T16:07:07+02:00 [styrman-update] self-install targets: /usr/local/bin/styrman-update, /etc/systemd/system/styrman-update.{service,timer}
2026-09-16T16:07:07+02:00 [styrman-update] OK — v1.34.1 is live
$ docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'          # 16:07:41
styrman-caddy   styrman-caddy:1.34.1    Up 49 seconds
styrman-server  styrman-server:1.34.1   Up 49 seconds (healthy)
styrman-fae465b5-…  styrman-runtime:1.34.0  Up 27 minutes        # the operator's "asdasdasd", untouched
styrman-ea28256e-…  styrman-runtime:1.34.0  Up 2 hours           # the throwaway project, before the check
$ docker inspect -f '{{index .Config.Labels "styrman.version"}}' styrman-server
1.34.1
$ sudo -n cat /etc/styrman/update/deployed-version /etc/styrman/update/deployed-sha
v1.34.1
f7d1b159dfaa1619c2be94911606860b6e57b9d7
$ docker exec styrman-server curl -fsS http://localhost:8080/api/health
{"status":"ok"}
$ curl -s -o /dev/null -w '%{http_code}\n' https://crew.styrman.xyz/
302
$ docker logs styrman-server 2>&1 | grep 'styrman listening'
{"time":"2026-09-16T14:06:52.107206197Z","level":"INFO","msg":"styrman listening","addr":"0.0.0.0:8080","auth":"header","image":"styrman-runtime:1.34.1","runtime":"mvm",…
```

No rollback, no latch.

## The Stop path (BUG-063) on the throwaway project

Before the stop the container ran on `styrman-runtime:1.34.0` since 12:31:20Z (v1.34.0's check), `restarts=0 policy=unless-stopped`.

```
$ docker inspect -f 'status={{.State.Status}} exit={{.State.ExitCode}} restarts={{.RestartCount}} policy={{.HostConfig.RestartPolicy.Name}}' styrman-ea28256e-…   # before
status=running exit=0 restarts=0 policy=unless-stopped
$ docker exec styrman-server sh -c 'curl -s -o /dev/null -w "%{http_code}\n" -X POST -H "X-Styrman-Ingress: $STYRMAN_INGRESS_TOKEN" -H "X-Caddy-Email: ostling.simon@gmail.com" http://localhost:8080/api/projects/ea28256e-…/stop'   # 16:07:41
200                                          # returned at 16:07:43, about two seconds
$ docker inspect … # after stop
status=exited exit=0 restarts=0 policy=no finished=2026-09-16T14:07:42.436592315Z
$ docker logs --tail 4 -t styrman-ea28256e-…
2026-09-16T12:31:22.633502675Z [styrman] creating 20G sparse docker image …
2026-09-16T12:31:23.675224623Z [styrman] dockerd ready (overlay2 on /var/lib/docker-project)
2026-09-16T12:31:23.677504466Z [styrman] project ea28256e-… ready (user=ostling-simon uid=1000)
2026-09-16T14:07:41.311197822Z [styrman] shutting down project ea28256e-…
$ sudo -n journalctl -u docker --since '2026-09-16 16:07:41' -o short-iso | grep -E '<container id>|restarting container'
2026-09-16T16:07:42+02:00 … level=info msg="received task-delete event from containerd" container=95cb1fe18e16…
$ docker logs --since 2026-09-16T14:07:41Z styrman-server 2>&1 | grep ea28256e
(no lines)
```

Matched, against v1.34.0's live failure: one shutdown line and no second boot; exit 0, `RestartCount` 0, policy `no`; the daemon logged the task delete only — no "restarting container" — and the server logged no inspect timeouts. (v1.34.0 on "google": a second boot one second after the shutdown, exit 255 after a 35 s hang, four WARNs.)

## The Start path

```
$ … curl -X POST … /api/projects/ea28256e-…/start     # 16:08:01
200
$ (ready at 16:08:05)
$ docker inspect … # after start
status=running exit=0 restarts=0 policy=unless-stopped image=styrman-runtime:1.34.1 created=2026-09-16T14:08:01.690463284Z
$ docker logs -t styrman-ea28256e-…                    # a NEW container: the image-upgrade recreate
2026-09-16T14:08:02.799051920Z [harness-migrate] sh-agent manifest present — already migrated
2026-09-16T14:08:03.967756616Z [styrman] agent harness deployed (version 1.34.1, styrman-harness 4aa19be; browser MCP registered for claude and agy)
2026-09-16T14:08:04.762803275Z [styrman] dockerd ready (overlay2 on /var/lib/docker-project)
2026-09-16T14:08:04.764886876Z [styrman] project ea28256e-… ready (user=ostling-simon uid=1000)
$ docker logs --since 2026-09-16T14:08:01Z styrman-server 2>&1 | grep ea28256e
{"time":"2026-09-16T14:08:01.666707987Z","level":"INFO","msg":"recreating stopped container for image upgrade","project":"ea28256e-…","old":"sha256:d12740437531","new":"sha256:69e5f84215d9"}
{"time":"2026-09-16T14:08:01.687651229Z","level":"INFO","msg":"creating microVM","container":"styrman-ea28256e-…","image":"styrman-runtime:1.34.1","runtime":"mvm"}
$ sudo -n journalctl -u docker --since '2026-09-16 16:08:01' | grep -c 'restarting container'
0
```

Matched: the stopped container on the old image was recreated on 1.34.1 (the image-upgrade path — the one the operator's stopped "google" takes on its next Start), the new container carries `unless-stopped`, one boot, ready in four seconds; the home re-seeded at 1.34.1 (the `settings:` line is absent because the key is already present — the installer leaves a present key alone and says nothing, as designed).

## Not examined

- A Stop through the sidebar button or the idle reaper (the API route is the same handler).
- A host reboot (the persisted `unless-stopped` on a running project is what it reads; not exercised).
- The operator's projects: "google" still sits stopped on its 1.33.0 container (its next Start recreates it on 1.34.1); "asdasdasd" (created 13:39Z) runs on 1.34.0 until its next stop/start.
- The v1.34.0 brief's Claude Code trailer check: still blocked on the expired brokered OAuth session (`testing/2026-09-16-verify-release-v1.34.0.md`); the throwaway project stays for it.

Scratch: `.agents/scratch/verify-release-v1.34.1/` (stop-check.txt, start-check.txt), `.agents/scratch/preflight-v1.34.1/` (verify-commit-f7d1b15.txt, sizing-probe.txt).
