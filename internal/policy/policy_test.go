package policy

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func keypair(t *testing.T) (pubHex string, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(pub), priv
}

// samplePolicy builds a v0 with alice as admin, public read on branches, and a
// deployers group; staging is restricted to deployers.
func samplePolicy(t *testing.T) (*Policy, ed25519.PrivateKey) {
	aSign, aPriv := keypair(t)
	p := &Policy{
		Keyring: map[string]Member{
			"alice": {Sign: aSign, Enc: "age1alice", Status: "active"},
			"bob":   {Sign: "bob", Enc: "age1bob", Status: "active"},
			"carol": {Sign: "carol", Enc: "age1carol", Status: "active"},
		},
		Groups: map[string][]string{"deployers": {"alice", "bob"}},
		Grants: []Grant{
			{ID: "g1", Subject: "alice", Verb: Admin, Resource: "refs/**"},
			{ID: "g2", Subject: "*", Verb: Read, Resource: "refs/branches/**"},
			{ID: "g3", Subject: "deployers", Verb: Read, Resource: "refs/branches/staging"},
		},
		Restricted: []string{"refs/branches/staging"},
	}
	p.Sign("alice", aPriv)
	return p, aPriv
}

func TestVerbHierarchy(t *testing.T) {
	p, _ := samplePolicy(t)
	// alice has admin on refs/** -> implies write and read everywhere.
	for _, v := range []string{Read, Write, Force, Admin} {
		if !p.Eval("alice", v, "refs/havens/secret") {
			t.Errorf("alice admin should satisfy %s", v)
		}
	}
}

func TestPublicReadAndDefaultDeny(t *testing.T) {
	p, _ := samplePolicy(t)
	if !p.Eval("carol", Read, "refs/branches/main") {
		t.Error("public read should let carol read main")
	}
	if p.Eval("carol", Write, "refs/branches/main") {
		t.Error("carol must not have write (default deny)")
	}
	if p.Eval("carol", Read, "refs/havens/alice-secret") {
		t.Error("carol must not read a haven (no grant)")
	}
}

func TestRestrictRemovesPublic(t *testing.T) {
	p, _ := samplePolicy(t)
	// staging is restricted to deployers: bob (deployer) yes, carol no.
	if !p.Eval("bob", Read, "refs/branches/staging") {
		t.Error("deployer bob should read staging")
	}
	if p.Eval("carol", Read, "refs/branches/staging") {
		t.Error("non-deployer carol must NOT read restricted staging")
	}
}

func TestRecipientsAreRefScoped(t *testing.T) {
	p, _ := samplePolicy(t)

	// Public branch -> all active members.
	mainR := p.Recipients("refs/branches/main")
	if len(mainR) != 3 {
		t.Errorf("main recipients = %v, want all 3 members", mainR)
	}
	// Restricted staging -> only deployers (alice, bob).
	st := p.Recipients("refs/branches/staging")
	if len(st) != 2 || !contains(st, "age1alice") || !contains(st, "age1bob") {
		t.Errorf("staging recipients = %v, want alice+bob", st)
	}
	if contains(st, "age1carol") {
		t.Error("carol must not be a recipient of staging secrets")
	}
	// Haven owned by alice (admin) -> only alice.
	hv := p.Recipients("refs/havens/spike")
	if len(hv) != 1 || !contains(hv, "age1alice") {
		t.Errorf("haven recipients = %v, want only alice", hv)
	}
}

func TestSignatureTamperDetected(t *testing.T) {
	p, _ := samplePolicy(t)
	if err := p.Verify(nil); err != nil {
		t.Fatalf("freshly signed v0 should verify: %v", err)
	}
	// Tamper a grant without re-signing.
	p.Grants[0].Subject = "carol"
	if err := p.Verify(nil); err == nil {
		t.Fatal("tampered policy must fail verification")
	}
}

func TestRevokedMemberNotRecipient(t *testing.T) {
	p, _ := samplePolicy(t)
	m := p.Keyring["bob"]
	m.Status = "revoked"
	p.Keyring["bob"] = m
	if contains(p.Recipients("refs/branches/staging"), "age1bob") {
		t.Error("revoked member must not be a recipient")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
