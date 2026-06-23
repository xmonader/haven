// Package hash provides SHA-256 content addressing for Haven objects.
//
// An object's hash covers its type and length as well as its payload, so the
// same bytes stored as different types (blob vs tree) hash differently.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Of returns the hex SHA-256 of a typed payload.
// The hashed preimage is "<type> <len>\x00<payload>".
func Of(typ string, payload []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s %d\x00", typ, len(payload))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// IsHash reports whether s looks like a full hex SHA-256 hash.
func IsHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
