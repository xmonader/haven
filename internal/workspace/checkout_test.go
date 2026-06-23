package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"

	"haven/internal/hash"
	"haven/internal/identity"
	"haven/internal/object"
	"haven/internal/secret"
	"haven/internal/store"
)

// newSecretStore opens a temp store and returns it plus a recipient identity
// "alice" and a non-recipient identity "bob".
func newSecretStore(t *testing.T) (*object.Store, *identity.Identity, *identity.Identity) {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	aliceEnc, _ := age.GenerateX25519Identity()
	bobEnc, _ := age.GenerateX25519Identity()
	return object.NewStore(db),
		&identity.Identity{X25519: aliceEnc},
		&identity.Identity{X25519: bobEnc}
}

// putSecret encrypts plaintext to recipient and stores it under the hash of
// claimedPlaintext (normally == plaintext; differ to forge), returning the
// FileEntry that references it.
func putSecret(t *testing.T, s *object.Store, plaintext, claimedPlaintext string, recipient *identity.Identity) object.FileEntry {
	t.Helper()
	ct, err := secret.Encrypt([]byte(plaintext), []string{recipient.X25519.Recipient().String()})
	if err != nil {
		t.Fatal(err)
	}
	h := hash.Of(string(object.Secret), []byte(claimedPlaintext))
	if err := s.PutRaw(h, object.Secret, ct); err != nil {
		t.Fatal(err)
	}
	return object.FileEntry{Hash: h, Mode: "100644", Type: object.Secret}
}

// C1: a decrypted secret must be written 0600, never world-readable.
func TestSecretWrittenPrivate(t *testing.T) {
	s, alice, _ := newSecretStore(t)
	fe := putSecret(t, s, "TOKEN=hunter2\n", "TOKEN=hunter2\n", alice)
	root := t.TempDir()
	if err := WriteEntry(root, s, "config/.env", fe, alice); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	full := filepath.Join(root, "config/.env")
	info, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("decrypted secret mode = %o, want 0600 (must not be world-readable)", perm)
	}
	if got, _ := os.ReadFile(full); string(got) != "TOKEN=hunter2\n" {
		t.Fatalf("decrypted content = %q", got)
	}
	// The directory created to hold a secret must also be private.
	di, _ := os.Stat(filepath.Join(root, "config"))
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Fatalf("secret parent dir mode = %o, want 0700", perm)
	}
}

// C2: ciphertext that decrypts to plaintext NOT matching the object hash is
// forged content and must be refused, leaving no file behind.
func TestForgedSecretRefused(t *testing.T) {
	s, alice, _ := newSecretStore(t)
	// Encrypt "EVIL" but store it under the hash claiming to be "REAL".
	fe := putSecret(t, s, "EVIL=payload\n", "REAL=value\n", alice)
	root := t.TempDir()
	err := WriteEntry(root, s, ".env", fe, alice)
	if err == nil {
		t.Fatal("forged secret (hash mismatch) must be refused")
	}
	if _, statErr := os.Stat(filepath.Join(root, ".env")); !os.IsNotExist(statErr) {
		t.Fatal("forged secret must not be written to disk")
	}
}

// H1: a non-recipient (or nil identity) gets the locked notice with no error;
// but a genuinely corrupt ciphertext is an error, never silently masked.
func TestDecryptErrorsNotMaskedAsLocked(t *testing.T) {
	s, alice, bob := newSecretStore(t)
	fe := putSecret(t, s, "S=1\n", "S=1\n", alice)
	root := t.TempDir()

	// bob is not a recipient -> locked notice, no error.
	if err := WriteEntry(root, s, "a.env", fe, bob); err != nil {
		t.Fatalf("non-recipient should get locked notice, got error: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "a.env")); string(got) != lockedNotice {
		t.Fatalf("non-recipient file = %q, want locked notice", got)
	}
	// nil identity (e.g. no key configured) -> locked notice, no error.
	if err := WriteEntry(root, s, "b.env", fe, nil); err != nil {
		t.Fatalf("nil identity should get locked notice, got error: %v", err)
	}

	// Corrupt ciphertext stored under a real-looking hash: decryption fails with
	// a NON-"not a recipient" error, which must surface, not be masked.
	garbage := []byte("this is not a valid age file")
	gh := hash.Of(string(object.Secret), []byte("whatever\n"))
	if err := s.PutRaw(gh, object.Secret, garbage); err != nil {
		t.Fatal(err)
	}
	cf := object.FileEntry{Hash: gh, Mode: "100644", Type: object.Secret}
	err := WriteEntry(root, s, "corrupt.env", cf, alice)
	if err == nil {
		t.Fatal("corrupt ciphertext must error, not be masked as a locked notice")
	}
	if _, statErr := os.Stat(filepath.Join(root, "corrupt.env")); !os.IsNotExist(statErr) {
		t.Fatal("corrupt secret must not write a (locked-notice) file over the path")
	}
}

// H4: atomic writes leave no temp files behind on success, and a regular
// (non-secret) blob round-trips with conventional perms.
func TestAtomicWriteNoTempLeak(t *testing.T) {
	s, _, _ := newSecretStore(t)
	h, err := s.Put(object.Blob, []byte("hello world\n"))
	if err != nil {
		t.Fatal(err)
	}
	fe := object.FileEntry{Hash: h, Mode: "100644", Type: object.Blob}
	root := t.TempDir()
	if err := WriteEntry(root, s, "readme.txt", fe, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "readme.txt")); string(got) != "hello world\n" {
		t.Fatalf("blob content = %q", got)
	}
	info, _ := os.Stat(filepath.Join(root, "readme.txt"))
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Fatalf("blob mode = %o, want 0644", perm)
	}
	leaks, _ := filepath.Glob(filepath.Join(root, ".haven-tmp-*"))
	if len(leaks) != 0 {
		t.Fatalf("atomic write leaked temp files: %v", leaks)
	}
}

// TestCheckoutAndIsClean covers the materialize + clean-check round trip:
// a tree with a plaintext file and a secret is checked out for a recipient,
// reports clean against its own tree, dirty after an edit, and removals are
// applied when switching trees.
func TestCheckoutAndIsClean(t *testing.T) {
	s, alice, _ := newSecretStore(t)
	blob, _ := s.Put(object.Blob, []byte("package main\n"))
	sfe := putSecret(t, s, "TOKEN=abc\n", "TOKEN=abc\n", alice)
	tree, err := object.BuildTree(s, map[string]object.FileEntry{
		"main.go": {Hash: blob, Mode: "100644", Type: object.Blob},
		".env":    sfe,
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := Checkout(root, s, "", tree, alice); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "main.go")); string(b) != "package main\n" {
		t.Fatalf("main.go = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".env")); string(b) != "TOKEN=abc\n" {
		t.Fatalf(".env = %q", b)
	}

	marks := []string{".env"}
	clean, err := IsClean(root, s, tree, marks)
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Fatal("freshly checked-out tree should be clean")
	}

	// Edit the plaintext file: now dirty.
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if clean, _ := IsClean(root, s, tree, marks); clean {
		t.Fatal("edited working tree should be dirty")
	}

	// Switch to a tree without main.go: it must be removed.
	tree2, err := object.BuildTree(s, map[string]object.FileEntry{".env": sfe})
	if err != nil {
		t.Fatal(err)
	}
	if err := Checkout(root, s, tree, tree2, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "main.go")); !os.IsNotExist(err) {
		t.Fatal("main.go should have been removed on checkout of a tree without it")
	}
}
