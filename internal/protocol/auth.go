package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Auth headers for signed requests. A client signs each request with its
// Ed25519 key; the server maps the public key to an actor via the policy
// keyring and authorizes with policy.Eval.
const (
	HdrPub  = "X-Haven-Pub"  // hex ed25519 public key
	HdrTime = "X-Haven-Time" // unix seconds
	HdrSig  = "X-Haven-Sig"  // hex signature over the canonical request
)

// MaxSkewSeconds bounds replay of a signed request.
const MaxSkewSeconds = 300

// canonicalRequest is the byte string a request signature covers. It binds the
// method, path, timestamp, AND a hash of the body, so a captured signature
// cannot be replayed against a different body (e.g. a tampered ref update).
func canonicalRequest(method, path, unixTime, bodyHashHex string) []byte {
	return []byte(fmt.Sprintf("%s\n%s\n%s\n%s", method, path, unixTime, bodyHashHex))
}

// bodyHash is the hex SHA-256 of a request body ("" bodies hash consistently on
// both ends).
func bodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
