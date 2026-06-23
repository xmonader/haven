package protocol

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
	"haven/internal/repo"
)

// Server serves a single repository over HTTP.
type Server struct {
	db    *sql.DB
	store *object.Store
	kind  string
}

// NewServer builds a server over an open database.
func NewServer(db *sql.DB, kind string) *Server {
	return &Server{db: db, store: object.NewStore(db), kind: kind}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info", s.getInfo)
	mux.HandleFunc("GET /refs", s.getRefs)
	mux.HandleFunc("POST /refs", s.postRefs)
	mux.HandleFunc("GET /objects/{hash}", s.getObject)
	mux.HandleFunc("PUT /objects/{hash}", s.putObject)
	return mux
}

// authActor authenticates a request against the policy keyring. It returns the
// actor name ("" for anonymous), whether the request is allowed to proceed at
// all, and whether a present signature was valid. Missing auth headers are
// anonymous (limited to public access); a present-but-invalid signature is
// rejected.
func (s *Server) authActor(p *policy.Policy, r *http.Request) (actor string, ok bool) {
	if p == nil {
		return "", true // open repo: no policy, no enforcement
	}
	pub := r.Header.Get(HdrPub)
	ts := r.Header.Get(HdrTime)
	sigHex := r.Header.Get(HdrSig)
	if pub == "" && ts == "" && sigHex == "" {
		return "", true // anonymous: public access only
	}
	tsec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || abs(time.Now().Unix()-tsec) > MaxSkewSeconds {
		return "", false
	}
	pubBytes, err := hex.DecodeString(pub)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return "", false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil || !ed25519.Verify(pubBytes, canonicalRequest(r.Method, r.URL.Path, ts), sig) {
		return "", false
	}
	for name, m := range p.Keyring {
		if m.Sign == pub && m.Status != "revoked" {
			return name, true
		}
	}
	return "", false // signed by an unknown/revoked key
}

func (s *Server) getInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Info{DefaultBranch: ref.ShortName(repo.DefaultBranch), ServerKind: s.kind})
}

func (s *Server) getRefs(w http.ResponseWriter, r *http.Request) {
	p, _ := policy.Load(s.db, s.store)
	actor, ok := s.authActor(p, r)
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
	var u RefUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, "bad request: "+err.Error(), 400)
		return
	}
	if s.kind == KindTeam && u.Visibility == ref.Private {
		http.Error(w, "team server refuses private refs", http.StatusForbidden)
		return
	}
	p, _ := policy.Load(s.db, s.store)
	actor, ok := s.authActor(p, r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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

	cur, err := ref.Resolve(s.db, u.Name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if cur != u.OldTarget {
		http.Error(w, "ref update conflict: stale old_target", http.StatusConflict)
		return
	}
	if err := ref.SetVisible(s.db, u.Name, u.Target, u.Visibility); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// verifyIncomingPolicy checks that the just-uploaded policy object at head forms
// a valid extension of the server's current policy.
func (s *Server) verifyIncomingPolicy(head string, _ *policy.Policy) error {
	curHead, err := ref.Resolve(s.db, policy.Ref)
	if err != nil {
		return err
	}
	return policy.VerifyExtension(s.store, head, curHead)
}

func (s *Server) getObject(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	p, _ := policy.Load(s.db, s.store)
	actor, ok := s.authActor(p, r)
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
	if actor, ok := s.authActor(p, r); !ok || (p != nil && actor == "") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if object.Type(typ) == object.Secret {
		if err := s.store.PutRaw(hash, object.Secret, content); err != nil {
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
// reachable from a ref the actor can read, or be part of the policy chain.
func (s *Server) readableObject(p *policy.Policy, actor, hash string) bool {
	chain, _ := policy.ChainHashes(s.db, s.store)
	if chain[hash] {
		return true
	}
	refs, _ := ref.List(s.db)
	for _, rf := range refs {
		if rf.Target == "" || rf.Visibility == ref.Policy || !canRead(p, actor, rf.Name) {
			continue
		}
		objs, err := s.store.Reachable(rf.Target)
		if err != nil {
			continue
		}
		if objs[hash] {
			return true
		}
	}
	return false
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
