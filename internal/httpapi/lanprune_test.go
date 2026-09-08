package httpapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/credstore"
	"github.com/kesokaj/styrman-crew/internal/identity"
)

// ADR-066 part 4: enabling LAN access withdraws host-reaching credentials
// from a project, including any copy already materialized in its home.

func seedCredStore(t *testing.T, n *netHarness) (user string) {
	t.Helper()
	cs, err := credstore.New(filepath.Join(n.state, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	user = identity.DeriveUsername(n.admin)
	for _, c := range []credstore.FileCred{
		{ID: "ssh-id", Path: "~/.ssh/id_ed25519", Mode: "0600", Data: []byte("KEY")},
		{ID: "adc", Path: "~/.config/gcloud/application_default_credentials.json", Mode: "0600", Data: []byte("ADC")},
	} {
		if err := cs.PutFile(user, c); err != nil {
			t.Fatal(err)
		}
	}
	n.s.SetCredStore(cs)
	return user
}

func TestPruneHostReachingCreds(t *testing.T) {
	n := newNetHarness(t)
	user := seedCredStore(t, n)
	home := n.s.cfg.ProjectHome(user, n.pid)

	key := filepath.Join(home, ".ssh", "id_ed25519")
	adc := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	for _, p := range []string{key, adc} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("materialized"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	n.s.pruneHostReachingCreds(user, n.pid)

	if _, err := os.Lstat(key); !os.IsNotExist(err) {
		t.Errorf("the ssh key should have been pruned, Lstat err = %v", err)
	}
	if _, err := os.Lstat(adc); err != nil {
		t.Errorf("a non-host-reaching credential must survive: %v", err)
	}
}

// The project home sits under a tree the guest writes, so a component of
// ~/.ssh can be a symlink aimed somewhere else. internal/files confines the
// delete to the tenant base (BUG-002/003); this asserts the target survives.
func TestPruneRefusesToFollowAPlantedSymlink(t *testing.T) {
	n := newNetHarness(t)
	user := seedCredStore(t, n)
	home := n.s.cfg.ProjectHome(user, n.pid)

	outside := t.TempDir() // deliberately NOT under HomesDir
	victim := filepath.Join(outside, "id_ed25519")
	if err := os.WriteFile(victim, []byte("someone else's key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".ssh")); err != nil {
		t.Fatal(err)
	}

	n.s.pruneHostReachingCreds(user, n.pid)

	if _, err := os.Lstat(victim); err != nil {
		t.Fatalf("the prune followed a planted symlink out of the tenant tree: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".ssh")); err != nil {
		t.Errorf("the symlink itself should be left alone, not chased: %v", err)
	}
}

// LanExposed is the broker's gate. It must be the union of the record, root's
// applied set, and "cannot confirm" — never the record alone, or a disable
// would re-serve the key while the firewall rule was still live.
func TestServerLanExposed(t *testing.T) {
	n := newNetHarness(t)
	user := identity.DeriveUsername(n.admin)

	exposed := func(t *testing.T) bool {
		t.Helper()
		got, err := n.s.LanExposed(user, n.pid)
		if err != nil {
			t.Fatalf("LanExposed: %v", err)
		}
		return got
	}

	// No status file at all: root's word is unreadable, which is never "off".
	if !exposed(t) {
		t.Error("no status file must read as exposed")
	}

	// Root confirms the rule is absent and the record is off.
	n.writeStatus(t, time.Now(), "")
	if exposed(t) {
		t.Error("record off + root confirms gone must read as not exposed")
	}

	// Record off, but root still lists the rule — the disable window.
	n.writeStatus(t, time.Now(), "lan="+n.pid+"\n")
	if !exposed(t) {
		t.Error("root still applying the rule must read as exposed")
	}

	// Record on, root has not caught up yet — withhold from the instant of intent.
	n.writeStatus(t, time.Now(), "")
	sp, err := n.s.space(user)
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.projects.SetLanAccess(n.pid, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !exposed(t) {
		t.Error("record on must read as exposed even before root applies it")
	}

	// A status too old to act on cannot confirm anything.
	if err := sp.projects.SetLanAccess(n.pid, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	n.writeStatus(t, time.Now().Add(-24*time.Hour), "")
	if !exposed(t) {
		t.Error("a stale status must read as exposed")
	}
}

// ADR-066 fields on the API. The host view is admin-only (it carries the
// operator's prefixes); credentialImpact describes the owner's own project
// and is not.
func TestLanViewADR066Fields(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "lan_allow_host=1\nlan_allow_published_ports=1\ningress_bridge=br-0123456789ab\nlan_host_addrs=192.0.2.1/32\nlan_pub_subnets=172.17.0.0/16\n")

	sp, err := n.s.space(identity.DeriveUsername(n.admin))
	if err != nil {
		t.Fatal(err)
	}
	p, err := sp.projects.Get(n.pid)
	if err != nil {
		t.Fatal(err)
	}

	v := n.s.lanView(p, true, time.Now())
	if v.Host == nil {
		t.Fatal("an admin must get the host view")
	}
	if !v.Host.AllowHost || !v.Host.AllowPublishedPorts {
		t.Errorf("opt-ins not surfaced: %+v", v.Host)
	}
	if v.Host.IngressBridge != "br-0123456789ab" {
		t.Errorf("IngressBridge = %q", v.Host.IngressBridge)
	}
	if len(v.Host.HostAddrs) != 1 || len(v.Host.PubSubnets) != 1 {
		t.Errorf("addrs/subnets = %v / %v", v.Host.HostAddrs, v.Host.PubSubnets)
	}
	if v.CredentialImpact != "" {
		t.Errorf("a project with LAN off withholds nothing, got %q", v.CredentialImpact)
	}

	// A non-admin owner gets no host view at all — the operator's prefixes
	// and bridge names are not theirs to see (BUG-017).
	if nv := n.s.lanView(p, false, time.Now()); nv.Host != nil {
		t.Error("a non-admin must not receive the host view")
	}

	// Turning it on surfaces the credential cost, admin or not.
	if err := sp.projects.SetLanAccess(n.pid, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, _ = sp.projects.Get(n.pid)
	if got := n.s.lanView(p, false, time.Now()).CredentialImpact; got != "ssh-key-withheld" {
		t.Errorf("CredentialImpact = %q, want ssh-key-withheld", got)
	}
}

// The mismatch check must stay silent unless it actually knows better: no
// published-port opt-in, no reported bridge, or no answer from Docker all
// mean "no opinion", never a false alarm.
func TestIngressBridgeMismatchStaysSilentWithoutAnAnswer(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "lan_allow_published_ports=1\ningress_bridge=br-0123456789ab\n")
	st := n.s.netpol.Status(time.Now())
	if st == nil {
		t.Fatal("status")
	}
	// No Docker in this harness, so BridgeNameForNetwork answers "" — which
	// must read as no opinion rather than as a mismatch.
	if w := n.s.ingressBridgeMismatch(st); w != "" {
		t.Errorf("unknown bridge must not warn, got %q", w)
	}
	// Not opted in: nothing to check at all.
	n.writeStatus(t, time.Now(), "ingress_bridge=br-0123456789ab\n")
	if w := n.s.ingressBridgeMismatch(n.s.netpol.Status(time.Now())); w != "" {
		t.Errorf("no opt-in must not warn, got %q", w)
	}
}
