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
	codecRaw  byte = 0 // payload stored verbatim
	codecZlib byte = 1 // payload zlib-compressed
)

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
