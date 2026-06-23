package policy

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func base() *Policy {
	return &Policy{
		Keyring: map[string]Member{
			"alice": {Sign: "a", Enc: "age1alice", Status: "active"},
			"bob":   {Sign: "b", Enc: "age1bob", Status: "active"},
			"eve":   {Sign: "e", Enc: "age1eve", Status: "revoked"},
		},
		Groups: map[string][]string{"devs": {"alice", "bob"}},
	}
}

func TestEvalVerbHierarchy(t *testing.T) {
	p := base()
	p.Grants = []Grant{{ID: "g", Subject: "alice", Verb: Admin, Resource: "refs/**"}}
	// admin satisfies read/write/force/admin.
	for _, v := range []string{Read, Write, Force, Admin} {
		if !p.Eval("alice", v, "refs/branches/main") {
			t.Errorf("admin should satisfy %s", v)
		}
	}
	// A write grant does NOT satisfy admin.
	p.Grants = []Grant{{ID: "g", Subject: "bob", Verb: Write, Resource: "refs/**"}}
	if p.Eval("bob", Admin, "refs/branches/main") {
		t.Error("write must not satisfy admin")
	}
	if !p.Eval("bob", Read, "refs/branches/main") {
		t.Error("write should satisfy read")
	}
}

func TestEvalPublicWildcardAndRestriction(t *testing.T) {
	p := base()
	p.Grants = []Grant{{ID: "pub", Subject: "*", Verb: Read, Resource: "refs/branches/**"}}
	// Anonymous ("") gets public read.
	if !p.Eval("", Read, "refs/branches/main") {
		t.Error("anonymous should read a public branch")
	}
	// Restrict the staging branch: the wildcard no longer applies.
	p.Restricted = []string{"refs/branches/staging"}
	if p.Eval("", Read, "refs/branches/staging") {
		t.Error("wildcard must be suppressed on a restricted ref")
	}
	if !p.Eval("", Read, "refs/branches/main") {
		t.Error("restriction must not leak to other refs")
	}
}

func TestEvalGroups(t *testing.T) {
	p := base()
	p.Grants = []Grant{{ID: "g", Subject: "devs", Verb: Write, Resource: "refs/branches/**"}}
	if !p.Eval("alice", Write, "refs/branches/main") {
		t.Error("group member alice should have write")
	}
	if p.Eval("carol", Write, "refs/branches/main") {
		t.Error("non-member must not inherit group grant")
	}
}

func TestRecipientsPublicVsRestrictedAndRevoked(t *testing.T) {
	p := base()
	p.Grants = []Grant{
		{ID: "pub", Subject: "*", Verb: Read, Resource: "refs/branches/**"},
		{ID: "sec", Subject: "devs", Verb: Read, Resource: "refs/havens/secret"},
	}
	// Public branch: all ACTIVE members are recipients (eve revoked excluded).
	pubR := p.Recipients("refs/branches/main")
	if !hasStr(pubR, "age1alice") || !hasStr(pubR, "age1bob") {
		t.Errorf("public recipients missing active members: %v", pubR)
	}
	if hasStr(pubR, "age1eve") {
		t.Error("revoked member must never be a recipient")
	}
	// Restricted ref: only the granted group.
	p.Restricted = []string{"refs/havens/secret"}
	secR := p.Recipients("refs/havens/secret")
	if !hasStr(secR, "age1alice") || !hasStr(secR, "age1bob") || hasStr(secR, "age1eve") {
		t.Errorf("restricted recipients wrong: %v", secR)
	}
	// A ref nobody can read has no recipients.
	if got := p.Recipients("refs/havens/nobody"); len(got) != 0 {
		t.Errorf("expected no recipients, got %v", got)
	}
}

func TestChainVerifyDetectsTampering(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	p := base()
	p.Keyring["alice"] = Member{Sign: hex.EncodeToString(pub), Enc: "age1alice", Status: "active"}
	p.Grants = []Grant{{ID: "root", Subject: "alice", Verb: Admin, Resource: "refs/**"}}
	p.Sign("alice", priv)

	if err := p.Verify(nil); err != nil {
		t.Fatalf("freshly signed v0 should verify: %v", err)
	}
	// Tamper after signing: a new grant must invalidate the signature.
	p.Grants = append(p.Grants, Grant{ID: "evil", Subject: "eve", Verb: Admin, Resource: "refs/**"})
	if err := p.Verify(nil); err == nil {
		t.Fatal("tampered policy must fail verification")
	}
}

func hasStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
