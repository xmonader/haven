package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"haven/internal/ref"
	"haven/internal/repo"
)

var cmdTag = Command{
	Name:    "tag",
	Summary: "create|list|delete lightweight tags (refs/tags/*)",
	Run:     runTag,
}

func runTag(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch args[0] {
	case "list":
		return tagList(r, out)
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: hv tag delete <name>")
		}
		if _, err := ref.Get(r.DB, ref.TagPrefix+args[1]); err != nil {
			return fmt.Errorf("no such tag %q", args[1])
		}
		if err := ref.Delete(r.DB, ref.TagPrefix+args[1]); err != nil {
			return err
		}
		fmt.Fprintf(out, "deleted tag %s\n", args[1])
		return nil
	}

	// `hv tag <name> [<revision>]`
	pos := positional(args)
	if len(pos) < 1 || len(pos) > 2 {
		return fmt.Errorf("usage: hv tag <name> [<revision>]  |  hv tag list|delete")
	}
	name, rev := pos[0], "HEAD"
	if len(pos) == 2 {
		rev = pos[1]
	}
	full := ref.TagPrefix + name
	if existing, _ := ref.Resolve(r.DB, full); existing != "" {
		return fmt.Errorf("tag %q already exists", name)
	}
	target, err := resolveCommit(r, rev)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("%s has no commit to tag", rev)
	}
	if _, err := store.GetCommit(target); err != nil {
		return fmt.Errorf("%s is not a commit", rev)
	}
	if err := ref.SetVisible(r.DB, full, target, ref.Public); err != nil {
		return err
	}
	fmt.Fprintf(out, "tagged %s -> %s\n", name, short(target))
	return nil
}

func tagList(r *repo.Repo, out io.Writer) error {
	refs, err := ref.List(r.DB)
	if err != nil {
		return err
	}
	var names []string
	for _, rf := range refs {
		if strings.HasPrefix(rf.Name, ref.TagPrefix) {
			names = append(names, strings.TrimPrefix(rf.Name, ref.TagPrefix))
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintln(out, n)
	}
	return nil
}
