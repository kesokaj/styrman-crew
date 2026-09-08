// Package netpolicy is the server's half of the per-project local-network
// exception (ADR-065). The server cannot touch the host firewall — it runs
// as the operator's uid with no NET_ADMIN — so it only projects INTENT: an
// empty marker file named by the project uuid under
// <StateDir>/netpolicy/lan, a trigger file in the root-parented spool
// /run/styrman-firewall/spool, and it reads back /run/styrman-firewall/status,
// which the root-owned deploy/firewall/styrman-firewall.sh writes on every
// run. Root decides what "local network" means; the server may only say
// on/off per project, and reports state from root's word, never from the
// request lifecycle.
package netpolicy

// MinScriptVersion is the lowest STYRMAN_FW_VERSION of
// deploy/firewall/styrman-firewall.sh whose status file this server accepts.
// Version 3 is the first that writes a status file at all; a host running an
// older script reports "not installed", and a future bump here turns silent
// drift into "outdated (have N, need M)" in Settings and a 503 on enable.
// TestMinScriptVersionMatchesScript keeps this equal to the script's constant.
const MinScriptVersion = 3
