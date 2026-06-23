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

var cmdCherryPick = Command{
	Name:    "cherry-pick",
	Summary: "apply the change introduced by a commit onto the current branch",
	Run:     runCherryPick,
}

func runCherryPick(args []string, out, errOut io.Writer) error {
	pos := positional(args)
	if len(pos) != 1 {
		return fmt.Errorf("usage: hv cherry-pick <revision>")
	}
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	pick, err := resolveCommit(r, pos[0])
	if err != nil {
		return err
	}
	if pick == "" {
		return fmt.Errorf("%s has no commit", pos[0])
	}
	c, err := store.GetCommit(pick)
	if err != nil {
		return err
	}
	parentTree, err := parentTreeOf(store, c)
	if err != nil {
		return err
	}
	_, conflicted, err := threeWayApply(r, store, applySpec{
		baseTree:  parentTree,
		theirTree: c.Tree,
		message:   c.Message,
		author:    c.Author,
		email:     c.Email,
	}, out)
	if conflicted {
		fmt.Fprintln(out, "fix conflicts, then 'hv add' and 'hv commit'")
		return fmt.Errorf("cherry-pick has conflicts")
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "cherry-picked %s\n", short(pick))
	return nil
}

var cmdRevert = Command{
	Name:    "revert",
	Summary: "apply the inverse of a commit as a new commit",
	Run:     runRevert,
}

func runRevert(args []string, out, errOut io.Writer) error {
	pos := positional(args)
	if len(pos) != 1 {
		return fmt.Errorf("usage: hv revert <revision>")
	}
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	target, err := resolveCommit(r, pos[0])
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("%s has no commit", pos[0])
	}
	c, err := store.GetCommit(target)
	if err != nil {
		return err
	}
	parentTree, err := parentTreeOf(store, c)
	if err != nil {
		return err
	}
	name, email := config.Author(r.DB)
	// Reverse the change: treat the commit's own tree as the base and its parent
	// as "theirs", so applying it removes what the commit introduced.
	_, conflicted, err := threeWayApply(r, store, applySpec{
		baseTree:  c.Tree,
		theirTree: parentTree,
		message:   fmt.Sprintf("revert %s: %s", short(target), firstLine(c.Message)),
		author:    name,
		email:     email,
	}, out)
	if conflicted {
		fmt.Fprintln(out, "fix conflicts, then 'hv add' and 'hv commit'")
		return fmt.Errorf("revert has conflicts")
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "reverted %s\n", short(target))
	return nil
}

// applySpec describes a three-way application onto HEAD: merge baseTree..theirTree
// against the current HEAD tree, committing the result with the given metadata.
type applySpec struct {
	baseTree  string
	theirTree string
	message   string
	author    string
	email     string
}

// threeWayApply acquires the working-copy lock and applies spec onto HEAD. Use
// applyOnto directly when already holding the lock (e.g. inside rebase).
func threeWayApply(r *repo.Repo, store *object.Store, spec applySpec, out io.Writer) (string, bool, error) {
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return "", false, err
	}
	defer wc.Release()
	return applyOnto(r, store, spec, out)
}

// applyOnto merges spec onto the current HEAD, materializes the result, and
// (when conflict-free) commits it parented on HEAD. Returns the new commit hash
// and whether the result conflicted. Requires a clean working tree and that the
// caller already holds the working-copy lock.
func applyOnto(r *repo.Repo, store *object.Store, spec applySpec, out io.Writer) (string, bool, error) {
	headRef, err := r.Head()
	if err != nil {
		return "", false, err
	}
	head, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return "", false, err
	}
	if head == "" {
		return "", false, fmt.Errorf("no commits on %s", ref.ShortName(headRef))
	}
	ourTree, err := store.TreeOfCommit(head)
	if err != nil {
		return "", false, err
	}
	marks, err := marksOf(r)
	if err != nil {
		return "", false, err
	}
	if clean, err := workspace.IsClean(r.Root, store, ourTree, marks); err != nil {
		return "", false, err
	} else if !clean {
		return "", false, fmt.Errorf("working tree has uncommitted changes; commit or discard first")
	}

	res, err := merge.Trees(store, spec.baseTree, ourTree, spec.theirTree)
	if err != nil {
		return "", false, err
	}
	mergedTree, err := object.BuildTree(store, res.Files)
	if err != nil {
		return "", false, err
	}
	if err := workspace.Checkout(r.Root, store, ourTree, mergedTree, currentIdentity()); err != nil {
		return "", false, err
	}
	if err := resetStaging(r, store, mergedTree); err != nil {
		return "", false, err
	}
	if len(res.Conflicts) > 0 {
		fmt.Fprintf(out, "conflicts in %d file(s):\n", len(res.Conflicts))
		for _, p := range res.Conflicts {
			fmt.Fprintf(out, "  %s\n", p)
		}
		return "", true, nil
	}
	if mergedTree == ourTree {
		return head, false, nil // nothing to apply (empty change)
	}

	commitHash, err := store.PutCommit(object.CommitObj{
		Tree:    mergedTree,
		Parents: []string{head},
		Author:  spec.author,
		Email:   spec.email,
		When:    time.Now().Unix(),
		Message: spec.message,
	})
	if err != nil {
		return "", false, err
	}
	if err := ref.Set(r.DB, headRef, commitHash); err != nil {
		return "", false, err
	}
	return commitHash, false, nil
}

// parentTreeOf returns the tree of a commit's first parent, or the empty tree
// for a root commit.
func parentTreeOf(store *object.Store, c object.CommitObj) (string, error) {
	if len(c.Parents) == 0 {
		return "", nil
	}
	return store.TreeOfCommit(c.Parents[0])
}
