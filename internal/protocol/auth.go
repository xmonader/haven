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
	HdrPub   = "X-Haven-Pub"   // hex ed25519 public key
	HdrTime  = "X-Haven-Time"  // unix seconds
	HdrSig   = "X-Haven-Sig"   // hex signature over the canonical request
	HdrNonce = "X-Haven-Nonce" // unique per-request token (anti-replay)
)

// MaxSkewSeconds bounds replay of a signed request. The server also remembers
// every accepted nonce for this long, so a captured request cannot be replayed
// even within the window.
const MaxSkewSeconds = 300

// MaxRequestBytes caps the size of any request body the server will buffer. The
// whole body is read into memory (the signature covers it), so without a bound a
// single client could exhaust server memory. 256 MiB is far above any real
// object or ref update.
const MaxRequestBytes = 256 << 20

// canonicalRequest is the byte string a request signature covers. It binds the
// method, path, timestamp, body hash, AND a per-request nonce, so a captured
// signature cannot be replayed against a different body or reused verbatim.
func canonicalRequest(method, path, unixTime, bodyHashHex, nonce string) []byte {
	return []byte(fmt.Sprintf("%s\n%s\n%s\n%s\n%s", method, path, unixTime, bodyHashHex, nonce))
}

// bodyHash is the hex SHA-256 of a request body ("" bodies hash consistently on
// both ends).
func bodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
