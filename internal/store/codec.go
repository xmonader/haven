package store

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// Object content is stored with a one-byte codec tag prefix so the storage
// format can evolve and mixed-codec rows can coexist during/after a migration.
const (
	codecRaw   byte = 0 // payload stored verbatim
	codecZlib  byte = 1 // payload zlib-compressed
	codecDelta byte = 2 // payload is a delta program against another object
)

// deltaBaseHexLen is the fixed width of the base-object hash (SHA-256 hex)
// embedded in a delta envelope.
const deltaBaseHexLen = 64

// EncodeDelta wraps a delta program as stored content: the delta tag, the
// fixed-width base hash, then the (further-encoded) delta bytes. The reader
// fetches the base, applies the delta, and gets back the exact payload — so the
// object's identity hash is unchanged by being stored as a delta.
func EncodeDelta(baseHash string, delta []byte) []byte {
	out := make([]byte, 0, 1+deltaBaseHexLen+len(delta)+1)
	out = append(out, codecDelta)
	out = append(out, baseHash...)
	return append(out, Encode(delta)...)
}

// DecodeDelta reports whether stored content is a delta envelope and, if so,
// returns the base hash and the decoded delta program.
func DecodeDelta(stored []byte) (baseHash string, delta []byte, ok bool, err error) {
	if len(stored) == 0 || stored[0] != codecDelta {
		return "", nil, false, nil
	}
	if len(stored) < 1+deltaBaseHexLen {
		return "", nil, true, fmt.Errorf("decode delta: envelope too short")
	}
	baseHash = string(stored[1 : 1+deltaBaseHexLen])
	delta, err = Decode(stored[1+deltaBaseHexLen:])
	if err != nil {
		return "", nil, true, fmt.Errorf("decode delta: %w", err)
	}
	return baseHash, delta, true, nil
}

// IsDelta reports whether stored content is a delta envelope, without decoding.
func IsDelta(stored []byte) bool {
	return len(stored) > 0 && stored[0] == codecDelta
}

// Encode prepares object content for storage: a one-byte codec tag followed by
// the payload, zlib-compressed only when that actually shrinks it. High-entropy
// content (ciphertext, already-compressed blobs) and tiny payloads are stored
// raw, so compression never enlarges an object. The object's hash is computed
// over the original payload, never over this encoded form.
func Encode(payload []byte) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(payload)
	zw.Close()
	if buf.Len() < len(payload) {
		return append([]byte{codecZlib}, buf.Bytes()...)
	}
	out := make([]byte, 0, len(payload)+1)
	out = append(out, codecRaw)
	return append(out, payload...)
}

// Decode reverses Encode, returning the original payload.
func Decode(stored []byte) ([]byte, error) {
	if len(stored) == 0 {
		return nil, nil
	}
	codec, body := stored[0], stored[1:]
	switch codec {
	case codecRaw:
		return body, nil
	case codecZlib:
		zr, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("decode object: %w", err)
		}
		defer zr.Close()
		return io.ReadAll(zr)
	default:
		return nil, fmt.Errorf("decode object: unknown codec byte %d", codec)
	}
}
