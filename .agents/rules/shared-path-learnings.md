# A second target exercises the shared path (BUG-070, operator 2026-09-21)

The first GCP bring-up found that `host-setup.sh` could not install Kata on
any fresh host: the download URL had been dead upstream for months, hidden by
`[[ ! -x /opt/kata/bin/kata-runtime ]]` on the NUC, which never re-runs the
step. The operator: "take your learnings back to local if any".

- What a second target (GCP, a fresh VM, a new machine) teaches about the
  SHARED path — `host-setup.sh`, `styrman-ctl`, the updater, the image — is
  fixed in the shared path in the same change, never kept target-specific.
  The change entry carries a "Learnings for the local host" section, with
  "none" written deliberately when there are none.
- A step guarded by "already installed" is verified only by a host that has
  not installed it. A change to such a step, or a release carrying one,
  states how the fresh-host path was exercised: `deploy/gcp/vm.sh create`
  + `deploy/gcp/install.sh` against a new VM, or "fresh-host path
  unexercised" in the gate report — never a green re-run on the NUC.
- A download's checksum comes from something the publisher still publishes
  (GitHub's release-asset digest), never a sidecar file assumed to exist.

Same family as BUG-035 (a mechanism must reach every caller — here the
fresh-host caller) and BUG-056 (an external publisher changing under a green
gate).
