package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	valid := []string{"a", "my-project", "web2", "a1-b2-c3"}
	invalid := []string{"", "My-Project", "1abc", "-abc", "a_b", "a.b", "a b",
		"this-name-is-way-too-long-for-a-project-slug"}
	for _, n := range valid {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range invalid {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", n)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(Project{Name: "alpha", CreatedAt: time.Now().UTC(), GitInit: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.Dir != "alpha" {
		t.Errorf("Add did not assign id/dir: %+v", p)
	}
	if _, err := s.Add(Project{Name: "alpha"}); err == nil {
		t.Error("duplicate live name should fail")
	}
	if _, err := s.Add(Project{Name: "Bad Name"}); err == nil {
		t.Error("invalid name should fail")
	}

	// Reload from disk: persistence + atomicity (file must be valid JSON).
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" || !got.GitInit {
		t.Errorf("reloaded project = %+v", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file left behind")
	}

	// Rename keeps the dir; name becomes reusable after removal.
	if err := s2.Rename(p.ID, "omega"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s2.Get(p.ID); got.Name != "omega" || got.Dir != "alpha" {
		t.Errorf("rename moved the dir: %+v", got)
	}
	if err := s2.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Get(p.ID); err != ErrNotFound {
		t.Errorf("Get after Remove = %v, want ErrNotFound", err)
	}
	if _, err := s2.Add(Project{Name: "omega"}); err != nil {
		t.Errorf("name reuse after deletion should work: %v", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "p.json"))
	base := time.Now()
	for i, name := range []string{"old", "mid", "new"} {
		if _, err := s.Add(Project{Name: name, CreatedAt: base.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	list := s.List()
	if len(list) != 3 || list[0].Name != "new" || list[2].Name != "old" {
		t.Errorf("List order wrong: %+v", list)
	}
}
