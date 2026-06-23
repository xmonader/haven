package protocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
	"haven/internal/store"
)

// newServerWithPolicy builds a server whose repo has a signed policy: `admin`
// is admin over refs/**, and refs/branches/** is public-read. It returns an
// authenticated admin client, an anonymous client, and the object store.
func newServerWithPolicy(t *testing.T) (admin, anon *Client, s *object.Store) {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s = object.NewStore(db)

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	if err := policy.Bootstrap(db, s, "admin", pubHex, "age1example", priv); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewServer(db, KindTeam).Handler())
	t.Cleanup(srv.Close)
	admin = NewClient(srv.URL).WithAuth(pubHex, priv)
	anon = NewClient(srv.URL)
	return admin, anon, s
}

func TestAnonReadsPublicRefHidesRestricted(t *testing.T) {
	admin, anon, _ := newServerWithPolicy(t)
	// Admin restricts a haven and publishes a public branch.
	if err := admin.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err != nil {
		t.Fatalf("admin publish branch: %v", err)
	}
	if err := admin.UpdateRef(RefUpdate{Name: "refs/havens/wip", Visibility: ref.Restricted, Target: "h1"}); err != nil {
		t.Fatalf("admin push haven: %v", err)
	}

	refs, err := anon.Refs()
	if err != nil {
		t.Fatalf("anon refs: %v", err)
	}
	saw := map[string]bool{}
	for _, r := range refs {
		saw[r.Name] = true
	}
	if !saw["refs/branches/main"] {
		t.Error("anon should see the public branch")
	}
	if saw["refs/havens/wip"] {
		t.Error("anon must NOT see the restricted haven")
	}
	// refs/policy is always visible so clients can verify the chain.
	if !saw[policy.Ref] {
		t.Error("policy ref should always be visible")
	}
}

// tamperTransport rewrites a request body after the client has signed it,
// simulating a man-in-the-middle altering a signed request.
type tamperTransport struct {
	base http.RoundTripper
	with []byte
}

func (t tamperTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost {
		req.Body = io.NopCloser(bytes.NewReader(t.with))
		req.ContentLength = int64(len(t.with))
	}
	return t.base.RoundTrip(req)
}

func TestTamperedBodyRejected(t *testing.T) {
	admin, _, _ := newServerWithPolicy(t)
	admin.HTTP = &http.Client{Transport: tamperTransport{
		base: http.DefaultTransport,
		with: []byte(`{"name":"refs/branches/main","visibility":"public","target":"evil"}`),
	}}
	if err := admin.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err == nil {
		t.Fatal("a tampered request body must invalidate the signature")
	}
}

// captureTransport records the exact bytes/headers of the first request it
// sees so the test can replay it verbatim.
type captureTransport struct {
	base http.RoundTripper
	req  *http.Request
	body []byte
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.req == nil && req.Method == http.MethodPost {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		clone := req.Clone(req.Context())
		c.req, c.body = clone, body
	}
	return c.base.RoundTrip(req)
}

func TestReplayedRequestRejected(t *testing.T) {
	admin, _, _ := newServerWithPolicy(t)
	cap := &captureTransport{base: http.DefaultTransport}
	admin.HTTP = &http.Client{Transport: cap}

	// First request is legitimate and should succeed.
	if err := admin.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err != nil {
		t.Fatalf("first update should succeed: %v", err)
	}
	if cap.req == nil {
		t.Fatal("no request captured")
	}
	// Replay the captured request verbatim (same nonce, time, signature, body).
	replay := cap.req.Clone(cap.req.Context())
	replay.Body = io.NopCloser(bytes.NewReader(cap.body))
	resp, err := http.DefaultTransport.RoundTrip(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("replayed request must be rejected (nonce already seen)")
	}
}

