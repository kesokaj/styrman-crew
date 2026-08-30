# Changelog backfill: v1.9.1 – v1.9.3 (tagged 2026-08-30, shipped 2026-08-30)

The reader pass backfilled release tags v1.9.0..v1.14.1; three tagged
versions had no changelog FILE of their own (their content was recorded
inside the running v1.9.0 remediation notes). For tag↔changelog coherence:

- **v1.9.1** (85e7a65): SidePanel.tsx split into a shell +
  SidePanelPorts/History/Git/Clips.tsx; harvestCredentials + helpers moved
  to internal/httpapi/credharvest.go. Completed the two refactors deferred
  from the /code-review ultra remediation.
- **v1.9.2** (e1bfc6e): full verified doc sweep to the shipped state +
  keyring-vestige removal (compose/.env.example/host-setup, runtime
  packages, dead api.ts union member); integration.sh moved off the deleted
  /diff + /checkpoints endpoints.
- **v1.9.3** (ca32cb2): broker-sync of the gcloud CLI ACCOUNT login
  (credentials.db + active_config + config_default), not just ADC;
  settings rows split gcloud (ADC) / gcloud CLI.

Details: 2026-08-30-security-review-remediation-v1.9.0.md (running notes)
and the commits themselves.
