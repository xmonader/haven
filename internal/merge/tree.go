package merge

import (
	"sort"

	"haven/internal/object"
)

// Result is the outcome of a three-way tree merge.
type Result struct {
	Files     map[string]object.FileEntry // merged file set (blobs stored)
	Conflicts []string                    // paths that contain conflict markers
}

// Trees performs a three-way merge of the ours and theirs trees against their
// common base. Cleanly-merged content is stored as new blobs; conflicting
// files are stored with markers and listed in Conflicts.
func Trees(store *object.Store, base, ours, theirs string) (Result, error) {
	bf, err := object.FlattenFull(store, base)
	if err != nil {
		return Result{}, err
	}
	of, err := object.FlattenFull(store, ours)
	if err != nil {
		return Result{}, err
	}
	tf, err := object.FlattenFull(store, theirs)
	if err != nil {
		return Result{}, err
	}

	res := Result{Files: map[string]object.FileEntry{}}
	for _, path := range unionKeys(bf, of, tf) {
		b, bok := bf[path]
		o, ook := of[path]
		t, tok := tf[path]
		bh, oh, th := hashOf(b, bok), hashOf(o, ook), hashOf(t, tok)

		switch {
		case oh == th: // identical on both sides (including both deleted)
			if ook {
				res.Files[path] = o
			}
		case oh == bh: // ours unchanged from base -> take theirs
			if tok {
				res.Files[path] = t
			}
		case th == bh: // theirs unchanged from base -> take ours
			if ook {
				res.Files[path] = o
			}
		default: // both sides changed -> content merge
			baseC := content(store, bh)
			ourC := content(store, oh)
			theirC := content(store, th)
			merged, conflict := Content(baseC, ourC, theirC)
			h, err := store.Put(object.Blob, merged)
			if err != nil {
				return Result{}, err
			}
			res.Files[path] = object.FileEntry{Hash: h, Mode: pickMode(o, ook, t, tok)}
			if conflict || !ook || !tok {
				res.Conflicts = append(res.Conflicts, path)
			}
		}
	}
	sort.Strings(res.Conflicts)
	return res, nil
}

func hashOf(fe object.FileEntry, ok bool) string {
	if !ok {
		return ""
	}
	return fe.Hash
}

func content(store *object.Store, hash string) []byte {
	if hash == "" {
		return nil
	}
	_, c, err := store.Get(hash)
	if err != nil {
		return nil
	}
	return c
}

func pickMode(o object.FileEntry, ook bool, t object.FileEntry, tok bool) string {
	if ook {
		return o.Mode
	}
	if tok {
		return t.Mode
	}
	return object.ModeFile
}

func unionKeys(maps ...map[string]object.FileEntry) []string {
	set := map[string]struct{}{}
	for _, m := range maps {
		for k := range m {
			set[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