func TestAnonCannotWrite(t *testing.T) {
	_, anon, _ := newServerWithPolicy(t)
	err := anon.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"})
	if err == nil {
		t.Fatal("anonymous write must be forbidden when a policy exists")
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	_, anon, _ := newServerWithPolicy(t)
	pub, priv, _ := ed25519.GenerateKey(nil)
	stranger := NewClient(anon.BaseURL).WithAuth(hex.EncodeToString(pub), priv)
	if err := stranger.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err == nil {
		t.Fatal("a key outside the keyring must be rejected")
	}
}

func TestAnonCannotPutObject(t *testing.T) {
	_, anon, s := newServerWithPolicy(t)
	content := []byte("anon blob")
	h, _ := s.Put(object.Blob, content)
	// Delete locally so the server is the only copy target; anon push must fail.
	if err := anon.PutObject(h, object.Blob, content); err == nil {
		t.Fatal("anonymous object upload must be forbidden when a policy exists")
	}
}

func TestRestrictedObjectHiddenFromAnon(t *testing.T) {
	admin, anon, s := newServerWithPolicy(t)
	// Build a real commit/tree/blob graph and point a restricted haven at it.
	blob, _ := s.Put(object.Blob, []byte("secret source"))
	tree, err := object.BuildTree(s, map[string]object.FileEntry{
		"app.go": {Hash: blob, Mode: "100644", Type: object.Blob},
	})
	if err != nil {
		t.Fatal(err)
	}
	commit, err := s.PutCommit(object.CommitObj{Tree: tree, Author: "x", Email: "x@e", Message: "wip"})
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.UpdateRef(RefUpdate{Name: "refs/havens/wip", Visibility: ref.Restricted, Target: commit}); err != nil {
		t.Fatal(err)
	}
	// Anon must not be able to fetch the blob reachable only from the restricted haven.
	if _, _, err := anon.GetObject(blob); err == nil {
		t.Fatal("anon must not fetch an object reachable only from a restricted ref")
	}
	// Admin can.
	if _, _, err := admin.GetObject(blob); err != nil {
		t.Fatalf("admin should fetch the object: %v", err)
	}
}

// TestBodyCapRejectsOversized proves the server refuses a request body larger
// than its cap instead of buffering it (a memory-exhaustion DoS vector).
func TestBodyCapRejectsOversized(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	srv := NewServer(db, KindTeam)
	srv.maxBody = 200 // small cap so we needn't allocate megabytes
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Open repo => no policy => anonymous writes are allowed, so a rejection here
	// can only come from the body cap, not from authorization.
	big := bytes.NewReader(make([]byte, 4096))
	resp, err := http.Post(ts.URL+"/refs", "application/json", big)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("oversized body was accepted (status %d); cap not enforced", resp.StatusCode)
	}

	// A body within the cap on the same endpoint still succeeds.
	small, _ := json.Marshal(RefUpdate{Name: "refs/branches/main", Target: "abc", Visibility: ref.Public})
	if len(small) > 200 {
		t.Fatalf("test setup: small body %d exceeds cap", len(small))
	}
	resp2, err := http.Post(ts.URL+"/refs", "application/json", bytes.NewReader(small))
	if err != nil {
		t.Fatalf("post small: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("within-cap body rejected: status %d", resp2.StatusCode)
	}
}

// TestSecretRewriteRequiresWriteAccess proves a keyring member who can READ a
// secret (public-read branch) but has NO write access cannot overwrite that
// secret's ciphertext with different bytes — the lock-out / availability attack
// on the PutSecret upsert. Identical bytes stay idempotent, and a writer (admin)
// can rotate.
func TestSecretRewriteRequiresWriteAccess(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := object.NewStore(db)

	adminPub, adminPriv, _ := ed25519.GenerateKey(nil)
	adminPubHex := hex.EncodeToString(adminPub)
	if err := policy.Bootstrap(db, s, "admin", adminPubHex, "age1admin", adminPriv); err != nil {
		t.Fatal(err)
	}
	// Add bob as an active member with no extra grants: public-read only.
	bobPub, bobPriv, _ := ed25519.GenerateKey(nil)
	bobPubHex := hex.EncodeToString(bobPub)
	if err := policy.Mutate(db, s, "admin", adminPriv, func(p *policy.Policy) error {
		p.Keyring["bob"] = policy.Member{Sign: bobPubHex, Enc: "age1bob", Status: "active"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A secret object reachable from a public branch.
	ctA := []byte("ciphertext-A-original")
	ctB := []byte("ciphertext-B-rewritten")
	secretHash := "0000000000000000000000000000000000000000000000000000000000000001"
	if err := s.PutRaw(secretHash, object.Secret, ctA); err != nil {
		t.Fatal(err)
	}
	tree, err := object.BuildTree(s, map[string]object.FileEntry{
		".env": {Hash: secretHash, Mode: "100644", Type: object.Secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	commit, err := s.PutCommit(object.CommitObj{Tree: tree, Author: "admin", Email: "a@e", Message: "add secret"})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(NewServer(db, KindTeam).Handler())
	t.Cleanup(srv.Close)
	admin := NewClient(srv.URL).WithAuth(adminPubHex, adminPriv)
	bob := NewClient(srv.URL).WithAuth(bobPubHex, bobPriv)

	if err := admin.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: commit}); err != nil {
		t.Fatalf("admin publish: %v", err)
	}

	// Bob (reader, not writer) must NOT be able to rewrite the ciphertext.
	if err := bob.PutObject(secretHash, object.Secret, ctB); err == nil {
		t.Fatal("bob rewrote a secret he has no write access to")
	}
	// The stored bytes must be unchanged.
	if _, got, _ := s.Get(secretHash); !bytes.Equal(got, ctA) {
		t.Fatal("ciphertext changed despite rejected rewrite")
	}
	// Idempotent re-upload of identical bytes is allowed for anyone authenticated.
	if err := bob.PutObject(secretHash, object.Secret, ctA); err != nil {
		t.Fatalf("bob's identical re-upload rejected: %v", err)
	}
	// Admin (writer of the ref reaching it) can rotate to new ciphertext.
	if err := admin.PutObject(secretHash, object.Secret, ctB); err != nil {
		t.Fatalf("admin rotate rejected: %v", err)
	}
	if _, got, _ := s.Get(secretHash); !bytes.Equal(got, ctB) {
		t.Fatal("admin rotation did not propagate")
	}
}

// TestPinnedPolicyRootGatesBootstrap proves that an un-bootstrapped server with
// a pinned --policy-root rejects a first policy whose root signing key doesn't
// match (the takeover vector), and accepts one that does.
func TestPinnedPolicyRootGatesBootstrap(t *testing.T) {
	// Donor repo: a real bootstrapped chain whose root signer is adminA.
	ddb, err := store.Open(t.TempDir() + "/d.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ddb.Close() })
	ds := object.NewStore(ddb)
	pubA, privA, _ := ed25519.GenerateKey(nil)
	hexA := hex.EncodeToString(pubA)
	if err := policy.Bootstrap(ddb, ds, "admin", hexA, "age1a", privA); err != nil {
		t.Fatal(err)
	}
	headA, _ := ref.Resolve(ddb, policy.Ref)
	chainA, _ := policy.ChainHashes(ddb, ds)

	// Fresh server DB: copy the chain objects in, but leave the policy ref unset
	// (so the server is un-bootstrapped: curHead == "").
	sdb, err := store.Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sdb.Close() })
	ss := object.NewStore(sdb)
	for h := range chainA {
		typ, content, err := ds.Get(h)
		if err != nil {
			t.Fatal(err)
		}
		if err := ss.PutRaw(h, typ, content); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer(sdb, KindTeam)
	pubB, _, _ := ed25519.GenerateKey(nil)
	srv.RequirePolicyRoot(hex.EncodeToString(pubB)) // pin a DIFFERENT root
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Anonymous client (no policy yet => open) tries to install policy headA.
	c := NewClient(ts.URL)
	if err := c.UpdateRef(RefUpdate{Name: policy.Ref, Visibility: ref.Policy, Target: headA}); err == nil {
		t.Fatal("a first policy whose root doesn't match the pin must be rejected")
	}
	if cur, _ := ref.Resolve(sdb, policy.Ref); cur != "" {
		t.Fatal("rejected policy must not have been installed")
	}

	// Pin the correct root and retry: now it's accepted.
	srv.RequirePolicyRoot(hexA)
	if err := c.UpdateRef(RefUpdate{Name: policy.Ref, Visibility: ref.Policy, Target: headA}); err != nil {
		t.Fatalf("a first policy matching the pinned root should be accepted: %v", err)
	}
}

// TestOriginEnforcementRejectsForeignHost proves that with a pinned origin, a
// request whose signed/sent host doesn't match THIS server's origin is rejected
// (the cross-server replay defense), while a matching origin is accepted.
func TestOriginEnforcementRejectsForeignHost(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := object.NewStore(db)
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	if err := policy.Bootstrap(db, s, "admin", pubHex, "age1a", priv); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(db, KindTeam)
	srv.RequireOrigin("someone-else.example:9999") // not the host the client will dial
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	admin := NewClient(ts.URL).WithAuth(pubHex, priv)
	if err := admin.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err == nil {
		t.Fatal("request to a server whose pinned origin differs from the dialed host must be rejected")
	}

	// Pin the origin to the host the client actually dials: now accepted.
	host := strings.TrimPrefix(ts.URL, "http://")
	srv.RequireOrigin(host)
	if err := admin.UpdateRef(RefUpdate{Name: "refs/branches/main", Visibility: ref.Public, Target: "v1"}); err != nil {
		t.Fatalf("matching origin should be accepted: %v", err)
	}
}
