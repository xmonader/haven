package merge

import (
	"strings"
	"testing"

	"haven/internal/object"
	"haven/internal/store"
)

func mkTree(t *testing.T, s *object.Store, files map[string]string) string {
	t.Helper()
	entries := map[string]object.FileEntry{}
	for path, content := range files {
		h, err := s.Put(object.Blob, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		entries[path] = object.FileEntry{Hash: h, Mode: object.ModeFile, Type: object.Blob}
	}
	tree, err := object.BuildTree(s, entries)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// A file renamed on one side and edited in place on the other must carry the
// edit onto the new path with no spurious modify/delete conflict.
func TestRenamePlusEditMergesCleanly(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := object.NewStore(db)

	base := mkTree(t, s, map[string]string{"old.txt": "line1\nline2\nline3\n"})
	ours := mkTree(t, s, map[string]string{"new.txt": "line1\nline2\nline3\n"})    // pure rename
	theirs := mkTree(t, s, map[string]string{"old.txt": "line1\nEDITED\nline3\n"}) // edit in place

	res, err := Trees(s, base, ours, theirs)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("expected no conflict, got %v", res.Conflicts)
	}
	if _, ok := res.Files["old.txt"]; ok {
		t.Error("old.txt should be gone after rename")
	}
	fe, ok := res.Files["new.txt"]
	if !ok {
		t.Fatal("new.txt missing from merge result")
	}
	_, content, _ := s.Get(fe.Hash)
	if !strings.Contains(string(content), "EDITED") {
		t.Errorf("edit not carried onto renamed path: %q", content)
	}
}
