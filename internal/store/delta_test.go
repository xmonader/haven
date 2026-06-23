package store

import (
	"bytes"
	"strings"
	"testing"
)

func roundtrip(t *testing.T, base, target []byte) {
	t.Helper()
	d := MakeDelta(base, target)
	got, err := ApplyDelta(base, d)
	if err != nil {
		t.Fatalf("applyDelta: %v", err)
	}
	if !bytes.Equal(got, target) {
		t.Fatalf("reconstructed %q, want %q", got, target)
	}
}

func TestDeltaRoundtrip(t *testing.T) {
	cases := []struct {
		name         string
		base, target string
	}{
		{"identical", "the quick brown fox jumps", "the quick brown fox jumps"},
		{"append", "the quick brown fox jumps", "the quick brown fox jumps over the lazy dog"},
		{"prepend", "the quick brown fox jumps", "well, the quick brown fox jumps"},
		{"middle-change", "the quick brown fox jumps over", "the quick RED fox jumps over"},
		{"empty-base", "", "the quick brown fox jumps over"},
		{"empty-target", "the quick brown fox jumps over", ""},
		{"both-empty", "", ""},
		{"unrelated", "aaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{"short", "hi", "hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			roundtrip(t, []byte(c.base), []byte(c.target))
		})
	}
}

func TestDeltaRealisticFileEdit(t *testing.T) {
	base := strings.Repeat("line of source code\n", 500)
	target := base + "// one new line appended at the very end\n"
	roundtrip(t, []byte(base), []byte(target))

	d := MakeDelta([]byte(base), []byte(target))
	// A one-line append against a 10 KB base must compress to a tiny delta —
	// otherwise delta storage buys nothing.
	if len(d) > 200 {
		t.Fatalf("delta for a one-line append is %d bytes; expected it to be small", len(d))
	}
}

func TestDeltaRejectsCorrupt(t *testing.T) {
	base := []byte("the quick brown fox jumps over the lazy dog")
	d := MakeDelta(base, []byte("the quick brown fox is sleeping"))

	// Truncated stream.
	if _, err := ApplyDelta(base, d[:len(d)/2]); err == nil {
		t.Fatal("expected error on truncated delta")
	}
	// Wrong base length must be detected.
	if _, err := ApplyDelta([]byte("different base entirely here!"), d); err == nil {
		t.Fatal("expected error when base length mismatches")
	}
	// Garbage op stream.
	if _, err := ApplyDelta(base, []byte{0x2c, 0x1f, 0xff, 0xff}); err == nil {
		t.Fatal("expected error on garbage delta")
	}
}
