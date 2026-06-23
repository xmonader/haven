package secret

import (
	"testing"

	"haven/internal/store"
)

func TestMarksStore(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if m, _ := Marks(db); len(m) != 0 {
		t.Fatalf("fresh repo should have no marks, got %v", m)
	}
	if err := SeedDefaultMarks(db); err != nil {
		t.Fatal(err)
	}
	got, err := Marks(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(DefaultMarks) {
		t.Fatalf("seeded %d marks, want %d", len(got), len(DefaultMarks))
	}
	// Idempotent: re-seeding does not duplicate.
	if err := SeedDefaultMarks(db); err != nil {
		t.Fatal(err)
	}
	if again, _ := Marks(db); len(again) != len(DefaultMarks) {
		t.Fatalf("re-seed duplicated marks: %d", len(again))
	}
	// A seeded mark actually classifies a file.
	if !Match(".env", got) {
		t.Fatal(".env should match a default mark")
	}
	if Match("main.go", got) {
		t.Fatal("main.go should not be a secret")
	}
	// Custom mark.
	if err := AddMark(db, "config/*.token"); err != nil {
		t.Fatal(err)
	}
	marks, _ := Marks(db)
	if !Match("config/api.token", marks) {
		t.Fatal("custom mark should classify config/api.token")
	}
}
