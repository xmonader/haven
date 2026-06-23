package cli

import (
	"fmt"
	"io"
	"strings"

	"haven/internal/lock"
	"haven/internal/ref"
	"haven/internal/workspace"
)

var cmdReset = Command{
	Name:    "reset",
	Summary: "move the current branch to a revision (--hard also resets the working tree)",
	Run:     runReset,
}

func runReset(args []string, out, errOut io.Writer) error {
	hard := hasFlag(args, "--hard")
	pos := positional(args)
	if len(pos) != 1 {
		return fmt.Errorf("usage: hv reset [--hard] <revision>")
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
	if !strings.HasPrefix(headRef, ref.BranchPrefix) && !strings.HasPrefix(headRef, ref.HavenPrefix) {
		return fmt.Errorf("HEAD is not on a branch or haven; cannot reset")
	}
	target, err := resolveCommit(r, pos[0])
	if err != nil {
		return err
	}
	newTree, err := store.TreeOfCommit(target)
	if err != nil {
		return err
	}

	if hard {
		oldCommit, err := ref.Resolve(r.DB, headRef)
		if err != nil {
			return err
		}
		oldTree, err := store.TreeOfCommit(oldCommit)
		if err != nil {
			return err
		}
		wc, err := lock.Acquire(r.Root)
		if err != nil {
			return err
		}
		defer wc.Release()
		if err := workspace.Checkout(r.Root, store, oldTree, newTree, currentIdentity()); err != nil {
			return err
		}
	}
	if err := ref.Set(r.DB, headRef, target); err != nil {
		return err
	}
	if err := resetStaging(r, store, newTree); err != nil {
		return err
	}

	mode := "(staging reset; working tree kept)"
	if hard {
		mode = "(working tree reset)"
	}
	fmt.Fprintf(out, "reset %s to %s %s\n", ref.ShortName(headRef), short(target), mode)
	return nil
}

func short(h string) string {
	if len(h) >= 10 {
		return h[:10]
	}
	return h
}
