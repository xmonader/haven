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

		oHere := oi < len(oh) && oh[oi].BaseStart == i
		tHere := ti < len(th) && th[ti].BaseStart == i

		switch {
		case oHere && tHere:
			o, t := oh[oi], th[ti]
			if o.BaseEnd == t.BaseEnd && equal(o.Repl, t.Repl) {
				out = append(out, o.Repl...) // both sides made the same edit
			} else {
				conflict = true
				out = append(out, markerOurs)
				out = append(out, o.Repl...)
				out = append(out, markerSplit)
				out = append(out, t.Repl...)
				out = append(out, markerTheirs)
			}
			i = max(o.BaseEnd, t.BaseEnd)
			oi++
			ti++
			for oi < len(oh) && oh[oi].BaseStart < i {
				oi++
			}
			for ti < len(th) && th[ti].BaseStart < i {
				ti++
			}
		case oHere:
			out = append(out, oh[oi].Repl...)
			i = oh[oi].BaseEnd
			oi++
		default: // tHere
			out = append(out, th[ti].Repl...)
			i = th[ti].BaseEnd
			ti++
		}
	}
	return out, conflict
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
