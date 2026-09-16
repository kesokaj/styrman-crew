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

## Operator-requested follow-up (16:11 local)

"start google and delete the throwaway project" — both through the API from inside the server container, as the operator's identity.

```
$ … curl -X POST … /api/projects/f8e77d67-04bf-4099-a933-16f8aa4bfc3d/start     # 16:11:30; before: exited exit=255 restarts=1 image=styrman-runtime:1.33.0
200                                                                             # ready at 16:11:33
$ docker inspect … styrman-f8e77d67-…
status=running exit=0 restarts=0 policy=unless-stopped image=styrman-runtime:1.34.1 created=2026-09-16T14:11:30.309875419Z
$ docker logs -t styrman-f8e77d67-…
2026-09-16T14:11:31.362518300Z [harness-migrate] sh-agent manifest present — already migrated
2026-09-16T14:11:31.525582391Z [styrman]   settings: /home/ostling-simon/.claude/settings.json created with "attribution": {"commit": "", "pr": ""}  Claude Code's commit and PR trailers are off
2026-09-16T14:11:32.547866110Z [styrman] agent harness deployed (version 1.34.1, styrman-harness 4aa19be; browser MCP registered for claude and agy)
2026-09-16T14:11:33.498292274Z [styrman] dockerd ready (overlay2 on /var/lib/docker-project)
2026-09-16T14:11:33.501376693Z [styrman] project f8e77d67-… ready (user=ostling-simon uid=1000)
$ docker logs styrman-server 2>&1 | grep f8e77d67 | tail -2
… "msg":"recreating stopped container for image upgrade","project":"f8e77d67-…","old":"sha256:9510f15f9a7e","new":"sha256:…"
… "msg":"creating microVM","container":"styrman-f8e77d67-…","image":"styrman-runtime:1.34.1","runtime":"mvm"
$ docker exec … 'ls -la ~/.claude/settings.json; grep -c "\"attribution\"" ~/.claude/settings.json'
-rw-r--r-- 1 ostling-simon ostling-simon 46 Sep 16 14:11 /home/ostling-simon/.claude/settings.json
1
```

The BUG-063 casualty recovered on its next Start exactly as the image-upgrade path promises, and its home — seeded at 1.33.0 without the key — received the attribution key on that boot: the `settings: … created with` line on the operator's own project, which the v1.34.0 brief asked for.

```
$ … curl -X DELETE … "/api/projects/ea28256e-0534-482b-aa33-c54464b58a9c?wipe=true"   # 16:11:35
202
$ docker ps -a | grep -c styrman-ea28256e; docker network ls | grep -c ea28256e
0
0
$ jq '.projects[] | {id, name}' users/ostling-simon/projects.json
{"id":"f8e77d67-…","name":"google"}
{"id":"fae465b5-…","name":"asdasdasd"}
$ ls -d projects/ostling-simon/ea28256e-… homes/ostling-simon/p/ea28256e-…
(both: No such file or directory)
```

The throwaway project, its network, its record, its workspace and its home are gone. The pending Claude Code trailer check therefore runs in any project after the re-sign-in ("google" now has the key).
