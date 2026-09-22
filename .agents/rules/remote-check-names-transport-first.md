# A remote check names the transport's failure first (BUG-069)

The first GCP install died with "no /dev/kvm on styrman" while the VM had
KVM: the operator's own network had started dropping port 22, and both
`vm.sh status` ("styrman not installed") and `install.sh` diagnosed a
condition through an ssh they had not checked. Scanners reached the VM's
public 22 within four minutes of boot.

- A check made over a transport (ssh, HTTP, a broker) probes the transport
  first — one `true` over ssh, one `/healthz` — and a transport failure is
  reported as exactly that, with the tool's own troubleshooting command,
  never as the condition the check was about. `deploy/gcp/lib.sh`
  `ssh_reachable`/`ssh_hint` is the model; a new remote check reuses it.
- A test of such a check plants a transport failure (`FAKE_SSH_RC=255`) and
  asserts the message names the transport and NOT the condition
  (`TestGCPSSHUnreachableIsNamed`).
- A management port (ssh) is not exposed publicly when the platform offers
  a tunnel (IAP here); the public path stays as an explicit opt-in.

Same family as BUG-034 (silence from your own instrumentation) and BUG-040
(a green run against the wrong server): a signal read as the wrong thing.
