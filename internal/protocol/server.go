package protocol

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
	"haven/internal/repo"
)

// Server serves a single repository over HTTP.
type Server struct {
	db      *sql.DB
	store   *object.Store
	kind    string
	maxBody int64 // request-body cap; defaults to MaxRequestBytes

	// policyRoot, when set, is the hex ed25519 signing key the root of an
	// incoming FIRST policy must match. It stops an arbitrary client from
	// claiming an un-bootstrapped server. Empty = open bootstrap (dev / trusted
	// network only).
	policyRoot string

	mu    sync.Mutex
	gen   uint64                // bumped on every ref change
	cache map[string]reachEntry // actor -> reachable object set
}

// reachEntry caches the set of objects an actor may fetch, valid for one gen.
type reachEntry struct {
	gen  uint64
	objs map[string]bool
}

// NewServer builds a server over an open database.
func NewServer(db *sql.DB, kind string) *Server {
	return &Server{db: db, store: object.NewStore(db), kind: kind, maxBody: MaxRequestBytes, cache: map[string]reachEntry{}}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info", s.getInfo)
	mux.HandleFunc("GET /refs", s.getRefs)
	mux.HandleFunc("POST /refs", s.postRefs)
	mux.HandleFunc("GET /objects/{hash}", s.getObject)
	mux.HandleFunc("PUT /objects/{hash}", s.putObject)
	return s.limitBody(mux)
}

// limitBody caps every request body at s.maxBody. MaxBytesReader makes an
// oversized body fail on read (and closes the connection), so authActor's
// io.ReadAll returns an error and the request is rejected before it can exhaust
// memory.
func (s *Server) limitBody(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBody)
		h.ServeHTTP(w, r)
	})
}

// authActor reads and authenticates a request against the policy keyring. It
// returns the actor name ("" for anonymous), the buffered request body (so the
// handler can reuse it — the signature covers it), and whether the request is
// allowed to proceed. Missing auth headers are anonymous (public access only);
// a present-but-invalid signature is rejected.
func (s *Server) authActor(p *policy.Policy, r *http.Request) (actor string, body []byte, ok bool) {
	var err error
	body, err = io.ReadAll(r.Body)
	if err != nil {
		return "", nil, false // body exceeded MaxRequestBytes (or read failed)
	}
	if p == nil {
		return "", body, true // open repo: no policy, no enforcement
	}
	pub := r.Header.Get(HdrPub)
	ts := r.Header.Get(HdrTime)
	sigHex := r.Header.Get(HdrSig)
	nonce := r.Header.Get(HdrNonce)
	if pub == "" && ts == "" && sigHex == "" {
		return "", body, true // anonymous: public access only
	}
	tsec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || abs(time.Now().Unix()-tsec) > MaxSkewSeconds {
		return "", body, false
	}
	pubBytes, err := hex.DecodeString(pub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return "", body, false
	}
	if nonce == "" {
		return "", body, false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil || !ed25519.Verify(pubBytes, canonicalRequest(r.Method, r.URL.Path, ts, bodyHash(body), nonce), sig) {
		return "", body, false
	}
	if !s.acceptNonce(nonce) {
		return "", body, false // replay of an already-seen request
	}
	for name, m := range p.Keyring {
		if m.Sign == pub && m.Status != "revoked" {
			return name, body, true
		}
	}
	return "", body, false // signed by an unknown/revoked key
}

// acceptNonce durably records a freshly-seen nonce and reports whether it was
// new. Persisted to the DB so replay protection survives restarts and holds
// across processes sharing the repo. The recorded time is the SERVER's receive
// time, not the client-supplied timestamp, so a client cannot influence when
// its nonce becomes evictable and thus the replay window. A row is only safe to
// evict once it is older than the skew window (a request that old fails the time
// check anyway). The INSERT is the atomic guard: a duplicate violates the
// primary key.
func (s *Server) acceptNonce(nonce string) bool {
	now := time.Now().Unix()
	s.db.Exec(`DELETE FROM seen_nonces WHERE seen_at < ?`, now-MaxSkewSeconds)
	res, err := s.db.Exec(`INSERT INTO seen_nonces(nonce, seen_at) VALUES(?,?)`, nonce, now)
	if err != nil {
		return false // primary-key conflict => replay
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (s *Server) getInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Info{DefaultBranch: ref.ShortName(repo.DefaultBranch), ServerKind: s.kind})
}

func (s *Server) getRefs(w http.ResponseWriter, r *http.Request) {
	p, _ := policy.Load(s.db, s.store)
	actor, _, ok := s.authActor(p, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	refs, err := ref.List(s.db)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]RefInfo, 0, len(refs))
	for _, rf := range refs {
		if !canRead(p, actor, rf.Name) {
			continue
		}
		out = append(out, RefInfo{Name: rf.Name, Visibility: rf.Visibility, Target: rf.Target})
	}
	writeJSON(w, out)
}

