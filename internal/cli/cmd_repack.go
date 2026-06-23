package cli

import (
	"fmt"
	"io"
	"sort"

	"haven/internal/lock"
	"haven/internal/object"
)

var cmdRepack = Command{
	Name:    "repack",
	Summary: "store similar objects as deltas to shrink the repository",
	Run:     runRepack,
}

const (
	// repackMinSize skips objects too small for a delta to beat the base-hash
	// envelope overhead.
	repackMinSize = 128
	// repackWindow bounds how many recent same-type bases each object is tried
	// against, keeping repack roughly linear.
	repackWindow = 8
)

// runRepack converts eligible whole objects into deltas against a similar base.
// It is safe to interrupt or re-run: each rewrite is independently self-verified
// (see object.StoreAsDelta), only ever shrinks an object, and only deltas
// against full objects so read chains stay shallow. Secrets and the policy chain
// are left whole — ciphertext is incompressible and the signed chain stays
// trivially verifiable.
func runRepack(args []string, out, errOut io.Writer) error {
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	// Hold the repo lock: rewriting objects to deltas must not run concurrently
	// with gc (which could delete a base mid-repack) or another repack (which
	// could build a deeper-than-1 chain). The lock is non-blocking, so concurrent
	// maintenance fails fast rather than racing.
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	metas, err := store.Metas()
	if err != nil {
		return err
	}

	// Candidates: whole (non-delta) blobs/trees/commits above the size floor.
	var cands []object.Meta
	for _, m := range metas {
		if m.IsDelta || m.Size < repackMinSize {
			continue
		}
		switch m.Type {
		case object.Blob, object.Tree, object.Commit:
			cands = append(cands, m)
		}
	}
	// Group similar objects: same type, then ascending size, so size-adjacent
	// (and likely similar) objects are tried against each other.
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Type != cands[j].Type {
			return cands[i].Type < cands[j].Type
		}
		return cands[i].Size < cands[j].Size
	})

	basesByType := map[object.Type][]string{}
	deltified, reclaimed := 0, 0
	for _, m := range cands {
		bases := basesByType[m.Type]
		applied := false
		// Try the most size-similar bases first (closest are last appended).
		for i := len(bases) - 1; i >= 0 && i >= len(bases)-repackWindow; i-- {
			before, err := store.StoredSize(m.Hash)
			if err != nil {
				return err
			}
			ok, err := store.StoreAsDelta(m.Hash, bases[i])
			if err != nil {
				// A self-check failure here means the delta path is unsound;
				// fail loudly rather than risk the object store.
				return fmt.Errorf("repack %s against %s: %w", m.Hash, bases[i], err)
			}
			if ok {
				after, err := store.StoredSize(m.Hash)
				if err != nil {
					return err
				}
				reclaimed += before - after
				deltified++
				applied = true
				break
			}
		}
		if !applied {
			basesByType[m.Type] = append(bases, m.Hash)
		}
	}

	// Rewrites shrink rows in place; VACUUM returns the freed pages to the OS so
	// the on-disk file actually gets smaller (like the compaction step of git gc).
	if deltified > 0 {
		if _, err := r.DB.Exec(`VACUUM`); err != nil {
			return fmt.Errorf("repack: vacuum: %w", err)
		}
	}

	fmt.Fprintf(out, "repacked %d object(s), reclaimed %d bytes; %d candidate(s) left whole\n",
		deltified, reclaimed, len(cands)-deltified)
	return nil
}
