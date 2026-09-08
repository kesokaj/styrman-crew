package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// LanExposed is the broker's gate, and since the ADR-066 amendment it is a
// LATCH on the record — no dependency on root's firewall status at all. The
// point of these cases is that no sequence of status files can make the key
// come back.
func TestServerLanExposedLatches(t *testing.T) {
	n := newNetHarness(t)
	user := identity.DeriveUsername(n.admin)
	sp, err := n.s.space(user)
	if err != nil {
		t.Fatal(err)
	}
	exposed := func() bool {
		t.Helper()
		got, err := n.s.LanExposed(user, n.pid)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// Never enabled: the key is served, whatever root's status says or does
	// not say. This is the case the pre-amendment code got wrong — with no
	// status file it withheld from every project on the host.
	if exposed() {
		t.Error("a project that has never had LAN access must get its key")
	}
	n.writeStatus(t, time.Now(), "")
	if exposed() {
		t.Error("a readable status must not change that")
	}

	// Enabled: withheld.
	if err := sp.projects.SetLanAccess(n.pid, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !exposed() {
		t.Fatal("an enabled project must be withheld from")
	}

	// Disabled again: STILL withheld. Re-serving would not un-expose the key,
	// and the live-state question it replaced had no reliable answer.
	if err := sp.projects.SetLanAccess(n.pid, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !exposed() {
		t.Error("the latch must survive turning LAN access back off")
	}

	// No status file, stale status, root reporting the rule applied or gone:
	// none of it can unlatch.
	for _, extra := range []string{"", "lan=" + n.pid + "\n", "error=" + n.pid + ":delete failed\n"} {
		n.writeStatus(t, time.Now(), extra)
		if !exposed() {
			t.Errorf("status %q must not unlatch", extra)
		}
	}
	n.writeStatus(t, time.Now().Add(-24*time.Hour), "")
	if !exposed() {
		t.Error("a stale status must not unlatch")
	}
	if err := os.RemoveAll(n.fw); err != nil {
		t.Fatal(err)
	}
	if !exposed() {
		t.Error("removing the firewall half entirely must not unlatch")
	}
}

// The rollback path must not clear the latch: it means the MARKER write
// failed after the record saved, so the intent was recorded and the broker
// may already have stopped serving. Un-latching is the one direction that can
// hand a key back by accident.
func TestLanLatchSurvivesRollback(t *testing.T) {
	n := newNetHarness(t)
	user := identity.DeriveUsername(n.admin)
	sp, err := n.s.space(user)
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.projects.SetLanAccess(n.pid, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	before, _ := sp.projects.Get(n.pid)
	if !before.LanEverEnabled {
		t.Fatal("enable must latch")
	}
	if err := sp.projects.RestoreLanAccess(n.pid, false, time.Time{}); err != nil {
		t.Fatal(err)
	}
	after, _ := sp.projects.Get(n.pid)
	if after.LanAccess {
		t.Error("rollback must clear LanAccess")
	}
	if !after.LanEverEnabled {
		t.Error("rollback must NOT clear the latch")
	}
	if got, _ := n.s.LanExposed(user, n.pid); !got {
		t.Error("a rolled-back enable still withholds")
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

// A wrong-but-VALID INGRESS_BRIDGE passes root's shape and existence checks
// while leaving the real ingress reachable. Root cannot detect it; the server
// must refuse the enable rather than merely noting it in an admin panel
// (security review, 2026-09-08).
func TestEnableRefusedOnIngressBridgeMismatch(t *testing.T) {
	n := newNetHarness(t)
	n.writeStatus(t, time.Now(), "lan_allow_published_ports=1\ningress_bridge=br-0123456789ab\n")

	// Simulate Docker having answered with a DIFFERENT bridge. Without this
	// the harness has no Docker, the lookup returns "" and the check is
	// correctly silent — which is the other half of the contract.
	n.s.ingressBrMu.Lock()
	n.s.ingressBr = "br-ffffffffffff"
	n.s.ingressBrAt = time.Now()
	n.s.ingressBrMu.Unlock()

	path := "/api/projects/" + n.pid + "/network"
	w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("enable with a mismatched ingress bridge: got %d, want 503 — body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "br-ffffffffffff") {
		t.Errorf("the refusal must name the bridge the network actually uses: %s", w.Body.String())
	}

	// Disable is never refused, mismatch or not — tightening always wins.
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":false}`); w.Code != http.StatusOK {
		t.Errorf("disable must still be accepted: %d %s", w.Code, w.Body.String())
	}

	// Agreement clears it.
	n.s.ingressBrMu.Lock()
	n.s.ingressBr = "br-0123456789ab"
	n.s.ingressBrAt = time.Now()
	n.s.ingressBrMu.Unlock()
	if w := doAs(t, n.h, n.admin, http.MethodPut, path, `{"lanAccess":true}`); w.Code != http.StatusOK {
		t.Errorf("matching bridges must not block the enable: %d %s", w.Code, w.Body.String())
	}
}

// The broker and the prune must agree about which paths are host-reaching.
// They were written separately and diverged on "./"-spelled paths — one
// withheld what the other left on disk.
func TestPrunePredicateMatchesTheBrokers(t *testing.T) {
	for _, p := range []string{
		"~/.ssh/id_ed25519", "$HOME/.ssh/id_rsa", ".ssh/config", "./.ssh/key", ".//.ssh/key", "~/.ssh",
	} {
		if !credstore.IsHostReaching(p) {
			t.Errorf("%q must be treated as host-reaching by BOTH callers", p)
		}
	}
	for _, p := range []string{
		"~/.config/gcloud/application_default_credentials.json", "~/.sshfoo/key", "~/notes/.ssh-notes.md",
	} {
		if credstore.IsHostReaching(p) {
			t.Errorf("%q must not be withheld", p)
		}
	}
}

// `git diff --no-index` takes git out of repository mode, so its path is not
// confined by git itself. scopeRel must reject an escape before the exec
// (audit 2026-09-08). Testing scopeRel directly is the honest unit here: the
// handler needs a live guest, which this harness has no way to provide.
func TestScopeRelRefusesEscapes(t *testing.T) {
	for _, bad := range []string{"../../../etc/shadow", "..", "../x", "/../etc/passwd", "  ../../y  "} {
		rel, err := scopeRel(bad)
		if err == nil && !strings.HasPrefix(rel, "../") && rel != "" {
			// path.Clean collapses a leading /.. to /, which scopeRel then
			// trims — so the surviving value must not escape.
			if strings.Contains(rel, "..") {
				t.Errorf("scopeRel(%q) = %q — still escapes", bad, rel)
			}
		}
	}
	// A normal relative path must survive untouched.
	if rel, err := scopeRel("src/main.go"); err != nil || rel != "src/main.go" {
		t.Errorf("scopeRel(src/main.go) = %q, %v", rel, err)
	}
	if _, err := scopeRel("   "); err == nil {
		t.Error("an empty path must be refused")
	}
}