func (s *Server) postRefs(w http.ResponseWriter, r *http.Request) {
	p, _ := policy.Load(s.db, s.store)
	actor, body, ok := s.authActor(p, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var u RefUpdate
	if err := json.Unmarshal(body, &u); err != nil {
		http.Error(w, "bad request: "+err.Error(), 400)
		return
	}
	if s.kind == KindTeam && u.Visibility == ref.Private {
		http.Error(w, "team server refuses private refs", http.StatusForbidden)
		return
	}
	// Policy updates: verify the incoming signed chain rather than a write grant.
	if u.Name == policy.Ref {
		if err := s.verifyIncomingPolicy(u.Target, p); err != nil {
			http.Error(w, "policy rejected: "+err.Error(), http.StatusForbidden)
			return
		}
	} else if p != nil && !p.Eval(actor, policy.Write, u.Name) {
		http.Error(w, "forbidden: no write access to "+u.Name, http.StatusForbidden)
		return
	}

	ok, err := ref.CompareAndSwap(s.db, u.Name, u.OldTarget, u.Target, u.Visibility)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if !ok {
		http.Error(w, "ref update conflict: stale old_target", http.StatusConflict)
		return
	}
	s.mu.Lock()
	s.gen++ // invalidate cached reachability
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// verifyIncomingPolicy checks that the just-uploaded policy object at head forms
// a valid extension of the server's current policy.
func (s *Server) verifyIncomingPolicy(head string, _ *policy.Policy) error {
	curHead, err := ref.Resolve(s.db, policy.Ref)
	if err != nil {
		return err
	}
	if err := policy.VerifyExtension(s.store, head, curHead); err != nil {
		return err
	}
	// Bootstrap protection: if this server has no policy yet and an operator
	// pinned an expected root, the first policy's root signing key must match.
	if curHead == "" && s.policyRoot != "" {
		root, err := policy.RootSignKey(s.store, head)
		if err != nil {
			return err
		}
		if root != s.policyRoot {
			return fmt.Errorf("first policy root %s does not match the pinned --policy-root", root)
		}
	}
	return nil
}

// RequirePolicyRoot pins the hex ed25519 signing key that the root of a first
// (bootstrap) policy must match. With no pin, an un-bootstrapped server accepts
// the first valid policy pushed to it (open bootstrap).
func (s *Server) RequirePolicyRoot(hexKey string) { s.policyRoot = hexKey }

func (s *Server) getObject(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	p, _ := policy.Load(s.db, s.store)
	actor, _, ok := s.authActor(p, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if p != nil && !s.readableObject(p, actor, hash) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	typ, content, err := s.store.Get(hash)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set(HeaderType, string(typ))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
}

func (s *Server) putObject(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	typ := r.Header.Get(HeaderType)
	if typ == "" {
		http.Error(w, "missing "+HeaderType, 400)
		return
	}
	p, _ := policy.Load(s.db, s.store)
	// Uploading objects requires authentication once a policy exists (anonymous
	// callers may read public data but not write into the store).
	actor, content, ok := s.authActor(p, r)
	if !ok || (p != nil && actor == "") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if object.Type(typ) == object.Secret {
		// A rotated secret keeps its plaintext-derived hash but carries new
		// ciphertext, so the store upserts. Rewriting EXISTING ciphertext with
		// DIFFERENT bytes is privileged: require write access to a ref that
		// reaches the secret, so a member cannot silently re-encrypt a secret
		// governed by a ref they don't control and lock its readers out.
		// Identical bytes are an idempotent no-op (the common re-push case) and
		// are always allowed.
		if p != nil {
			if _, existing, err := s.store.Get(hash); err == nil {
				if !bytes.Equal(existing, content) && !s.writableObject(p, actor, hash) {
					http.Error(w, "forbidden: cannot rewrite a secret without write access to a ref containing it", http.StatusForbidden)
					return
				}
			}
		}
		if err := s.store.PutSecret(hash, content); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	got, err := s.store.Put(object.Type(typ), content)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if got != hash {
		http.Error(w, "hash mismatch: content does not match "+hash, 400)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// readableObject reports whether actor may fetch an object: it must be
// reachable from a ref the actor can read, or be part of the policy chain. The
// reachable set is computed once per actor and reused until a ref change bumps
// the generation, so a clone fetching N objects walks the graph once, not N
// times.
func (s *Server) readableObject(p *policy.Policy, actor, hash string) bool {
	return s.reachableSet(p, actor, policy.Read)[hash]
}

// writableObject reports whether actor may overwrite an object: it must be
// reachable from a ref the actor has WRITE access to. Used to gate rewriting a
// secret's ciphertext, so a member cannot re-encrypt a secret governed by a ref
// they don't control.
func (s *Server) writableObject(p *policy.Policy, actor, hash string) bool {
	return s.reachableSet(p, actor, policy.Write)[hash]
}

// reachableSet returns the set of objects reachable from every ref on which
// actor holds `verb` (read or write). Results are cached per (actor, verb) and
// reused until a ref change bumps the generation.
func (s *Server) reachableSet(p *policy.Policy, actor, verb string) map[string]bool {
	key := actor + "\x00" + verb
	s.mu.Lock()
	if e, ok := s.cache[key]; ok && e.gen == s.gen {
		objs := e.objs
		s.mu.Unlock()
		return objs
	}
	gen := s.gen
	s.mu.Unlock()

	objs := map[string]bool{}
	// The policy chain is fetchable by anyone (clients must verify it); it is
	// never writable through the object endpoint, so include it for reads only.
	if verb == policy.Read {
		if chain, err := policy.ChainHashes(s.db, s.store); err == nil {
			for h := range chain {
				objs[h] = true
			}
		}
	}
	refs, _ := ref.List(s.db)
	for _, rf := range refs {
		if rf.Target == "" || rf.Visibility == ref.Policy {
			continue
		}
		allowed := p == nil
		if p != nil {
			if verb == policy.Read {
				allowed = canRead(p, actor, rf.Name)
			} else {
				allowed = p.Eval(actor, verb, rf.Name)
			}
		}
		if !allowed {
			continue
		}
		reach, err := s.store.Reachable(rf.Target)
		if err != nil {
			continue
		}
		for h := range reach {
			objs[h] = true
		}
	}

	s.mu.Lock()
	s.cache[key] = reachEntry{gen: gen, objs: objs}
	s.mu.Unlock()
	return objs
}

// canRead reports whether actor may read a ref. The policy ref is always
// readable (clients must fetch it to verify the chain).
func canRead(p *policy.Policy, actor, refName string) bool {
	if p == nil || refName == policy.Ref {
		return true
	}
	return p.Eval(actor, policy.Read, refName)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
