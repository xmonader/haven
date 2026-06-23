package secret

import (
	"testing"

	"filippo.io/age"
)

func TestMatchGlobs(t *testing.T) {
	marks := []string{".env", ".env.*", "*.pem", "secrets/**", "**/credentials.json"}
	hit := []string{".env", ".env.production", "server.pem", "config/server.pem",
		"secrets/db/password", "a/b/credentials.json"}
	miss := []string{"main.go", "env", "readme.md", "secret.txt", "credentials.json.bak"}
	for _, p := range hit {
		if !Match(p, marks) {
			t.Errorf("%q should match a secret mark", p)
		}
	}
	for _, p := range miss {
		if Match(p, marks) {
			t.Errorf("%q should NOT match", p)
		}
	}
}

func TestEncryptDecryptRoundTripAndNonRecipient(t *testing.T) {
	alice, _ := age.GenerateX25519Identity()
	bob, _ := age.GenerateX25519Identity()

	plain := []byte("DB_PASSWORD=hunter2")
	ct, err := Encrypt(plain, []string{alice.Recipient().String()})
	if err != nil {
		t.Fatal(err)
	}
	// Ciphertext must not contain the plaintext.
	if string(ct) == string(plain) {
		t.Fatal("ciphertext equals plaintext")
	}
	// Recipient decrypts.
	got, err := Decrypt(ct, alice)
	if err != nil || string(got) != string(plain) {
		t.Fatalf("recipient decrypt = %q, %v", got, err)
	}
	// Non-recipient fails.
	if _, err := Decrypt(ct, bob); err == nil {
		t.Fatal("a non-recipient must not decrypt the secret")
	}
}
