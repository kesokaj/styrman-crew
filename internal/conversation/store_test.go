package conversation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversations.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := s.Create("alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	// An empty title gets a generated friendly name (adjective noun).
	if r1.ID != "1" || r1.Title == "" || len(strings.Fields(r1.Title)) < 2 {
		t.Errorf("first record = %+v", r1)
	}
	r2, _ := s.Create("alpha", "My chat")
	if r2.ID != "2" || r2.Title != "My chat" {
		t.Errorf("second record = %+v", r2)
	}

	// Reload: persistence + validation-on-load.
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.List("alpha"); len(got) != 2 {
		t.Fatalf("reloaded %d records, want 2", len(got))
	}
	if _, ok := s2.Get("alpha", "1"); !ok {
		t.Error("record 1 lost across reload")
	}

	if err := s2.Rename("alpha", "2", "Renamed"); err != nil {
		t.Fatal(err)
	}
	if rec, _ := s2.Get("alpha", "2"); rec.Title != "Renamed" {
		t.Error("rename did not stick")
	}
	if err := s2.Delete("alpha", "1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get("alpha", "1"); ok {
		t.Error("deleted record still present")
	}
	if err := s2.DeleteProject("alpha"); err != nil {
		t.Fatal(err)
	}
	if got := s2.List("alpha"); len(got) != 0 {
		t.Errorf("DeleteProject left %d records", len(got))
	}
}

// A cached terminal Store must survive its metadata dir being scrubbed under a
// running server (shared cause with the project/profile stores): the next save
// recreates users/<user>/ instead of failing with ENOENT.
func TestSaveRecreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "users", "ostling-simon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s, err := NewStore(filepath.Join(dir, "conversations.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("alpha", ""); err != nil {
		t.Fatalf("first Create (dir present): %v", err)
	}
	if err := os.RemoveAll(dir); err != nil { // the scrub, store still live
		t.Fatalf("scrub: %v", err)
	}
	if _, err := s.Create("alpha", ""); err != nil {
		t.Fatalf("Create after the dir was scrubbed must recover, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "conversations.json")); err != nil {
		t.Fatalf("conversations.json not persisted after recovery: %v", err)
	}
}

// TestListNaturalOrder pins the sidebar ordering (operator request
// 2026-09-01): titles ascending in natural order, NOT most-recently-opened
// first, which made the list reshuffle on every open.
func TestListNaturalOrder(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "conversations.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Created out of order on purpose; "Terminal 10" must NOT sort before
	// "Terminal 2" (the plain-string-sort trap).
	for _, title := range []string{"Terminal 10", "beta", "Terminal 2", "0-first", "Alpha"} {
		if _, err := s.Create("p", title); err != nil {
			t.Fatal(err)
		}
	}
	// Touch the one that would previously have jumped to the top.
	s.Touch("p", "1")

	got := []string{}
	for _, r := range s.List("p") {
		got = append(got, r.Title)
	}
	want := []string{"0-first", "Alpha", "beta", "Terminal 2", "Terminal 10"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("List order = %v, want %v", got, want)
	}

	// Touching must not reorder anything.
	s.Touch("p", "3")
	got2 := []string{}
	for _, r := range s.List("p") {
		got2 = append(got2, r.Title)
	}
	if strings.Join(got2, "|") != strings.Join(want, "|") {
		t.Errorf("order changed after Touch: %v", got2)
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"Terminal 2", "Terminal 10", true},  // numeric, not lexical
		{"Terminal 10", "Terminal 2", false}, //
		{"a", "b", true},                     //
		{"0", "a", true},                     // digits before letters
		{"Apple", "apple", false},            // case-insensitive → equal → not less
		{"apple", "Banana", true},            // case-insensitive ordering
		{"item2", "item2b", true},            // prefix is shorter
		{"v1.9.0", "v1.15.0", true},          // multi-segment numbers
		{"007", "7a", true},                  // leading zeros ignored, then shorter
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// Swedish titles must fold case like any other (review finding: byte-wise
// ASCII folding left "Åke"/"åke" unfolded on a Swedish operator's box).
func TestNaturalLessUnicodeFolding(t *testing.T) {
	if naturalLess("Åke", "åke") || naturalLess("åke", "Åke") {
		t.Error(`"Åke" and "åke" should fold to equivalent`)
	}
	if naturalLess("Ärende", "ärende") || naturalLess("ärende", "Ärende") {
		t.Error(`"Ärende" and "ärende" should fold to equivalent`)
	}
	// Multi-byte runes must not be split mid-sequence by the digit scanner.
	if !naturalLess("Åke 2", "Åke 10") {
		t.Error("numeric ordering broken after a multi-byte rune")
	}
}

// Titles that are EQUIVALENT under naturalLess but not byte-identical must
// still get a deterministic order — sort.Slice is not stable, so without the
// ID tie-break their order could flip on any store mutation.
func TestListStableForEquivalentTitles(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "conversations.json"))
	if err != nil {
		t.Fatal(err)
	}
	// "Apple" and "apple" are equivalent under case-insensitive comparison.
	for _, title := range []string{"apple", "Apple", "APPLE"} {
		if _, err := s.Create("p", title); err != nil {
			t.Fatal(err)
		}
	}
	first := []string{}
	for _, r := range s.List("p") {
		first = append(first, r.ID)
	}
	// Repeated calls, and a mutation in between, must not reshuffle them.
	_ = s.Rename("p", "2", "Apple")
	for i := 0; i < 20; i++ {
		got := []string{}
		for _, r := range s.List("p") {
			got = append(got, r.ID)
		}
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("order flipped between calls: %v vs %v", got, first)
		}
	}
}
