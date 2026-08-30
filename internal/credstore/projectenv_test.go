package credstore

import "testing"

// Project-scoped env tier (ADR-043): per-project isolation, same validation
// as the user-wide tier, invisible to List, and removable wholesale.
func TestProjectEnv(t *testing.T) {
	cs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Empty → empty map, no error.
	if m, err := cs.ProjectEnv("alice", "p1"); err != nil || len(m) != 0 {
		t.Fatalf("empty ProjectEnv = %v, %v", m, err)
	}
	if err := cs.PutProjectEnv("alice", "p1", "DATABASE_URL", "postgres://p1"); err != nil {
		t.Fatal(err)
	}
	if err := cs.PutProjectEnv("alice", "p2", "DATABASE_URL", "postgres://p2"); err != nil {
		t.Fatal(err)
	}

	// Cross-project isolation — each project sees only its own value.
	m1, _ := cs.ProjectEnv("alice", "p1")
	m2, _ := cs.ProjectEnv("alice", "p2")
	if m1["DATABASE_URL"] != "postgres://p1" || m2["DATABASE_URL"] != "postgres://p2" {
		t.Fatalf("isolation broken: p1=%v p2=%v", m1, m2)
	}
	// Cross-user isolation.
	if m, _ := cs.ProjectEnv("bob", "p1"); len(m) != 0 {
		t.Fatal("cross-user project env leak")
	}
	// The user-wide tier is untouched.
	if m, _ := cs.AllEnv("alice"); len(m) != 0 {
		t.Fatalf("project put leaked into user-wide tier: %v", m)
	}
	// List never surfaces project-scoped vars (they'd render as deletable
	// rows in Settings whose delete would target the wrong tier).
	if items, _ := cs.List("alice"); len(items) != 0 {
		t.Fatalf("List surfaced project-scoped vars: %+v", items)
	}

	// Same validation as the user-wide tier.
	if err := cs.PutProjectEnv("alice", "p1", "bad key", "v"); err == nil {
		t.Error("bad key accepted")
	}
	if err := cs.PutProjectEnv("alice", "p1", "K", "a\nb"); err == nil {
		t.Error("control-char value accepted")
	}
	if err := cs.PutProjectEnv("alice", "", "K", "v"); err == nil {
		t.Error("empty project accepted")
	}

	// Delete one key.
	if err := cs.DeleteProjectEnv("alice", "p1", "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	if err := cs.DeleteProjectEnv("alice", "p1", "DATABASE_URL"); err != ErrNotFound {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}

	// DeleteProjectAll wipes the whole project dir; other projects keep theirs.
	cs.PutProjectEnv("alice", "p1", "A", "1")
	cs.PutProjectEnv("alice", "p1", "B", "2")
	if err := cs.DeleteProjectAll("alice", "p1"); err != nil {
		t.Fatal(err)
	}
	if m, _ := cs.ProjectEnv("alice", "p1"); len(m) != 0 {
		t.Fatalf("DeleteProjectAll left vars: %v", m)
	}
	if m, _ := cs.ProjectEnv("alice", "p2"); m["DATABASE_URL"] != "postgres://p2" {
		t.Fatal("DeleteProjectAll leaked into another project")
	}
}
