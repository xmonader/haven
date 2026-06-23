package cli

import (
	"fmt"
	"io"

	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/repo"
)

var cmdBranch = Command{
	Name:    "branch",
	Summary: "create|switch|list|delete public branches",
	Run:     runBranch,
}

func runBranch(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub, rest := args[0], args[1:]
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "create":
		return refCreate(r, rest, ref.BranchPrefix, ref.Public, out)
	case "switch":
		return refSwitch(r, store, rest, ref.BranchPrefix, out)
	case "list":
		return refList(r, ref.BranchPrefix, out)
	case "delete":
		return refDelete(r, rest, ref.BranchPrefix, out)
	default:
		return fmt.Errorf("unknown subcommand %q (want create|switch|list|delete)", sub)
	}
}

// refCreate creates a ref at the current HEAD commit with the given
// visibility. Shared by branch (public) and haven (private).
func refCreate(r *repo.Repo, args []string, prefix, visibility string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: create <name>")
	}
	name := prefix + args[0]
	if _, err := ref.Get(r.DB, name); err == nil {
		return fmt.Errorf("%s already exists", args[0])
	}
	headRef, err := r.Head()
	if err != nil {
		return err
	}
	target, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if err := ref.SetVisible(r.DB, name, target, visibility); err != nil {
		return err
	}
	fmt.Fprintf(out, "created %s %s\n", visibility, args[0])
	return nil
}

func refSwitch(r *repo.Repo, store *object.Store, args []string, prefix string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: switch <name>")
	}
	name := prefix + args[0]
	if _, err := ref.Get(r.DB, name); err != nil {
		return fmt.Errorf("%s: no such ref", args[0])
	}
	if err := switchTo(r, store, name); err != nil {
		return err
	}
	fmt.Fprintf(out, "switched to %s\n", args[0])
	return nil
}

func refList(r *repo.Repo, prefix string, out io.Writer) error {
	head, _ := r.Head()
	refs, err := ref.List(r.DB)
	if err != nil {
		return err
	}
	for _, rf := range refs {
		if len(rf.Name) < len(prefix) || rf.Name[:len(prefix)] != prefix {
			continue
		}
		marker := "  "
		if rf.Name == head {
			marker = "* "
		}
		fmt.Fprintf(out, "%s%s\n", marker, ref.ShortName(rf.Name))
	}
	return nil
}

func refDelete(r *repo.Repo, args []string, prefix string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: delete <name>")
	}
	name := prefix + args[0]
	head, _ := r.Head()
	if name == head {
		return fmt.Errorf("cannot delete the current ref %q", args[0])
	}
	if _, err := ref.Get(r.DB, name); err != nil {
		return fmt.Errorf("%s: no such ref", args[0])
	}
	if err := ref.Delete(r.DB, name); err != nil {
		return err
	}
	fmt.Fprintf(out, "deleted %s\n", args[0])
	return nil
}
