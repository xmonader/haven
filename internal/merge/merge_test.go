package merge

import (
	"strings"
	"testing"
)

func TestCleanMergeNonOverlapping(t *testing.T) {
	base := []byte("a\nb\nc\n")
	ours := []byte("a CHANGED\nb\nc\n")   // edit first line
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

// TestOverlappingEditsConflict guards a real bug: edits that overlap but start
// at different base lines were silently auto-merged into a tree neither side
// authored (conflict reported false). Both sides here change base line "c", so
// it must be a conflict.
func TestOverlappingEditsConflict(t *testing.T) {
	base := []byte("a\nb\nc\nd\n")
	ours := []byte("a\nX\nY\nd\n")   // replaces b,c
	theirs := []byte("a\nb\nZ\nW\n") // replaces c,d
	merged, conflict := Content(base, ours, theirs)
	if !conflict {
		t.Fatalf("overlapping edits to 'c' must conflict; got clean merge:\n%s", merged)
	}
	s := string(merged)
	if !strings.Contains(s, markerOurs) || !strings.Contains(s, markerTheirs) {
		t.Fatalf("expected conflict markers, got:\n%s", s)
	}
	// Neither side's content may be silently dropped: ours' view and theirs' view
	// must both appear between the markers.
	if !strings.Contains(s, "X\nY\nd") || !strings.Contains(s, "b\nZ\nW") {
		t.Fatalf("a side's view was lost in the conflict region:\n%s", s)
	}
}

// TestAdjacentNonOverlapStillClean ensures the conservative overlap grouping
// did not turn independent, adjacent edits into false conflicts.
func TestAdjacentNonOverlapStillClean(t *testing.T) {
	base := []byte("a\nb\nc\n")
	ours := []byte("X\nb\nc\n")   // changes line 0
	theirs := []byte("a\nb\nZ\n") // changes line 2
	merged, conflict := Content(base, ours, theirs)
	if conflict {
		t.Fatalf("adjacent non-overlapping edits must merge cleanly:\n%s", merged)
	}
	if got := string(merged); got != "X\nb\nZ\n" {
		t.Fatalf("clean merge = %q, want %q", got, "X\nb\nZ\n")
	}
}
