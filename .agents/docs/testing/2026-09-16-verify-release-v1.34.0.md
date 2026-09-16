---
owner: main
date: 2026-09-16
---
# Verification — release v1.34.0 on one re-seeded project (2026-09-16)

Range reviewed: the live deploy of tag `v1.34.0` (release commit `c1f7e31`) on the production host, verified on a throwaway project created for it. Commands as run, output as printed (long lines cut at the width shown; `[styrman]` boot-log lines verbatim).

## Status: PASS with two items not matched

Lead: what did not match.

1. **A commit made by Claude Code carries no Co-Authored-By trailer — NOT VERIFIED.** `claude -p` in the guest fails before any tool runs: `Failed to authenticate: OAuth session expired and could not be refreshed` (rc 1). That is the brokered Claude credential on the host, served to every project, not the release; `~/.claude/.credentials.json` is materialized in the home (0600, present). Operator step: sign Claude in once (Settings → Credentials), then run the same command in the project `verify-v1-34-0` (left in place for it) — `git log -1 --format=%B` must show no `Co-Authored-By` line. The setting that makes it so is in place (item 3 below) and the harness's own tests cover the write; the CLI behaviour itself was not observed.
2. **Not part of the brief, found on the way — BUG-063 (open, pre-existing):** the operator's project "google" (f8e77d67, created 12:27:15Z on the 1.33.0 image) was stopped at 14:30:42 local, right after the deploy; the Stop path's in-guest `kill -TERM 1` made the VM exit 0, Docker's `unless-stopped` restart policy restarted it one second later (`dockerd: restarting container … exitCode=0 … manualRestart=false`), and the `ContainerStop` that follows 12 s later hard-killed the restarted VM after 35 s (`containerd: … Dead agent … Agent did not stop sandbox`; `docker inspect`: exited 255, `RestartCount=1`), while the server's inspect calls on it timed out (four WARNs 12:31:01–12:31:20Z). Not this release's change (`orchestrator.go` untouched since v1.33.0); recorded as `.agents/docs/bugs/BUG-063-*.md`. The project now reads `"state":"stopped"`; on its next Start the orchestrator recreates it on 1.34.0 (the image-upgrade path, `orchestrator.go:301-311`) and re-seeds the home. Home and workspace are bind mounts, untouched. Nothing was done to it (no live workarounds).

Everything else in the brief matched.

## Deploy observed

```
$ git push origin main; git push origin v1.34.0        # 2026-09-16T14:29:33+02:00
   8ada624..c1f7e31  main -> main
 * [new tag]         v1.34.0 -> v1.34.0
$ sudo -n journalctl -u styrman-update --since '2026-09-16 14:29:00' -o short-iso | grep -E '\[styrman-update\]|Good "git"'
2026-09-16T14:30:17+02:00 Good "git" signature for ostling.simon@gmail.com with ED25519 key SHA256:5HpYQJjpG+yE1xXBTOIZPsTkbkgkVnV8z0eG8hBawyU
2026-09-16T14:30:17+02:00 [styrman-update] v1.34.0 signature OK (allowed signer)
2026-09-16T14:30:17+02:00 [styrman-update] deployed=v1.33.0 latest-stable=v1.34.0
2026-09-16T14:30:17+02:00 [styrman-update] deploying v1.34.0 as 1.34.0
2026-09-16T14:30:17+02:00 [styrman-update] building runtime image (styrman-runtime:1.34.0)
2026-09-16T14:30:23+02:00 [styrman-update] building + starting the stack (server + ingress) at 1.34.0
2026-09-16T14:30:46+02:00 [styrman-update] self-install targets: /usr/local/bin/styrman-update, /etc/systemd/system/styrman-update.{service,timer}
2026-09-16T14:30:46+02:00 [styrman-update] OK — v1.34.0 is live
$ docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'          # 14:31
styrman-caddy   styrman-caddy:1.34.0    Up 45 seconds
styrman-server  styrman-server:1.34.0   Up 45 seconds (healthy)
$ docker inspect -f '{{index .Config.Labels "styrman.version"}}' styrman-server
1.34.0
$ sudo -n cat /etc/styrman/update/deployed-version /etc/styrman/update/deployed-sha
v1.34.0
c1f7e311d92196975b24397f87ce9705f1fd9a12
$ docker exec styrman-server curl -fsS http://localhost:8080/api/health
{"status":"ok"}
$ curl -s -o /dev/null -w '%{http_code}\n' https://crew.styrman.xyz/
302
$ docker logs styrman-server 2>&1 | head -3
{"time":"2026-09-16T12:30:31.509741235Z","level":"INFO","msg":"credential broker up (bridge-only)","addr":":8091"}
{"time":"2026-09-16T12:30:31.509973265Z","level":"INFO","msg":"styrman listening","addr":"0.0.0.0:8080","auth":"header","image":"styrman-runtime:1.34.0","runtime":"mvm","homes":"/var/lib/styrman/homes","version":"1.34.0"}
{"time":"2026-09-16T12:30:31.510514218Z","level":"INFO","msg":"adopted state","containers":2}
$ docker run --rm --entrypoint sh styrman-runtime:1.34.0 -c 'cat /styrman/VERSION; echo; cat /styrman/harness/COMMIT; echo; test -e /styrman/harness/.git && echo HAS-GIT || echo no-git'
1.34.0
4aa19be7eeb5b15e4b876832bd81807cd5d91b37
no-git
```

