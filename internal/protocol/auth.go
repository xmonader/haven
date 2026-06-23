package protocol

import "fmt"

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

// canonicalRequest is the byte string a request signature covers.
func canonicalRequest(method, path, unixTime string) []byte {
	return []byte(fmt.Sprintf("%s\n%s\n%s", method, path, unixTime))
}
