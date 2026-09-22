package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// makeEntries fills dir with n empty files and returns dir.
func makeEntries(t testing.TB, dir string, n int) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range n {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("f%06d.txt", i)))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	return dir
}

// The directory is guest-writable and the explorer polls it, so List is
// bounded like ListNames. Before 2026-09-22 it read the whole directory and
// sorted it in O(n^2) — a per-tenant lever on the one shared process.
func TestListIsCappedAndReportsTruncation(t *testing.T) {
	root := t.TempDir()
	makeEntries(t, filepath.Join(root, "many"), maxListEntries+50)

	lst, err := List(root, "many")
	if err != nil {
		t.Fatal(err)
	}
	if len(lst.Entries) != maxListEntries {
		t.Errorf("List returned %d entries, want the cap %d", len(lst.Entries), maxListEntries)
	}
	if !lst.Truncated {
		t.Error("Truncated is false on a directory past the cap: a cut listing must not read as a complete one")
	}
}

// The other half of the same claim: an ordinary directory is not reported as
// truncated. A cap that always says "truncated" would pass the test above and
// tell the UI nothing.
func TestListIsNotTruncatedForASmallDirectory(t *testing.T) {
	root := t.TempDir()
	makeEntries(t, filepath.Join(root, "few"), 3)

	lst, err := List(root, "few")
	if err != nil {
		t.Fatal(err)
	}
	if len(lst.Entries) != 3 {
		t.Errorf("List returned %d entries, want 3", len(lst.Entries))
	}
	if lst.Truncated {
		t.Error("Truncated is true for a 3-entry directory")
	}
}

// An empty directory reads as empty, not as an error: ReadDir(n>0) returns
// io.EOF there, and List must not surface that as a failure.
func TestListOnAnEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	lst, err := List(root, "empty")
	if err != nil {
		t.Fatalf("List on an empty directory failed: %v", err)
	}
	if len(lst.Entries) != 0 || lst.Truncated {
		t.Errorf("empty directory listed as %+v", lst)
	}
}

// BUG-004's blind spot: a root reached through Root.OpenRoot below a trusted
// tenant base behaves differently from a plain root, and most of this
// package's tests only ever exercise the plain one. The cap is asserted on
// the path production actually uses.
func TestListCapAppliesBelowATrustedBase(t *testing.T) {
	base := t.TempDir()
	Trust(base)
	ws := filepath.Join(base, "alice", "proj")
	makeEntries(t, filepath.Join(ws, "many"), maxListEntries+10)

	lst, err := List(ws, "many")
	if err != nil {
		t.Fatal(err)
	}
	if len(lst.Entries) != maxListEntries {
		t.Errorf("below a trusted base List returned %d entries, want the cap %d", len(lst.Entries), maxListEntries)
	}
	if !lst.Truncated {
		t.Error("below a trusted base a cut listing did not report Truncated")
	}
}

// sortEntries stopped being an insertion sort on 2026-09-22; the order it
// produced is the part that must not change.
func TestSortEntriesOrdersDirsFirstThenCaseInsensitively(t *testing.T) {
	in := []Entry{
		{Name: "zebra.txt"},
		{Name: "Alpha.txt"},
		{Name: "src", Dir: true},
		{Name: "beta.txt"},
		{Name: "Docs", Dir: true},
	}
	sortEntries(in)

	var got []string
	for _, e := range in {
		got = append(got, e.Name)
	}
	want := "Docs,src,Alpha.txt,beta.txt,zebra.txt"
	if strings.Join(got, ",") != want {
		t.Errorf("sortEntries produced %q, want %q", strings.Join(got, ","), want)
	}
}

// Stability is why this uses SortStableFunc: two names equal under
// ToLower must keep the order the filesystem gave them, as the insertion
// sort did. An unstable sort passes the ordering test above and fails this.
func TestSortEntriesIsStableForNamesEqualUnderCaseFolding(t *testing.T) {
	in := []Entry{
		{Name: "README", Size: 1},
		{Name: "readme", Size: 2},
		{Name: "ReadMe", Size: 3},
	}
	sortEntries(in)

	var got []int64
	for _, e := range in {
		got = append(got, e.Size)
	}
	if got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("sizes after sort = %v, want [1 2 3]: order of case-equal names was not preserved", got)
	}
}

// ReadRawNoFollow opened its PARENT without O_NONBLOCK until 2026-09-22 — the
// only open in the package missing it. A guest that replaces ~/.ssh with a
// FIFO wedged the serving goroutine, and its OS thread, in open(2) forever.
// The timeout is the assertion: without the flag this test hangs, so it is
// written to fail instead.
func TestReadRawNoFollowDoesNotWedgeOnAFifoParent(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, "dotssh")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ReadRawNoFollow(root, "dotssh/id_ed25519.pub", 0)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("reading through a FIFO parent returned no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadRawNoFollow blocked on a guest-planted FIFO parent: the open is missing O_NONBLOCK")
	}
}

// The cost the cap and the sort replacement exist to bound. Run with
// -benchtime=1x for a single honest pass:
//
//	go test ./internal/files/ -run x -bench ListLargeDirectory -benchtime=1x
func BenchmarkListLargeDirectory(b *testing.B) {
	root := b.TempDir()
	makeEntries(b, filepath.Join(root, "many"), 20000)

	b.ResetTimer()
	for b.Loop() {
		if _, err := List(root, "many"); err != nil {
			b.Fatal(err)
		}
	}
}
