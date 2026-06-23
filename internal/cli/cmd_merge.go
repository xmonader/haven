package cli

import (
	"fmt"
	"io"
	"time"

	"haven/internal/config"
	"haven/internal/lock"
	"haven/internal/merge"
	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/repo"
	"haven/internal/workspace"
)

var cmdMerge = Command{
	Name:    "merge",
	Summary: "merge another ref into the current one (three-way)",
	Run:     runMerge,
}

func runMerge(args []string, out, errOut io.Writer) error {
	pos := positional(args)
	if len(pos) != 1 {
		return fmt.Errorf("usage: hv merge <ref>")
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	ours, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if ours == "" {
		return fmt.Errorf("no commits on %s to merge into", ref.ShortName(headRef))
	}
	theirs, err := resolveCommit(r, pos[0])
	if err != nil {
		return err
	}
	if theirs == "" {
		return fmt.Errorf("%s has no commits", pos[0])
	}
	return mergeInto(r, store, theirs, pos[0], out)
}

// mergeInto merges the commit theirs into the current HEAD ref. label names the
// source for messages. Shared by merge and pull.
func mergeInto(r *repo.Repo, store *object.Store, theirs, label string, out io.Writer) error {
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	ours, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if ours == "" {
		// Nothing here yet: adopt theirs wholesale.
		theirTree, err := store.TreeOfCommit(theirs)
		if err != nil {
			return err
		}
		if err := workspace.Checkout(r.Root, store, "", theirTree, currentIdentity()); err != nil {
			return err
		}
		if err := ref.Set(r.DB, headRef, theirs); err != nil {
			return err
		}
		if err := resetStaging(r, store, theirTree); err != nil {
			return err
		}
		fmt.Fprintf(out, "set %s to %s (%s)\n", ref.ShortName(headRef), label, theirs[:10])
		return nil
	}

	ourTree, err := store.TreeOfCommit(ours)
	if err != nil {
		return err
	}
	marks, err := marksOf(r)
	if err != nil {
		return err
	}
	if clean, err := workspace.IsClean(r.Root, store, ourTree, marks); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or discard first")
	}

	base, err := store.MergeBase(ours, theirs)
	if err != nil {
		return err
	}
	if theirs == ours || base == theirs {
		fmt.Fprintln(out, "already up to date")
		return nil
	}

	theirTree, err := store.TreeOfCommit(theirs)
	if err != nil {
		return err
	}

	// Fast-forward: our tip is the merge base.
	if base == ours {
		if err := workspace.Checkout(r.Root, store, ourTree, theirTree, currentIdentity()); err != nil {
			return err
		}
		if err := ref.Set(r.DB, headRef, theirs); err != nil {
			return err
		}
		if err := resetStaging(r, store, theirTree); err != nil {
			return err
		}
		fmt.Fprintf(out, "fast-forward to %s\n", theirs[:10])
		return nil
	}

	// True three-way merge.
	baseTree, err := store.TreeOfCommit(base)
	if err != nil {
		return err
	}
	res, err := merge.Trees(store, baseTree, ourTree, theirTree)
	if err != nil {
		return err
	}
	mergedTree, err := object.BuildTree(store, res.Files)
	if err != nil {
		return err
	}
	if err := workspace.Checkout(r.Root, store, ourTree, mergedTree, currentIdentity()); err != nil {
		return err
	}
	if err := resetStaging(r, store, mergedTree); err != nil {
		return err
	}

	if len(res.Conflicts) > 0 {
		fmt.Fprintf(out, "merge conflicts in %d file(s):\n", len(res.Conflicts))
		for _, p := range res.Conflicts {
			fmt.Fprintf(out, "  %s\n", p)
		}
		fmt.Fprintln(out, "fix conflicts, then 'hv add' and 'hv commit'")
		return fmt.Errorf("merge has conflicts")
	}

	name, email := config.Author(r.DB)
	commitHash, err := store.PutCommit(object.CommitObj{
		Tree:    mergedTree,
		Parents: []string{ours, theirs},
		Author:  name,
		Email:   email,
		When:    time.Now().Unix(),
		Message: fmt.Sprintf("merge %s", label),
	})
	if err != nil {
		return err
	}
	if err := ref.Set(r.DB, headRef, commitHash); err != nil {
		return err
	}
	fmt.Fprintf(out, "merged %s into %s (%s)\n", label, ref.ShortName(headRef), commitHash[:10])
	return nil
}
