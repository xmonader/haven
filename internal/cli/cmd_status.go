package cli

import (
	"fmt"
	"io"
	"sort"

	"haven/internal/index"
	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/workspace"
)

var cmdStatus = Command{
	Name:    "status",
	Summary: "show staged, modified, and untracked files",
	Run:     runStatus,
}

func runStatus(args []string, out, errOut io.Writer) error {
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	parent, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	headTreeHash, err := store.TreeOfCommit(parent)
	if err != nil {
		return err
	}
	head, err := object.Flatten(store, headTreeHash)
	if err != nil {
		return err
	}
	staged, err := index.All(r.DB)
	if err != nil {
		return err
	}
	working, err := workspace.Scan(r.Root)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "On %s\n", ref.ShortName(headRef))
	if parent == "" {
		fmt.Fprintln(out, "No commits yet")
	}

	var toCommit, notStaged, untracked []string
	for path, h := range staged {
		if head[path] != h {
			toCommit = append(toCommit, path)
		}
	}
	for path, fe := range working {
		sh, isStaged := staged[path]
		if !isStaged {
			if head[path] == "" {
				untracked = append(untracked, path)
			} else {
				// tracked in HEAD but not staged; show as not-staged if changed
				if head[path] != fe.Hash {
					notStaged = append(notStaged, path)
				}
			}
			continue
		}
		if sh != fe.Hash {
			notStaged = append(notStaged, path)
		}
	}

	printGroup(out, "Changes to be committed:", toCommit)
	printGroup(out, "Changes not staged for commit:", notStaged)
	printGroup(out, "Untracked files:", untracked)

	if len(toCommit)+len(notStaged)+len(untracked) == 0 {
		fmt.Fprintln(out, "nothing to commit, working tree clean")
	}
	return nil
}

func printGroup(out io.Writer, title string, paths []string) {
	if len(paths) == 0 {
		return
	}
	sort.Strings(paths)
	fmt.Fprintf(out, "\n%s\n", title)
	for _, p := range paths {
		fmt.Fprintf(out, "  %s\n", p)
	}
}
