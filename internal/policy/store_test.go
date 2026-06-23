package policy

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/store"
)

func newPolicyStore(t *testing.T) (*sql.DB, *object.Store) {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, object.NewStore(db)
}

func genKey(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(pub), priv
}

// TestBootstrapMutateAndChain covers the happy path of the chain-persistence
// core: Bootstrap, Mutate, Load, save, clone, VerifyChain, ChainHashes,
// RootSignKey.
func TestBootstrapMutateAndChain(t *testing.T) {
	db, st := newPolicyStore(t)
	alicePub, alicePriv := genKey(t)

	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// A second bootstrap must be refused.
	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err == nil {
		t.Fatal("second bootstrap should fail")
	}

	// Admin alice extends the chain with a new member.
	if err := Mutate(db, st, "alice", alicePriv, func(p *Policy) error {
		p.Keyring["bob"] = Member{Sign: "b", Enc: "age1bob", Status: "active"}
		return nil
	}); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	n, err := VerifyChain(db, st)
	if err != nil || n != 2 {
		t.Fatalf("VerifyChain = (%d, %v), want (2, nil)", n, err)
	}
	hashes, err := ChainHashes(db, st)
	if err != nil || len(hashes) != 2 {
		t.Fatalf("ChainHashes len = %d (err %v), want 2", len(hashes), err)
	}

	head, _ := ref.Resolve(db, Ref)
	rk, err := RootSignKey(st, head)
	if err != nil || rk != alicePub {
		t.Fatalf("RootSignKey = (%q, %v), want (%q, nil)", rk, err, alicePub)
	}

	cur, err := Load(db, st)
	if err != nil || cur == nil || cur.Version != 1 {
		t.Fatalf("Load head version = %v (err %v), want v1", cur, err)
	}
	if _, ok := cur.Keyring["bob"]; !ok {
		t.Fatal("bob missing from head policy")
	}
}

