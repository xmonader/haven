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

	// Rename-aware step: a file renamed on one side and edited in place on the
	// other would otherwise surface as a spurious modify/delete conflict that
	// drops the edit. Carry the edit onto the new path and consume both paths.
	consumed := map[string]bool{}
	carryRename(store, bf, of, tf, consumed, &res) // ours renamed, theirs edited
	carryRename(store, bf, tf, of, consumed, &res) // theirs renamed, ours edited

	for _, path := range unionKeys(bf, of, tf) {
		if consumed[path] {
			continue
		}
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

// carryRename finds files that "renamed" renamed (old->new, exact content from
// base) while "edited" modified the same file in place at the old path. It
// merges base/new/edited-old content onto the new path, marks both paths
// consumed, and records any conflict. Only plain blobs are considered.
func carryRename(store *object.Store, bf, renamed, edited map[string]object.FileEntry, consumed map[string]bool, res *Result) {
	for old, b := range bf {
		if b.Type != object.Blob {
			continue
		}
		if _, still := renamed[old]; still { // not renamed away on this side
			continue
		}
		e, eok := edited[old]
		if !eok || e.Hash == b.Hash { // other side deleted or left it unchanged
			continue
		}
		// Find a new path on the renamed side holding base's exact content.
		newPath, nf, found := exactRenameTarget(bf, renamed, old, b.Hash)
		if !found || consumed[old] || consumed[newPath] {
			continue
		}
		merged, conflict := Content(content(store, b.Hash), content(store, nf.Hash), content(store, e.Hash))
		h, err := store.Put(object.Blob, merged)
		if err != nil {
			continue // fall back to normal handling
		}
		res.Files[newPath] = object.FileEntry{Hash: h, Mode: nf.Mode, Type: object.Blob}
		consumed[old] = true
		consumed[newPath] = true
		if conflict {
			res.Conflicts = append(res.Conflicts, newPath)
		}
	}
}

// exactRenameTarget returns the path the renamed side added that did not exist
// in base and carries base[old]'s exact content (a pure rename of old).
func exactRenameTarget(bf, renamed map[string]object.FileEntry, old, baseHash string) (string, object.FileEntry, bool) {
	for path, fe := range renamed {
		if path == old || fe.Type != object.Blob {
			continue
		}
		if _, inBase := bf[path]; inBase {
			continue
		}
		if fe.Hash == baseHash {
			return path, fe, true
		}
	}
	return "", object.FileEntry{}, false
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
