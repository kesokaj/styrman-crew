package netpolicy

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kesokaj/styrman-crew/internal/config"
)

const (
	u1 = "3e6a772c-b177-4cfc-b209-b14db514bac2"
	u2 = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

func newDir(t *testing.T) (*Dir, string, string) {
	t.Helper()
	state := t.TempDir()
	run := t.TempDir()
	if err := os.MkdirAll(filepath.Join(run, "spool"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{StateDir: state, FirewallRunDir: run}
	d := New(cfg, slog.New(slog.DiscardHandler))
	return d, state, run
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestSetMarkerIdempotentAndValidated(t *testing.T) {
	d, state, _ := newDir(t)
	marker := filepath.Join(state, "netpolicy", "lan", u1)
	for i := 0; i < 2; i++ {
		if err := d.SetMarker(u1, true); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Lstat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o644 || fi.Size() != 0 {
		t.Fatalf("marker must be an empty regular 0644 file: %v %d", fi.Mode(), fi.Size())
	}
	for i := 0; i < 2; i++ {
		if err := d.SetMarker(u1, false); err != nil {
			t.Fatal(err)
		}
	}
	if exists(marker) {
		t.Fatal("marker should be gone")
	}
	for _, bad := range []string{"", "not-a-uuid", "../etc", "3E6A772C-B177-4CFC-B209-B14DB514BAC2", u1 + "x", "sdb-sys-setup-alice"} {
		if err := d.SetMarker(bad, true); err == nil {
			t.Errorf("SetMarker(%q) accepted a non-uuid", bad)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(state, "netpolicy", "lan")); len(entries) != 0 {
		t.Fatalf("rejected ids must leave nothing behind: %v", entries)
	}
}

func TestSetMarkerRefusesSymlinkedDir(t *testing.T) {
	d, state, _ := newDir(t)
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(state, "netpolicy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(state, "netpolicy", "lan")); err != nil {
		t.Fatal(err)
	}
	if err := d.SetMarker(u1, true); err == nil {
		t.Fatal("a symlinked marker dir must be refused")
	}
	if exists(filepath.Join(elsewhere, u1)) {
		t.Fatal("wrote through the symlink")
	}
	if _, _, err := d.Sync(func() (map[string]bool, error) { return map[string]bool{u1: true}, nil }); err == nil {
		t.Fatal("Sync must refuse a symlinked marker dir too")
	}
}

func TestSyncMirrorsRecords(t *testing.T) {
	d, state, _ := newDir(t)
	dir := filepath.Join(state, "netpolicy", "lan")
	if err := d.SetMarker(u2, true); err != nil { // will become a stray
		t.Fatal(err)
	}
	// Foreign entries root ignores and Sync must leave alone.
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "11111111-2222-4333-8444-555555555555")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "22222222-2222-4222-8222-222222222222"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sys-setup-alice"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	added, removed, err := d.Sync(func() (map[string]bool, error) {
		return map[string]bool{u1: true, "bogus": true, "cccccccc-cccc-4ccc-8ccc-cccccccccccc": false}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || removed != 1 {
		t.Fatalf("added=%d removed=%d, want 1/1", added, removed)
	}
	if !exists(filepath.Join(dir, u1)) || exists(filepath.Join(dir, u2)) || exists(filepath.Join(dir, "bogus")) {
		t.Fatal("marker set does not mirror the records")
	}
	for _, keep := range []string{"11111111-2222-4333-8444-555555555555", "22222222-2222-4222-8222-222222222222", "sys-setup-alice"} {
		if !exists(filepath.Join(dir, keep)) {
			t.Errorf("foreign entry %s must be left alone", keep)
		}
	}
	// Idempotent.
	if a, r, err := d.Sync(func() (map[string]bool, error) { return map[string]bool{u1: true}, nil }); err != nil || a != 0 || r != 0 {
		t.Fatalf("second sync: %d/%d %v", a, r, err)
	}
	// A snapshot error aborts before any deletion.
	if _, _, err := d.Sync(func() (map[string]bool, error) { return nil, errors.New("boom") }); err == nil || !exists(filepath.Join(dir, u1)) {
		t.Fatal("snapshot error must abort the sync and delete nothing")
	}
	// An unreadable/empty snapshot removes every marker (fail closed).
	if _, r, err := d.Sync(func() (map[string]bool, error) { return map[string]bool{}, nil }); err != nil || r != 1 {
		t.Fatalf("empty want must remove the marker: removed=%d err=%v", r, err)
	}
}

// A SetMarker issued while Sync holds the lock cannot land between Sync's
// snapshot and its delete pass: it waits, then creates the marker after Sync
// is done — the toggle is never reverted (cold-reader finding, plan v2).
func TestSyncDoesNotEatConcurrentSetMarker(t *testing.T) {
	d, state, _ := newDir(t)
	dir := filepath.Join(state, "netpolicy", "lan")
	inside := make(chan struct{})
	release := make(chan struct{})
	syncDone := make(chan error, 1)
	go func() {
		_, _, err := d.Sync(func() (map[string]bool, error) {
			close(inside) // snapshot taken (u1 not in it)
			<-release
			return map[string]bool{}, nil
		})
		syncDone <- err
	}()
	<-inside
	setDone := make(chan error, 1)
	go func() { setDone <- d.SetMarker(u1, true) }()
	select {
	case <-setDone:
		t.Fatal("SetMarker completed while Sync held the lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-syncDone; err != nil {
		t.Fatal(err)
	}
	if err := <-setDone; err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, u1)) {
		t.Fatal("the marker created after the sync must survive")
	}
}

func TestKick(t *testing.T) {
	d, _, run := newDir(t)
	spool := filepath.Join(run, "spool")
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return now }
	if !d.Kick(u1) {
		t.Fatal("kick should succeed")
	}
	entries, _ := os.ReadDir(spool)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), u1+".") {
		t.Fatalf("spool entries: %v", entries)
	}
	for _, bad := range []string{"", "../x", "UPPER", "a b", strings.Repeat("a", 65)} {
		if d.Kick(bad) {
			t.Errorf("Kick(%q) accepted a bad tag", bad)
		}
	}
	// Throttled: same tag inside the window is absorbed, a new tag is not.
	now = now.Add(time.Second)
	if !d.KickThrottled(u1, 10*time.Second) {
		t.Fatal("throttled kick should report sent")
	}
	if entries, _ = os.ReadDir(spool); len(entries) != 1 {
		t.Fatalf("throttled kick must not write: %d entries", len(entries))
	}
	now = now.Add(20 * time.Second)
	if !d.KickThrottled(u1, 10*time.Second) {
		t.Fatal("kick outside the window should write")
	}
	if entries, _ = os.ReadDir(spool); len(entries) != 2 {
		t.Fatalf("expected a second entry: %d", len(entries))
	}
	// Bad tags are refused by the throttled path too.
	if d.KickThrottled("../x", time.Second) {
		t.Fatal("KickThrottled accepted a bad tag")
	}
	// Missing spool: false, never an error.
	if err := os.RemoveAll(spool); err != nil {
		t.Fatal(err)
	}
	if d.Kick(u2) {
		t.Fatal("kick into a missing spool must report false")
	}
	// Disabled feature.
	off := New(&config.Config{StateDir: t.TempDir()}, slog.New(slog.DiscardHandler))
	if off.Enabled() || off.Kick(u1) || off.Status(now) != nil {
		t.Fatal("an empty FirewallRunDir must disable kicks and status")
	}
}

func TestStatusCacheAndLimits(t *testing.T) {
	d, _, run := newDir(t)
	path := filepath.Join(run, "status")
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	if d.Status(now) != nil {
		t.Fatal("absent status must be nil")
	}
	good := "schema=1\nversion=3\napplied_at=" + strconv.FormatInt(now.Unix(), 10) + "\nlan_dev=enp0\nlan_cidrs=192.0.2.0/24\nlan=" + u1 + "\nspool=0\nrun_ms=5\n"
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	// Still cached as absent within the TTL…
	if d.Status(now.Add(time.Second)) != nil {
		t.Fatal("cache should still say absent")
	}
	// …fresh after it.
	st := d.Status(now.Add(3 * time.Second))
	if st == nil || st.Version != 3 {
		t.Fatalf("status not read: %+v", st)
	}
	if _, ok := st.Applied[u1]; !ok {
		t.Fatal("applied set missing the uuid")
	}
	// Oversized → nil.
	if err := os.WriteFile(path, []byte(good+strings.Repeat("warn=x\n", 20000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if d.Status(now.Add(10*time.Second)) != nil {
		t.Fatal("oversized status must be treated as absent")
	}
	// Garbage → nil.
	if err := os.WriteFile(path, []byte("not a status\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d.Status(now.Add(20*time.Second)) != nil {
		t.Fatal("garbage status must be treated as absent")
	}
}
