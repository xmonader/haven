package diff

import (
	"strings"
	"testing"
)

func TestUnifiedIdentical(t *testing.T) {
	if got := Unified("a", "b", []byte("x\n"), []byte("x\n")); got != "" {
		t.Errorf("identical content should yield empty diff, got %q", got)
	}
}

func TestUnifiedModification(t *testing.T) {
	old := []byte("line1\nline2\nline3\n")
	new := []byte("line1\nline2 EDITED\nline3\nline4\n")
	got := Unified("a/f", "b/f", old, new)

	for _, want := range []string{
		"--- a/f", "+++ b/f",
		"-line2\n", "+line2 EDITED\n", "+line4\n",
		" line1\n", " line3\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("diff missing %q:\n%s", want, got)
		}
	}
}

func TestUnifiedAddDelete(t *testing.T) {
	if got := Unified("a", "b", nil, []byte("new\n")); !strings.Contains(got, "+new") {
		t.Errorf("added content:\n%s", got)
	}
	if got := Unified("a", "b", []byte("gone\n"), nil); !strings.Contains(got, "-gone") {
		t.Errorf("deleted content:\n%s", got)
	}
}

func TestMaps(t *testing.T) {
	a := map[string]string{"keep": "1", "mod": "2", "del": "3"}
	b := map[string]string{"keep": "1", "mod": "9", "add": "4"}
	changes := Maps(a, b)

	kinds := map[string]Kind{}
	for _, c := range changes {
		kinds[c.Path] = c.Kind
	}
	if kinds["mod"] != Modified || kinds["del"] != Deleted || kinds["add"] != Added {
		t.Errorf("unexpected kinds: %+v", kinds)
	}
	if _, ok := kinds["keep"]; ok {
		t.Error("unchanged path should not appear")
	}
}
