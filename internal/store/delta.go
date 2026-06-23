package store

import (
	"encoding/binary"
	"fmt"
)

// Delta encoding: a target object can be stored as a copy/insert program against
// a base object, so similar objects (e.g. successive versions of a file) share
// bytes. The program is a sequence of ops:
//
//	insert: 0x00, uvarint n, n literal bytes
//	copy:   0x01, uvarint offset, uvarint length   (from the base)
//
// The stream is prefixed with the base length and target length (uvarints) so
// applyDelta can validate it. Reconstruction is exact — the object's hash is
// still computed over the reconstructed content, so identity never changes.

const deltaWindow = 16 // min match length considered for a copy

// makeDelta produces a program that reconstructs target from base.
func MakeDelta(base, target []byte) []byte {
	// Index the first offset of every deltaWindow-byte window of base.
	index := make(map[string]int, len(base))
	for i := 0; i+deltaWindow <= len(base); i++ {
		key := string(base[i : i+deltaWindow])
		if _, ok := index[key]; !ok {
			index[key] = i
		}
	}

	var out []byte
	var hdr [binary.MaxVarintLen64]byte
	out = append(out, hdr[:binary.PutUvarint(hdr[:], uint64(len(base)))]...)
	out = append(out, hdr[:binary.PutUvarint(hdr[:], uint64(len(target)))]...)

	var literal []byte
	flush := func() {
		if len(literal) == 0 {
			return
		}
		out = append(out, 0x00)
		out = append(out, hdr[:binary.PutUvarint(hdr[:], uint64(len(literal)))]...)
		out = append(out, literal...)
		literal = literal[:0]
	}

	j := 0
	for j < len(target) {
		matched := false
		if j+deltaWindow <= len(target) {
			if off, ok := index[string(target[j:j+deltaWindow])]; ok {
				// Verify (guard against hash-key collisions are impossible here
				// since the key IS the bytes) and extend the match greedily.
				mlen := deltaWindow
				for off+mlen < len(base) && j+mlen < len(target) && base[off+mlen] == target[j+mlen] {
					mlen++
				}
				flush()
				out = append(out, 0x01)
				out = append(out, hdr[:binary.PutUvarint(hdr[:], uint64(off))]...)
				out = append(out, hdr[:binary.PutUvarint(hdr[:], uint64(mlen))]...)
				j += mlen
				matched = true
			}
		}
		if !matched {
			literal = append(literal, target[j])
			j++
		}
	}
	flush()
	return out
}

// applyDelta reconstructs the target from base and a delta program. It validates
// every offset/length, so a corrupt or malicious delta yields an error rather
// than a panic or wrong output.
func ApplyDelta(base, delta []byte) ([]byte, error) {
	r := delta
	baseLen, n := binary.Uvarint(r)
	if n <= 0 {
		return nil, fmt.Errorf("delta: bad base length header")
	}
	r = r[n:]
	if int(baseLen) != len(base) {
		return nil, fmt.Errorf("delta: base length %d != actual %d", baseLen, len(base))
	}
	targetLen, n := binary.Uvarint(r)
	if n <= 0 {
		return nil, fmt.Errorf("delta: bad target length header")
	}
	r = r[n:]

	out := make([]byte, 0, targetLen)
	for len(r) > 0 {
		op := r[0]
		r = r[1:]
		switch op {
		case 0x00: // insert
			ln, n := binary.Uvarint(r)
			if n <= 0 {
				return nil, fmt.Errorf("delta: bad insert length")
			}
			r = r[n:]
			if uint64(len(r)) < ln {
				return nil, fmt.Errorf("delta: insert truncated")
			}
			out = append(out, r[:ln]...)
			r = r[ln:]
		case 0x01: // copy
			off, n := binary.Uvarint(r)
			if n <= 0 {
				return nil, fmt.Errorf("delta: bad copy offset")
			}
			r = r[n:]
			ln, n := binary.Uvarint(r)
			if n <= 0 {
				return nil, fmt.Errorf("delta: bad copy length")
			}
			r = r[n:]
			if off+ln > uint64(len(base)) || off+ln < off {
				return nil, fmt.Errorf("delta: copy out of range")
			}
			out = append(out, base[off:off+ln]...)
		default:
			return nil, fmt.Errorf("delta: unknown op %d", op)
		}
	}
	if uint64(len(out)) != targetLen {
		return nil, fmt.Errorf("delta: reconstructed %d bytes, want %d", len(out), targetLen)
	}
	return out, nil
}
