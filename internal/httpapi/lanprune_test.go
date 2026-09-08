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
