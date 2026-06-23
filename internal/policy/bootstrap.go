package policy

import (
	"crypto/ed25519"
	"database/sql"
	"fmt"

	"haven/internal/object"
)

// Bootstrap creates policy v0 for a fresh repository: the founding member as
// admin over everything, with public read on branches. Errors if a policy
// already exists.
func Bootstrap(db *sql.DB, store *object.Store, me, signPub, encPub string, priv ed25519.PrivateKey) error {
	cur, err := Load(db, store)
	if err != nil {
		return err
	}
	if cur != nil {
		return fmt.Errorf("policy already exists")
	}
	return Mutate(db, store, me, priv, func(p *Policy) error {
		p.Keyring[me] = Member{Sign: signPub, Enc: encPub, Status: "active"}
		p.Grants = []Grant{
			{ID: "root-admin", Subject: me, Verb: Admin, Resource: "refs/**"},
			{ID: "public-read", Subject: "*", Verb: Read, Resource: "refs/branches/**"},
		}
		return nil
	})
}

// RecipientsFor returns the age recipients a secret on refName must be
// encrypted to (the active readers of that ref). Returns nil if there is no
// policy yet.
func RecipientsFor(db *sql.DB, store *object.Store, refName string) ([]string, error) {
	p, err := Load(db, store)
	if err != nil || p == nil {
		return nil, err
	}
	return p.Recipients(refName), nil
}