// TestMutateRejectsNonAdminSigner proves authority is evaluated against the
// PARENT: a member who is not admin cannot extend the policy, even with a valid
// signature.
func TestMutateRejectsNonAdminSigner(t *testing.T) {
	db, st := newPolicyStore(t)
	alicePub, alicePriv := genKey(t)
	bobPub, bobPriv := genKey(t)

	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err != nil {
		t.Fatal(err)
	}
	// Alice adds bob as an ACTIVE member with a real key, but grants him no admin.
	if err := Mutate(db, st, "alice", alicePriv, func(p *Policy) error {
		p.Keyring["bob"] = Member{Sign: bobPub, Enc: "age1bob", Status: "active"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Bob (valid signature, no admin) tries to mutate → must be rejected.
	err := Mutate(db, st, "bob", bobPriv, func(p *Policy) error {
		p.Grants = append(p.Grants, Grant{ID: "self", Subject: "bob", Verb: Admin, Resource: "refs/**"})
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "admin") {
		t.Fatalf("expected admin-authority rejection, got %v", err)
	}
}

// TestVerifyExtensionAcceptsLinearExtension confirms a chain that genuinely
// extends the current head verifies.
func TestVerifyExtensionAcceptsLinearExtension(t *testing.T) {
	db, st := newPolicyStore(t)
	alicePub, alicePriv := genKey(t)
	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err != nil {
		t.Fatal(err)
	}
	head0, _ := ref.Resolve(db, Ref)
	if err := Mutate(db, st, "alice", alicePriv, func(p *Policy) error {
		p.Keyring["bob"] = Member{Sign: "b", Enc: "age1bob", Status: "active"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	head1, _ := ref.Resolve(db, Ref)

	if err := VerifyExtension(st, head1, head0); err != nil {
		t.Fatalf("linear extension should verify: %v", err)
	}
	if err := VerifyExtension(st, head1, ""); err != nil {
		t.Fatalf("extension with no current head should verify: %v", err)
	}
}

// TestVerifyExtensionRejectsHistoryRewrite is the security linchpin: a validly
// signed but FORKED chain (one that drops the server's current head) must be
// refused, so an admin cannot silently rewrite signed policy history.
func TestVerifyExtensionRejectsHistoryRewrite(t *testing.T) {
	db, st := newPolicyStore(t)
	alicePub, alicePriv := genKey(t)
	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err != nil {
		t.Fatal(err)
	}
	head0, _ := ref.Resolve(db, Ref)
	// The server's real current head: v1 adding bob.
	if err := Mutate(db, st, "alice", alicePriv, func(p *Policy) error {
		p.Keyring["bob"] = Member{Sign: "b", Enc: "age1bob", Status: "active"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	head1, _ := ref.Resolve(db, Ref)

	// Craft a competing v1' that forks from v0 (parent=head0) — validly signed by
	// admin alice, but with different content, and NOT descending from head1.
	v0, _ := loadObject(st, head0)
	forkP := clone(v0)
	forkP.Version = 1
	forkP.Parent = head0
	forkP.Keyring["carol"] = Member{Sign: "c", Enc: "age1carol", Status: "active"}
	forkP.Sign("alice", alicePriv)
	payload, _ := json.Marshal(&forkP)
	forkHash, err := st.Put(object.Policy, payload)
	if err != nil {
		t.Fatal(err)
	}

	// The fork is itself a valid chain...
	if err := VerifyExtension(st, forkHash, ""); err != nil {
		t.Fatalf("fork should be a valid chain on its own: %v", err)
	}
	// ...but it does NOT extend the server's current head → must be refused.
	err = VerifyExtension(st, forkHash, head1)
	if err == nil || !strings.Contains(err.Error(), "history rewrite") {
		t.Fatalf("expected history-rewrite refusal, got %v", err)
	}
}

// TestVerifyExtensionRejectsTamperedSignature confirms a chain whose signature
// was altered is refused.
func TestVerifyExtensionRejectsTamperedSignature(t *testing.T) {
	db, st := newPolicyStore(t)
	alicePub, alicePriv := genKey(t)
	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err != nil {
		t.Fatal(err)
	}
	head0, _ := ref.Resolve(db, Ref)

	// Build a v1 with a deliberately wrong signature.
	v0, _ := loadObject(st, head0)
	bad := clone(v0)
	bad.Version = 1
	bad.Parent = head0
	bad.Sign("alice", alicePriv)
	bad.Sig = strings.Repeat("00", ed25519.SignatureSize) // corrupt the signature
	payload, _ := json.Marshal(&bad)
	badHash, err := st.Put(object.Policy, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyExtension(st, badHash, head0); err == nil {
		t.Fatal("tampered signature should be refused")
	}
}

// TestRecipientsForAndIsSecretRef covers the bootstrap.go helpers.
func TestRecipientsForAndIsSecretRef(t *testing.T) {
	db, st := newPolicyStore(t)
	alicePub, alicePriv := genKey(t)
	if err := Bootstrap(db, st, "alice", alicePub, "age1alice", alicePriv); err != nil {
		t.Fatal(err)
	}
	// Public branch: alice (the only active member) is a recipient.
	recips, err := RecipientsFor(db, st, "refs/branches/main")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range recips {
		if r == "age1alice" {
			found = true
		}
	}
	if !found {
		t.Fatalf("alice should be a recipient of a public branch: %v", recips)
	}

	if ok, _ := IsSecretRef(db, st, "refs/branches/main"); ok {
		t.Fatal("main is not a secret ref")
	}
	if err := Mutate(db, st, "alice", alicePriv, func(p *Policy) error {
		p.SecretRefs = append(p.SecretRefs, "refs/havens/vault")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsSecretRef(db, st, "refs/havens/vault"); err != nil || !ok {
		t.Fatalf("vault should be a secret ref (err %v)", err)
	}
}
