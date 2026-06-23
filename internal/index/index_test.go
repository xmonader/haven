package index

import (
	"testing"

	"haven/internal/store"
)

func TestIndexAddAllRemoveClear(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Add(db, "a.txt", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := Add(db, "b.txt", "h2"); err != nil {
		t.Fatal(err)
	}
	if err := Add(db, "a.txt", "h1b"); err != nil { // upsert
		t.Fatal(err)
	}
	all, err := All(db)
	if err != nil || all["a.txt"] != "h1b" || all["b.txt"] != "h2" || len(all) != 2 {
		t.Fatalf("All = %v (err %v)", all, err)
	}
	if err := Remove(db, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if all, _ := All(db); len(all) != 1 {
		t.Fatalf("after Remove len = %d, want 1", len(all))
	}
	if err := Clear(db); err != nil {
		t.Fatal(err)
	}
	if all, _ := All(db); len(all) != 0 {
		t.Fatalf("after Clear len = %d, want 0", len(all))
	}
}
