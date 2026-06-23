package cli

import (
	"fmt"
	"io"

	"haven/internal/lock"
	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/repo"
	"haven/internal/workspace"
)

var cmdRebase = Command{
	Name:    "rebase",
	Summary: "replay the current branch's commits onto another base (linear)",
	Run:     runRebase,
}

func runRebase(args []string, out, errOut io.Writer) error {
	pos := positional(args)
	if len(pos) != 1 {
		return fmt.Errorf("usage: hv rebase <upstream>")
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
	orig, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if orig == "" {
		return fmt.Errorf("no commits on %s to rebase", ref.ShortName(headRef))
	}
	upstream, err := resolveCommit(r, pos[0])
	if err != nil {
		return err
	}
	if upstream == "" {
		return fmt.Errorf("%s has no commit", pos[0])
	}

	base, err := store.MergeBase(orig, upstream)
	if err != nil {
		return err
	}
	if base == upstream {
		fmt.Fprintln(out, "already up to date")
		return nil
	}

	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	origTree, err := store.TreeOfCommit(orig)
	if err != nil {
		return err
	}
	marks, err := marksOf(r)
	if err != nil {
		return err
	}
	if clean, err := workspace.IsClean(r.Root, store, origTree, marks); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or discard first")
	}

	// HEAD is an ancestor of upstream: nothing to replay, fast-forward.
	if base == orig {
		if err := moveHeadTo(r, store, headRef, origTree, upstream); err != nil {
			return err
		}
		fmt.Fprintf(out, "fast-forwarded %s to %s\n", ref.ShortName(headRef), short(upstream))
		return nil
	}

	// Commits to replay: base..orig, oldest first (first-parent linear).
	todo, err := commitsBetween(store, base, orig)
	if err != nil {
		return err
	}

	// Detach onto upstream, then replay each commit. Roll back fully on conflict
	// (no interactive --continue in v1).
	if err := moveHeadTo(r, store, headRef, origTree, upstream); err != nil {
		return err
	}
	for _, h := range todo {
		c, err := store.GetCommit(h)
		if err != nil {
			return err
		}
		parentTree, err := parentTreeOf(store, c)
		if err != nil {
			return err
		}
		_, conflicted, err := applyOnto(r, store, applySpec{
			baseTree:  parentTree,
			theirTree: c.Tree,
			message:   c.Message,
			author:    c.Author,
			email:     c.Email,
		}, out)
		if err != nil {
			return err
		}
		if conflicted {
			// Restore the original branch and working tree.
			cur, _ := ref.Resolve(r.DB, headRef)
			curTree, _ := store.TreeOfCommit(cur)
			_ = workspace.Checkout(r.Root, store, curTree, origTree, currentIdentity())
			_ = ref.Set(r.DB, headRef, orig)
			_ = resetStaging(r, store, origTree)
			return fmt.Errorf("rebase stopped: %q conflicts with the new base; branch left unchanged", firstLine(c.Message))
		}
	}
	tip, _ := ref.Resolve(r.DB, headRef)
	fmt.Fprintf(out, "rebased %d commit(s) onto %s (%s)\n", len(todo), short(upstream), short(tip))
	return nil
}

// moveHeadTo repoints headRef at target and materializes its tree.
func moveHeadTo(r *repo.Repo, store *object.Store, headRef, fromTree, target string) error {
	toTree, err := store.TreeOfCommit(target)
	if err != nil {
		return err
	}
	if err := workspace.Checkout(r.Root, store, fromTree, toTree, currentIdentity()); err != nil {
		return err
	}
	if err := ref.Set(r.DB, headRef, target); err != nil {
		return err
	}
	return resetStaging(r, store, toTree)
}

// commitsBetween returns the first-parent commits reachable from tip but not
// from base, oldest first.
func commitsBetween(store *object.Store, base, tip string) ([]string, error) {
	var chain []string
	for h := tip; h != "" && h != base; {
		c, err := store.GetCommit(h)
		if err != nil {
			return nil, err
		}
		chain = append(chain, h)
		if len(c.Parents) == 0 {
			break
		}
		h = c.Parents[0]
	}
	// Reverse to oldest-first.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}
