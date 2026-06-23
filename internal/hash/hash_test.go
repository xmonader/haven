package hash

import "testing"

func TestOfIsDeterministicAndTypeScoped(t *testing.T) {
	a := Of("blob", []byte("hello"))
	b := Of("blob", []byte("hello"))
	if a != b {
		t.Fatal("Of must be deterministic for identical input")
	}
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(a))
	}
	// Same payload, different type => different hash (namespacing).
	if Of("blob", []byte("hello")) == Of("secret", []byte("hello")) {
		t.Fatal("type prefix must change the hash")
	}
	// Different payload => different hash.
	if Of("blob", []byte("hello")) == Of("blob", []byte("world")) {
		t.Fatal("different payloads must differ")
	}
	// Length is part of the preimage: "ab"+"" vs "a"+"b" must not collide.
	if Of("blob", []byte("ab")) == Of("blob", []byte("a")) {
		t.Fatal("unexpected collision")
	}
}

func TestIsHash(t *testing.T) {
	good := Of("blob", []byte("x"))
	if !IsHash(good) {
		t.Errorf("%q should be a hash", good)
	}
	for _, bad := range []string{"", "xyz", "ABCDEF", good + "0", good[:63], "main"} {
		if IsHash(bad) {
			t.Errorf("%q should not be a hash", bad)
		}
	}
}
