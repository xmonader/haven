package cli

import (
	"fmt"
	"io"

	"haven/internal/diff"
	"haven/internal/object"
)

var cmdDiff = Command{
	Name:    "diff",
	Summary: "show changes (no args: working vs HEAD; or <ref> [<ref>])",
	Run:     runDiff,
}

func runDiff(args []string, out, errOut io.Writer) error {
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	var treeA, treeB string
	switch len(args) {
	case 0: // HEAD vs working tree
		if treeA, err = resolveTree(r, store, "HEAD"); err != nil {
			return err
		}
		if treeB, err = workingTree(r, store); err != nil {
			return err
		}
	case 1: // <ref> vs working tree
		if treeA, err = resolveTree(r, store, args[0]); err != nil {
			return err
		}
		if treeB, err = workingTree(r, store); err != nil {
			return err
		}
	default: // <refA> vs <refB>
		if treeA, err = resolveTree(r, store, args[0]); err != nil {
			return err
		}
		if treeB, err = resolveTree(r, store, args[1]); err != nil {
			return err
		}
	}

	changes, err := diff.Tree(store, treeA, treeB)
	if err != nil {
		return err
	}
	for _, c := range changes {
		if c.Kind == diff.Renamed {
			fmt.Fprintf(out, "diff renamed %s -> %s\n", c.From, c.Path)
			continue
		}
		fmt.Fprintf(out, "diff %s %s\n", c.Kind, c.Path)
		oldContent := contentOf(store, c.Old)
		newContent := contentOf(store, c.New)
		ud := diff.Unified("a/"+c.Path, "b/"+c.Path, oldContent, newContent)
		io.WriteString(out, ud)
	}
	if len(changes) == 0 {
		fmt.Fprintln(out, "no changes")
	}
	return nil
}

// contentOf returns a blob's content, or nil for the empty hash.
func contentOf(store *object.Store, h string) []byte {
	if h == "" {
		return nil
	}
	_, content, err := store.Get(h)
	if err != nil {
		return nil
	}
	return content
}
