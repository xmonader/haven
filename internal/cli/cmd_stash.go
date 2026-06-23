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

// stashRef holds the newest stash; older stashes hang off its second parent.
const stashRef = "refs/stash"

var cmdStash = Command{
	Name:    "stash",
	Summary: "save|list|pop uncommitted changes off to the side",
	Run:     runStash,
}

func runStash(args []string, out, errOut io.Writer) error {
	sub := "save"
	if len(args) > 0 {
		sub = args[0]
	}
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "save":
		return stashSave(r, store, out)
	case "list":
		return stashList(r, store, out)
	case "pop":
		return stashPop(r, store, out)
	default:
		return fmt.Errorf("unknown subcommand %q (want save|list|pop)", sub)
	}
}

func stashSave(r *repo.Repo, store *object.Store, out io.Writer) error {
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	head, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if head == "" {
		return fmt.Errorf("nothing to stash: no commits yet")
	}
	headTree, err := store.TreeOfCommit(head)
	if err != nil {
		return err
	}
	workTree, err := workingTree(r, store)
	if err != nil {
		return err
	}
	if workTree == headTree {
		fmt.Fprintln(out, "no local changes to save")
		return nil
	}

	parents := []string{head}
	if prev, _ := ref.Resolve(r.DB, stashRef); prev != "" {
		parents = append(parents, prev)
	}
	name, email := config.Author(r.DB)
	stash, err := store.PutCommit(object.CommitObj{
		Tree:    workTree,
		Parents: parents,
		Author:  name,
		Email:   email,
		When:    time.Now().Unix(),
		Message: fmt.Sprintf("stash on %s", ref.ShortName(headRef)),
	})
	if err != nil {
		return err
	}
	if err := ref.SetVisible(r.DB, stashRef, stash, ref.Public); err != nil {
		return err
	}

	// Reset the working tree back to HEAD.
	if err := workspace.Checkout(r.Root, store, workTree, headTree, currentIdentity()); err != nil {
		return err
	}
	if err := resetStaging(r, store, headTree); err != nil {
		return err
	}
	fmt.Fprintf(out, "saved working changes to stash (%s)\n", short(stash))
	return nil
}

func stashList(r *repo.Repo, store *object.Store, out io.Writer) error {
	h, err := ref.Resolve(r.DB, stashRef)
	if err != nil {
		return err
	}
	i := 0
	for h != "" {
		c, err := store.GetCommit(h)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "stash@{%d} %s: %s\n", i, short(h), c.Message)
		if len(c.Parents) < 2 {
			break
		}
		h = c.Parents[1]
		i++
	}
	if i == 0 && h == "" {
		fmt.Fprintln(out, "no stashes")
	}
	return nil
}

func stashPop(r *repo.Repo, store *object.Store, out io.Writer) error {
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	stash, err := ref.Resolve(r.DB, stashRef)
	if err != nil {
		return err
	}
	if stash == "" {
		return fmt.Errorf("no stash to pop")
	}
	sc, err := store.GetCommit(stash)
	if err != nil {
		return err
	}

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	head, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	headTree, err := store.TreeOfCommit(head)
	if err != nil {
		return err
	}
	marks, err := marksOf(r)
	if err != nil {
		return err
	}
	if clean, err := workspace.IsClean(r.Root, store, headTree, marks); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or discard before popping")
	}

	// Three-way: base = HEAD when stashed, ours = current HEAD, theirs = stash.
	baseTree, err := parentTreeOf(store, sc)
	if err != nil {
		return err
	}
	res, err := merge.Trees(store, baseTree, headTree, sc.Tree)
	if err != nil {
		return err
	}
	mergedTree, err := object.BuildTree(store, res.Files)
	if err != nil {
		return err
	}
	if err := workspace.Checkout(r.Root, store, headTree, mergedTree, currentIdentity()); err != nil {
		return err
	}
	// Leave changes unstaged (staging mirrors HEAD).
	if err := resetStaging(r, store, headTree); err != nil {
		return err
	}

	if len(res.Conflicts) > 0 {
		fmt.Fprintf(out, "stash applied with conflicts in %d file(s) (stash kept):\n", len(res.Conflicts))
		for _, p := range res.Conflicts {
			fmt.Fprintf(out, "  %s\n", p)
		}
		return fmt.Errorf("stash pop has conflicts")
	}

	// Success: drop the stash by advancing the ref to the next-older entry.
	if len(sc.Parents) >= 2 {
		if err := ref.Set(r.DB, stashRef, sc.Parents[1]); err != nil {
			return err
		}
	} else if err := ref.Delete(r.DB, stashRef); err != nil {
		return err
	}
	fmt.Fprintf(out, "popped stash %s\n", short(stash))
	return nil
}
