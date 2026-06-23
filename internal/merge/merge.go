// Package merge implements three-way content and tree merges with git-style
// conflict markers.
package merge

import (
	"strings"

	"haven/internal/diff"
)

// Conflict markers.
const (
	markerOurs   = "<<<<<<< ours"
	markerSplit  = "======="
	markerTheirs = ">>>>>>> theirs"
)

// Content performs a three-way line merge of base/ours/theirs. It returns the
// merged bytes and whether any conflict markers were emitted.
func Content(base, ours, theirs []byte) ([]byte, bool) {
	bl := lines(base)
	ol := lines(ours)
	tl := lines(theirs)

	merged, conflict := mergeLines(bl, ol, tl)

	out := strings.Join(merged, "\n")
	if len(merged) > 0 {
		out += "\n"
	}
	return []byte(out), conflict
}

func mergeLines(base, ours, theirs []string) ([]string, bool) {
	oh := diff.Chunks(base, ours)
	th := diff.Chunks(base, theirs)

	var out []string
	conflict := false
	i, oi, ti := 0, 0, 0

	const inf = int(^uint(0) >> 1)
	for i < len(base) || oi < len(oh) || ti < len(th) {
		oStart, tStart := inf, inf
		if oi < len(oh) {
			oStart = oh[oi].BaseStart
		}
		if ti < len(th) {
			tStart = th[ti].BaseStart
		}
		next := min(oStart, tStart)

		// Emit common base lines up to the next change.
		if next == inf {
			out = append(out, base[i:]...)
			break
		}
		if i < next {
			out = append(out, base[i:next]...)
			i = next
		}

		// Group all chunks (from either side) that overlap the change region
		// starting at i into a single region [i, regionEnd). A chunk joins the
		// region if it starts at i (the seed) or strictly overlaps the region so
		// far (BaseStart < regionEnd); adjacent, non-overlapping edits stay
		// separate so they merge cleanly. This is what makes overlapping edits
		// that start at *different* base lines a conflict rather than a silent,
		// wrong auto-merge.
		regionEnd := i
		oFirst, tFirst := oi, ti
		for {
			advanced := false
			for oi < len(oh) && (oh[oi].BaseStart == i || oh[oi].BaseStart < regionEnd) {
				if oh[oi].BaseEnd > regionEnd {
					regionEnd = oh[oi].BaseEnd
				}
				oi++
				advanced = true
			}
			for ti < len(th) && (th[ti].BaseStart == i || th[ti].BaseStart < regionEnd) {
				if th[ti].BaseEnd > regionEnd {
					regionEnd = th[ti].BaseEnd
				}
				ti++
				advanced = true
			}
			if !advanced {
				break
			}
		}

		oChanged := oi > oFirst
		tChanged := ti > tFirst
		switch {
		case oChanged && tChanged:
			oSide := regionSide(base, oh, oFirst, oi, i, regionEnd)
			tSide := regionSide(base, th, tFirst, ti, i, regionEnd)
			if equal(oSide, tSide) {
				out = append(out, oSide...) // both sides resolve identically
			} else {
				conflict = true
				out = append(out, markerOurs)
				out = append(out, oSide...)
				out = append(out, markerSplit)
				out = append(out, tSide...)
				out = append(out, markerTheirs)
			}
		case oChanged:
			out = append(out, regionSide(base, oh, oFirst, oi, i, regionEnd)...)
		default: // tChanged
			out = append(out, regionSide(base, th, tFirst, ti, i, regionEnd)...)
		}
		i = regionEnd
	}
	return out, conflict
}

// regionSide reconstructs one side's text for the base region [rs, re), given
// that side's chunks[lo:hi] (which fall within the region). Unchanged base lines
// between/around those chunks are carried through, so the result is the side's
// full view of the region — exactly what belongs between conflict markers.
func regionSide(base []string, chunks []diff.Chunk, lo, hi, rs, re int) []string {
	var s []string
	pos := rs
	for k := lo; k < hi; k++ {
		c := chunks[k]
		if c.BaseStart > pos {
			s = append(s, base[pos:c.BaseStart]...)
		}
		s = append(s, c.Repl...)
		pos = c.BaseEnd
	}
	if pos < re {
		s = append(s, base[pos:re]...)
	}
	return s
}

// lines splits content into lines without trailing newlines. A trailing
// newline in the input does not produce a final empty line.
func lines(b []byte) []string {
	s := string(b)
	if s == "" {
		return nil
	}
	out := strings.Split(s, "\n")
	if n := len(out); n > 0 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
