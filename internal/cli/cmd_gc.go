package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

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

	grace, err := parsePruneGrace(args)
	if err != nil {
		return err
	}
	cutoff := time.Now().Unix() - int64(grace.Seconds())

	metas, err := store.Metas()
	if err != nil {
		return err
	}
	removed, spared := 0, 0
	for _, m := range metas {
		if reachable[m.Hash] {
			continue
		}
		// Grace period: never prune a recently-written object. This closes the
		// race where gc sweeps the objects of an in-flight commit/push before its
		// ref update lands. Use --prune=now to override.
		if m.CreatedAt > cutoff {
			spared++
			continue
		}
		if err := store.Delete(m.Hash); err != nil {
			return err
		}
		removed++
	}
	if spared > 0 {
		fmt.Fprintf(out, "removed %d unreachable object(s); %d kept; %d recent object(s) spared by grace period (--prune=now to force)\n", removed, len(reachable), spared)
	} else {
		fmt.Fprintf(out, "removed %d unreachable object(s); %d kept\n", removed, len(reachable))
	}
	return nil
}

// gcDefaultGrace is how long an unreachable object is protected from pruning
// after it was written, so concurrent writers' not-yet-referenced objects are
// safe. Mirrors git's default prune-expiry stance (conservative, not instant).
const gcDefaultGrace = 10 * time.Minute

// parsePruneGrace reads an optional `--prune <when>` / `--prune=<when>` argument.
// <when> is "now" (no grace, prune everything unreachable) or a Go duration
// (e.g. "2h", "30m"). Absent → gcDefaultGrace.
func parsePruneGrace(args []string) (time.Duration, error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		var val string
		switch {
		case a == "--prune":
			if i+1 >= len(args) {
				return 0, fmt.Errorf("--prune requires a value (now or a duration like 2h)")
			}
			val = args[i+1]
			i++
		case strings.HasPrefix(a, "--prune="):
			val = strings.TrimPrefix(a, "--prune=")
		default:
			continue
		}
		if val == "now" {
			return 0, nil
		}
		d, err := time.ParseDuration(val)
		if err != nil {
			return 0, fmt.Errorf("--prune: invalid duration %q (use now or e.g. 2h)", val)
		}
		if d < 0 {
			return 0, fmt.Errorf("--prune: duration must not be negative")
		}
		return d, nil
	}
	return gcDefaultGrace, nil
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
