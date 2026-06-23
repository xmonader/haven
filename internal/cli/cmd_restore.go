package cli

import (
	"fmt"
	"io"

	"haven/internal/index"
	"haven/internal/lock"
	"haven/internal/object"
	"haven/internal/workspace"
)

var cmdRestore = Command{
	Name:    "restore",
	Summary: "restore working-tree file(s) from a revision (--source, default HEAD)",
	Run:     runRestore,
}

func runRestore(args []string, out, errOut io.Writer) error {
	source, err := flagValue(args, "--source")
	if err != nil {
		return err
	}
	if source == "" {
		source = "HEAD"
	}
	pos := positional(args, "--source")
	if len(pos) == 0 {
		return fmt.Errorf("usage: hv restore [--source <rev>] <path>...")
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	tree, err := resolveTree(r, store, source)
	if err != nil {
		return err
	}
	files, err := object.FlattenFull(store, tree)
	if err != nil {
		return err
	}

	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	id := currentIdentity()
	restored := 0
	for _, arg := range pos {
		rel, err := relPath(r.Root, arg)
		if err != nil {
			return err
		}
		matched := false
		for path, fe := range files {
			if path != rel && !hasDirPrefix(path, rel) {
				continue
			}
			matched = true
			if err := workspace.WriteEntry(r.Root, store, path, fe, id); err != nil {
				return err
			}
			if err := index.Add(r.DB, path, fe.Hash); err != nil {
				return err
			}
			restored++
		}
		if !matched {
			return fmt.Errorf("%s: not found in %s", arg, source)
		}
	}
	fmt.Fprintf(out, "restored %d file(s) from %s\n", restored, source)
	return nil
}

func hasDirPrefix(path, dir string) bool {
	return dir != "" && len(path) > len(dir) && path[:len(dir)] == dir && path[len(dir)] == '/'
}
