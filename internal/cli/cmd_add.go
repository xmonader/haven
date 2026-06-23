package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"haven/internal/index"
	"haven/internal/object"
	"haven/internal/workspace"
)

var cmdAdd = Command{
	Name:    "add",
	Summary: "stage files for the next commit",
	Run:     runAdd,
}

func runAdd(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hv add <path>...")
	}
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	scan, err := workspace.Scan(r.Root)
	if err != nil {
		return err
	}

	staged := 0
	for _, arg := range args {
		matches, err := selectPaths(r.Root, arg, scan)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("%s: no matching files", arg)
		}
		for _, rel := range matches {
			content, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			h, err := store.Put(object.Blob, content)
			if err != nil {
				return err
			}
			if err := index.Add(r.DB, rel, h); err != nil {
				return err
			}
			staged++
		}
	}
	fmt.Fprintf(out, "staged %d file(s)\n", staged)
	return nil
}

// selectPaths resolves a CLI argument to a set of relative tracked paths,
// using the working-tree scan as the universe of files.
func selectPaths(root, arg string, scan map[string]object.FileEntry) ([]string, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	if rel == "." {
		return keys(scan), nil
	}
	if workspace.IsIgnored(rel) {
		return nil, nil
	}
	// Exact file match.
	if _, ok := scan[rel]; ok {
		return []string{rel}, nil
	}
	// Directory prefix match.
	var out []string
	prefix := rel + "/"
	for p := range scan {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out, nil
}

func keys(m map[string]object.FileEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