No rollback, no latch; `/etc/styrman/update/` holds `deployed-sha` and `deployed-version` only. No `ip_forward` line in the server log (the v1.33.0 guard's silent branch).

## The re-seeded project

The one running project had an attached terminal, so the check used a throwaway project created through the API from inside the server container, as the operator's own identity:

```
$ docker exec styrman-server sh -c 'curl -s -X POST -H "X-Styrman-Ingress: $STYRMAN_INGRESS_TOKEN" -H "X-Caddy-Email: ostling.simon@gmail.com" -H "Content-Type: application/json" -d "{\"name\":\"verify-v1-34-0\"}" http://localhost:8080/api/projects'
{"id":"ea28256e-0534-482b-aa33-c54464b58a9c","name":"verify-v1-34-0","dir":"verify-v1-34-0","createdAt":"2026-09-16T12:31:20.185875533Z","state":"creating"}
$ docker logs styrman-ea28256e-0534-482b-aa33-c54464b58a9c          # first boot, 12:31:2xZ
[harness-migrate] no previous harness stamp — nothing to migrate
[styrman]   settings: /home/ostling-simon/.claude/settings.json created with "attribution": {"commit": "", "pr": ""}  Claude Code's commit and PR trailers are off
[styrman] agent harness deployed (version 1.34.0, styrman-harness 4aa19be; browser MCP registered for claude and agy)
[styrman] creating 20G sparse docker image /home/ostling-simon/.dockerd/ea28256e-0534-482b-aa33-c54464b58a9c.img
[styrman] dockerd ready (overlay2 on /var/lib/docker-project)
[styrman] project ea28256e-0534-482b-aa33-c54464b58a9c ready (user=ostling-simon uid=1000)
```

Matched: `agent harness deployed (version 1.34.0, styrman-harness 4aa19be…)` and exactly one `settings: /home/<user>/.claude/settings.json created with …` line. Observation, cosmetic: the installer's em dash before "Claude Code's" is dropped by the entrypoint's `tr -cd '[:print:]\n'` (non-ASCII), leaving two spaces.

In the home, as the project user:

```
$ docker exec … runuser -u ostling-simon -- env HOME=/home/ostling-simon bash -lc 'grep -c '"'"'"attribution"'"'"' ~/.claude/settings.json; cat ~/.claude/settings.json'
1
{
  "attribution": {"commit": "", "pr": ""}
}
$ … bash -lc 'sh-agent status'                 # non-interactive login shell
bash: line 1: sh-agent: command not found      # rc 127 — ~/.local/bin is added by ~/.bashrc:23, which only an interactive shell sources
$ … bash -lic 'command -v sh-agent; sh-agent status'   # interactive login shell, as the terminal runs
/home/ostling-simon/.local/bin/sh-agent
repo       /styrman/harness (plain copy, not its own git checkout; update is refused)
installed  2026-09-16T12:31:21Z from commit  (vendor: all, 260 files, manifest ~/.config/sh-agent/state.json)
binary     (devel)
retro      none yet — run /retro at the end of a session with corrections
repo has   27 skills, 7 agents, 0 commands, 0 rules
claude     ~/.claude  (AGENTS.md, CLAUDE.md, skills/, agents/, commands/, rules/)
agy        ~/.gemini/config  (rules/AGENTS.md, skills/, agents/, workflows/, rules/)
drift      none — 259 files match the repo
```

Matched: `repo has 27 skills, 7 agents` and no `note` line (the "command not found" was the test shape, a non-interactive shell; the terminal's interactive shell has `sh-agent` on PATH). The `installed … from commit  (…)` field is empty for a plain copy (no `.git`), as before.

The Claude Code commit:

```
$ … bash -lc 'cd ~/projects/verify-v1-34-0 && claude -p "Create a file named hello.txt … git add hello.txt and git commit -m \"verify v1.34.0 attribution\" …" --allowedTools "Write,Bash(git add:*),Bash(git commit:*)" < /dev/null'
Failed to authenticate: OAuth session expired and could not be refreshed
claude rc=1
$ git log -1 --format=%B
fatal: your current branch 'main' does not have any commits yet
```

Not verified (item 1 above). `claude --version` in the guest: `2.1.270 (Claude Code)`; git identity in the guest from the broker: the operator's name and email. (A first attempt put the prompt after `--allowedTools`, which swallowed it — "Input must be provided either through stdin or as a prompt argument" — corrected before the run above.)

## Not examined

- The trailer behaviour of Claude Code itself (blocked by the expired OAuth session).
- A fresh Google sign-in in a browser showing 1.34.0 in Settings (versioning.md step 5, the one step an agent cannot do).
- The "google" project's next Start (the image-upgrade recreate and re-seed) — the operator's; BUG-063's fix is a separate task.
- The throwaway project `verify-v1-34-0` was NOT deleted, so the trailer check can be run in it after the re-sign-in; delete it afterwards from the sidebar or `DELETE /api/projects/ea28256e-0534-482b-aa33-c54464b58a9c`.

Scratch: `.agents/scratch/verify-release-v1.34.0/` (create.json, home-checks.txt, sh-agent-status*.txt, claude-commit*.txt, google-*.txt).
