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
	p := Project{Name: "alpha", Agent: "claude", CreatedAt: time.Now().UTC(), GitInit: true}
	if err := s.Add(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(p); err == nil {
		t.Error("duplicate Add should fail")
	}
	if err := s.Add(Project{Name: "beta", Agent: "nope"}); err == nil {
		t.Error("unknown agent should fail")
	}
	if err := s.Add(Project{Name: "Bad Name", Agent: "agy"}); err == nil {
		t.Error("invalid name should fail")
	}

	// Reload from disk: persistence + atomicity (file must be valid JSON).
	s2, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "claude" || !got.GitInit {
		t.Errorf("reloaded project = %+v", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file left behind")
	}

	if err := s2.Remove("alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Get("alpha"); err != ErrNotFound {
		t.Errorf("Get after Remove = %v, want ErrNotFound", err)
	}
	if err := s2.Remove("alpha"); err != ErrNotFound {
		t.Errorf("double Remove = %v, want ErrNotFound", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "p.json"))
	base := time.Now()
	for i, name := range []string{"old", "mid", "new"} {
		if err := s.Add(Project{Name: name, Agent: "agy", CreatedAt: base.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	list := s.List()
	if len(list) != 3 || list[0].Name != "new" || list[2].Name != "old" {
		t.Errorf("List order wrong: %+v", list)
	}
}
