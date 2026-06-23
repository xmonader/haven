package config

import (
	"testing"

	"haven/internal/store"
)

func TestConfigSetGetAndAuthorFallback(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, ok, _ := Get(db, "user.name"); ok {
		t.Fatal("expected user.name absent initially")
	}
	if err := Set(db, "user.name", "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := Set(db, "user.email", "a@e.com"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := Get(db, "user.name"); !ok || v != "Alice" {
		t.Fatalf("Get user.name = %q ok=%v", v, ok)
	}
	name, email := Author(db)
	if name != "Alice" || email != "a@e.com" {
		t.Fatalf("Author = %q/%q", name, email)
	}
}
