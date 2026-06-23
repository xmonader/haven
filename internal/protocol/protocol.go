// Package protocol defines Haven's HTTPS wire format and the client/server
// that speak it. Plain JSON + raw object bytes, debuggable with curl.
package protocol

// HeaderType carries an object's type on object transfers.
const HeaderType = "X-Haven-Type"

// Server kinds. A team server refuses private refs; a personal server accepts
// them (so havens can sync between your own machines).
const (
	KindTeam     = "team"
	KindPersonal = "personal"
)

// Info is the response of GET /info.
type Info struct {
	DefaultBranch string `json:"default_branch"`
	ServerKind    string `json:"server_kind"`
}

// RefInfo describes one ref in GET /refs.
type RefInfo struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	Target     string `json:"target"`
}

// RefUpdate is the body of POST /refs: a conditional ref update. OldTarget must
// match the server's current target ("" means the ref must not yet exist).
type RefUpdate struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	Target     string `json:"target"`
	OldTarget  string `json:"old_target"`
}
