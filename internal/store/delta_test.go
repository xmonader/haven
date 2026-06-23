package store

import (
	"bytes"
	"encoding/binary"
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

// FuzzDeltaRoundtrip asserts two properties on arbitrary inputs:
//  1. ApplyDelta(base, MakeDelta(base, target)) == target — reconstruction is
//     always exact, for any byte sequences.
//  2. ApplyDelta never panics on an arbitrary (possibly corrupt) delta stream —
//     it returns an error instead. A corrupt DB row must not crash a read.
func FuzzDeltaRoundtrip(f *testing.F) {
	f.Add([]byte("the quick brown fox"), []byte("the quick red fox jumps"))
	f.Add([]byte(""), []byte("abc"))
	f.Add([]byte("abc"), []byte(""))
	f.Add([]byte(strings.Repeat("ab", 64)), []byte(strings.Repeat("ab", 64)+"z"))

	f.Fuzz(func(t *testing.T, base, target []byte) {
		d := MakeDelta(base, target)
		got, err := ApplyDelta(base, d)
		if err != nil {
			t.Fatalf("roundtrip apply error: %v", err)
		}
		if !bytes.Equal(got, target) {
			t.Fatalf("roundtrip mismatch: got %q want %q", got, target)
		}
		// Feeding the target itself as a (likely invalid) delta must not panic.
		_, _ = ApplyDelta(base, target)
	})
}

func TestApplyDeltaRejectsHugeTarget(t *testing.T) {
	// Hand-craft a delta whose base length matches but whose target length is
	// astronomically large. ApplyDelta must reject it without trying to allocate
	// (a corrupt DB row must not OOM the process).
	base := []byte("small base")
	var d []byte
	var hdr [10]byte
	d = append(d, hdr[:binaryPutUvarint(hdr[:], uint64(len(base)))]...)
	d = append(d, hdr[:binaryPutUvarint(hdr[:], 1<<60)]...) // 1 EiB declared target
	if _, err := ApplyDelta(base, d); err == nil {
		t.Fatal("expected rejection of an absurd target length")
	}
}

func binaryPutUvarint(b []byte, v uint64) int { return binary.PutUvarint(b, v) }
