package protocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
