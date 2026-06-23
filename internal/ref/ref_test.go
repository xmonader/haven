package ref

import (
	"testing"

	"haven/internal/store"
)

func TestRefLifecycle(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Resolve of a missing ref is "" with no error.
	if tgt, err := Resolve(db, BranchPrefix+"main"); err != nil || tgt != "" {
		t.Fatalf("Resolve missing = %q, %v", tgt, err)
	}
	if err := Set(db, BranchPrefix+"main", "c1"); err != nil {
		t.Fatal(err)
	}
	if tgt, _ := Resolve(db, BranchPrefix+"main"); tgt != "c1" {
		t.Fatalf("Resolve = %q, want c1", tgt)
	}
	// Set infers public visibility for a branch.
	if rf, _ := Get(db, BranchPrefix+"main"); rf.Visibility != Public {
		t.Errorf("branch visibility = %q, want public", rf.Visibility)
	}
	// SetVisible overrides.
	if err := SetVisible(db, HavenPrefix+"secret", "c2", Private); err != nil {
		t.Fatal(err)
	}
	if rf, _ := Get(db, HavenPrefix+"secret"); rf.Visibility != Private {
		t.Errorf("haven visibility = %q, want private", rf.Visibility)
	}
	list, _ := List(db)
	if len(list) != 2 {
		t.Fatalf("List len = %d, want 2", len(list))
	}
	if err := Delete(db, BranchPrefix+"main"); err != nil {
		t.Fatal(err)
	}
	if tgt, _ := Resolve(db, BranchPrefix+"main"); tgt != "" {
		t.Error("ref should be gone after Delete")
	}
}

func TestVisibilityForAndShortName(t *testing.T) {
	cases := map[string]string{
		BranchPrefix + "main": Public,
		HavenPrefix + "wip":   Private,
		"refs/policy":         Policy,
		"refs/tags/v1":        Public,
	}
	for name, want := range cases {
		if got := VisibilityFor(name); got != want {
			t.Errorf("VisibilityFor(%q) = %q, want %q", name, got, want)
		}
	}
	if ShortName(BranchPrefix+"main") != "main" {
		t.Error("ShortName should strip the branch prefix")
	}
}
