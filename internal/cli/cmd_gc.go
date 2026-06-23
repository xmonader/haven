package cli

import (
	"fmt"
	"io"

	"haven/internal/lock"
	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
)

var cmdGc = Command{
	Name:    "gc",
	Summary: "delete objects unreachable from any ref",
	Run:     runGc,
}

func runGc(args []string, out, errOut io.Writer) error {
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	// Hold the repo lock: sweeping objects must not run concurrently with repack
	// (which could create a delta against a base this gc is about to delete) or
	// another mutating op. Non-blocking — fails fast if the repo is busy.
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	// Union of objects reachable from every ref.
	reachable := map[string]bool{}
	refs, err := ref.List(r.DB)
	if err != nil {
		return err
	}
	for _, rf := range refs {
		if rf.Target == "" {
			continue
		}
		if rf.Visibility == ref.Policy {
			continue // policy chain handled below
		}
		objs, err := store.Reachable(rf.Target)
		if err != nil {
			return err
		}
		for h := range objs {
			reachable[h] = true
		}
	}
	// The signed policy chain is reachable by definition.
	chain, err := policy.ChainHashes(r.DB, store)
	if err != nil {
		return err
	}
	for h := range chain {
		reachable[h] = true
	}

	// Delta storage is invisible to the object graph: a reachable object may be
	// stored as a delta against a base that nothing else references. That base
	// must be kept, or deleting it would corrupt the reachable object.
	if err := addDeltaBases(store, reachable); err != nil {
		return err
	}

	all, err := store.AllHashes()
	if err != nil {
		return err
	}
	removed := 0
	for _, h := range all {
		if !reachable[h] {
			if err := store.Delete(h); err != nil {
				return err
			}
			removed++
		}
	}
	fmt.Fprintf(out, "removed %d unreachable object(s); %d kept\n", removed, len(reachable))
	return nil
}

// addDeltaBases extends a reachable set with the base of every reachable object
// that is stored as a delta. Bases are full objects (repack keeps chains at
// depth 1), so one pass suffices.
func addDeltaBases(store *object.Store, reachable map[string]bool) error {
	keys := make([]string, 0, len(reachable))
	for h := range reachable {
		keys = append(keys, h)
	}
	for _, h := range keys {
		base, ok, err := store.DeltaBase(h)
		if err != nil {
			return err
		}
		if ok {
			reachable[base] = true
		}
	}
	return nil
}
