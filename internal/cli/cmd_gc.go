package cli

import (
	"fmt"
	"io"

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
