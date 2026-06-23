package cli

import (
	"fmt"
	"io"

	"haven/internal/ref"
)

var cmdPublish = Command{
	Name:    "publish",
	Summary: "graduate a private haven into a public branch (--as <branch>)",
	Run:     runPublish,
}

func runPublish(args []string, out, errOut io.Writer) error {
	as, err := flagValue(args, "--as")
	if err != nil {
		return err
	}
	pos := positional(args, "--as")
	if len(pos) != 1 {
		return fmt.Errorf("usage: hv publish <haven> [--as <branch>]")
	}
	havenName := pos[0]
	branchName := havenName
	if as != "" {
		branchName = as
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	havenRef := ref.HavenPrefix + havenName
	hv, err := ref.Get(r.DB, havenRef)
	if err != nil {
		return fmt.Errorf("%s: no such haven", havenName)
	}
	if hv.Target == "" {
		return fmt.Errorf("%s has no commits to publish", havenName)
	}

	branchRef := ref.BranchPrefix + branchName
	if existing, err := ref.Get(r.DB, branchRef); err == nil {
		// Refuse unless this is a fast-forward: the public tip must be an
		// ancestor of the haven tip. Never silently overwrite public history.
		ff, err := store.IsAncestor(existing.Target, hv.Target)
		if err != nil {
			return err
		}
		if !ff {
			return fmt.Errorf("branch %q has diverged from haven %q; merge before publishing", branchName, havenName)
		}
	}

	if err := ref.SetVisible(r.DB, branchRef, hv.Target, ref.Public); err != nil {
		return err
	}
	fmt.Fprintf(out, "published %s -> branch %s at %s\n", havenName, branchName, hv.Target[:10])
	return nil
}
