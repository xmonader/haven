package protocol

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"

	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/repo"
)

// Server serves a single repository over HTTP.
type Server struct {
	db    *sql.DB
	store *object.Store
	kind  string // KindTeam or KindPersonal
}

// NewServer builds a server over an open database. kind controls whether
// private refs are accepted (personal) or refused (team).
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

func (s *Server) getInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Info{DefaultBranch: ref.ShortName(repo.DefaultBranch), ServerKind: s.kind})
}

func (s *Server) getRefs(w http.ResponseWriter, r *http.Request) {
	refs, err := ref.List(s.db)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := make([]RefInfo, 0, len(refs))
	for _, rf := range refs {
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
	// A team server structurally refuses private refs (defense in depth).
	if s.kind == KindTeam && u.Visibility == ref.Private {
		http.Error(w, "team server refuses private refs", http.StatusForbidden)
		return
	}
	// Conditional update: OldTarget must match the current target.
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

func (s *Server) getObject(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
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
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Secret objects are addressed by their plaintext hash but carry ciphertext
	// the server cannot read, so their hash cannot be recomputed here — store as
	// given. All other objects are verified against their content.
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
