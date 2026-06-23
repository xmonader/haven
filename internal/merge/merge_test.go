package merge

import (
	"strings"
	"testing"
)

func TestCleanMergeNonOverlapping(t *testing.T) {
	base := []byte("a\nb\nc\n")
	ours := []byte("a CHANGED\nb\nc\n")  // edit first line
	theirs := []byte("a\nb\nc CHANGED\n") // edit last line
	merged, conflict := Content(base, ours, theirs)
	if conflict {
		t.Fatalf("non-overlapping edits should merge cleanly, got:\n%s", merged)
	}
	got := string(merged)
	if !strings.Contains(got, "a CHANGED") || !strings.Contains(got, "c CHANGED") {
		t.Errorf("merged content missing edits:\n%s", got)
	}
}

func TestConflictingMerge(t *testing.T) {
	base := []byte("a\nb\nc\n")
	ours := []byte("a\nOURS\nc\n")
	theirs := []byte("a\nTHEIRS\nc\n")
	merged, conflict := Content(base, ours, theirs)
	if !conflict {
		t.Fatal("overlapping edits should conflict")
	}
	got := string(merged)
	for _, want := range []string{markerOurs, "OURS", markerSplit, "THEIRS", markerTheirs} {
		if !strings.Contains(got, want) {
			t.Errorf("conflict output missing %q:\n%s", want, got)
		}
	}
}

func TestIdenticalEditNoConflict(t *testing.T) {
	base := []byte("a\nb\nc\n")
	same := []byte("a\nB\nc\n")
	merged, conflict := Content(base, same, same)
	if conflict {
		t.Fatal("identical edits on both sides should not conflict")
	}
	if string(merged) != "a\nB\nc\n" {
		t.Errorf("got %q", merged)
	}
}

func TestAddSameLines(t *testing.T) {
	base := []byte("a\n")
	ours := []byte("a\nours-tail\n")
	theirs := []byte("a\n") // unchanged
	merged, conflict := Content(base, ours, theirs)
	if conflict || string(merged) != "a\nours-tail\n" {
		t.Errorf("one-sided append: conflict=%v got=%q", conflict, merged)
	}
}
