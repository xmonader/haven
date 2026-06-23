package policy

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"fmt"

	"haven/internal/object"
	"haven/internal/ref"
)

// Ref is the reserved ref pointing at the head policy version.
const Ref = "refs/policy"

// Load returns the current (head) policy, or nil if the repo has no policy yet.
func Load(db *sql.DB, store *object.Store) (*Policy, error) {
	head, err := ref.Resolve(db, Ref)
	if err != nil {
		return nil, err
	}
	if head == "" {
		return nil, nil
	}
	return loadObject(store, head)
}

func loadObject(store *object.Store, hash string) (*Policy, error) {
	t, payload, err := store.Get(hash)
	if err != nil {
		return nil, err
	}
	if t != object.Policy {
		return nil, fmt.Errorf("object %s is %s, want policy", hash, t)
	}
	var p Policy
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse policy %s: %w", hash, err)
	}
	return &p, nil
}

// save stores a policy object and repoints refs/policy at it.
func save(db *sql.DB, store *object.Store, p *Policy) (string, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	h, err := store.Put(object.Policy, payload)
	if err != nil {
		return "", err
	}
	if err := ref.SetVisible(db, Ref, h, ref.Policy); err != nil {
		return "", err
	}
	return h, nil
}

// Mutate creates the next signed policy version by applying fn to a copy of the
// current policy (or bootstraps v0 if none exists), signs it as signer, verifies
// it against its parent, and saves it.
func Mutate(db *sql.DB, store *object.Store, signer string, priv ed25519.PrivateKey, fn func(*Policy) error) error {
	cur, err := Load(db, store)
	if err != nil {
		return err
	}
	var next Policy
	headHash := ""
	if cur == nil {
		next = Policy{
			Keyring: map[string]Member{},
			Groups:  map[string][]string{},
		}
	} else {
		headHash, _ = ref.Resolve(db, Ref)
		next = clone(cur)
		next.Version = cur.Version + 1
		next.Parent = headHash
	}
	if err := fn(&next); err != nil {
		return err
	}
	next.Sign(signer, priv)
	if err := next.Verify(cur); err != nil {
		return err
	}
	_, err = save(db, store, &next)
	return err
}

// clone deep-copies a policy via JSON round-trip.
func clone(p *Policy) Policy {
	b, _ := json.Marshal(p)
	var out Policy
	json.Unmarshal(b, &out)
	out.Sig = ""
	out.Signer = ""
	return out
}

// VerifyChain walks the policy chain from head to root, verifying every
// version's signature and authority, and that parent hashes link correctly.
func VerifyChain(db *sql.DB, store *object.Store) (int, error) {
	head, err := ref.Resolve(db, Ref)
	if err != nil {
		return 0, err
	}
	if head == "" {
		return 0, nil
	}
	// Collect chain head..root.
	var chain []*Policy
	var hashes []string
	h := head
	for h != "" {
		p, err := loadObject(store, h)
		if err != nil {
			return 0, err
		}
		chain = append(chain, p)
		hashes = append(hashes, h)
		h = p.Parent
	}
	// Verify from root upward so each has its parent available.
	for i := len(chain) - 1; i >= 0; i-- {
		var parent *Policy
		if i+1 < len(chain) {
			parent = chain[i+1]
		}
		if err := chain[i].Verify(parent); err != nil {
			return 0, err
		}
	}
	return len(chain), nil
}

// VerifyExtension checks that the policy object at newHead is a fully valid
// signed chain and, when the server already has a policy (curHead != ""), that
// it extends that policy — i.e. the current head is an ancestor of newHead, so
// no signed history was rewritten. Objects must already be in the store.
func VerifyExtension(store *object.Store, newHead, curHead string) error {
	if newHead == "" {
		return fmt.Errorf("empty policy head")
	}
	var chain []*Policy
	hashes := map[string]bool{}
	h := newHead
	for h != "" {
		p, err := loadObject(store, h)
		if err != nil {
			return err
		}
		chain = append(chain, p)
		hashes[h] = true
		h = p.Parent
	}
	for i := len(chain) - 1; i >= 0; i-- {
		var parent *Policy
		if i+1 < len(chain) {
			parent = chain[i+1]
		}
		if err := chain[i].Verify(parent); err != nil {
			return err
		}
	}
	if curHead != "" && !hashes[curHead] {
		return fmt.Errorf("policy does not extend current head %s (history rewrite refused)", curHead)
	}
	return nil
}

// RootSignKey returns the hex ed25519 signing key of the root (v0) policy's
// signer, walking the chain from head to root. This is the key a clone pins as
// its trust anchor, and the value a server can require an incoming first policy
// to match (so an open server can't be claimed by an arbitrary key).
func RootSignKey(store *object.Store, head string) (string, error) {
	if head == "" {
		return "", fmt.Errorf("empty policy head")
	}
	var root *Policy
	h := head
	for h != "" {
		p, err := loadObject(store, h)
		if err != nil {
			return "", err
		}
		root = p
		h = p.Parent
	}
	m, ok := root.Keyring[root.Signer]
	if !ok {
		return "", fmt.Errorf("root policy signer %q not in its own keyring", root.Signer)
	}
	return m.Sign, nil
}

// ChainHashes returns every policy object hash in the chain (for gc/fsck).
func ChainHashes(db *sql.DB, store *object.Store) (map[string]bool, error) {
	out := map[string]bool{}
	h, err := ref.Resolve(db, Ref)
	if err != nil {
		return nil, err
	}
	for h != "" {
		out[h] = true
		p, err := loadObject(store, h)
		if err != nil {
			return nil, err
		}
		h = p.Parent
	}
	return out, nil
}
