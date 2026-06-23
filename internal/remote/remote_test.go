package remote

import (
	"testing"

	"haven/internal/store"
)

func TestRemoteAddGetListRemove(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Add(db, "origin", "http://h/r.hv", Team); err != nil {
		t.Fatal(err)
	}
	if err := Add(db, "mine", "http://h/p.hv", Personal); err != nil {
		t.Fatal(err)
	}
	if err := Add(db, "bad", "http://x", "nonsense"); err == nil {
		t.Fatal("invalid kind must be rejected")
	}

	got, err := Get(db, "origin")
	if err != nil || got.URL != "http://h/r.hv" || got.Kind != Team {
		t.Fatalf("Get origin = %+v, err=%v", got, err)
	}
	if list, err := List(db); err != nil || len(list) != 2 {
		t.Fatalf("List = %v (err %v), want 2", list, err)
	}
	if err := Remove(db, "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(db, "origin"); err == nil {
		t.Fatal("Get after Remove should fail")
	}
}
