package cli

import (
	"fmt"
	"io"
	"strings"

	"haven/internal/object"
	"haven/internal/protocol"
	"haven/internal/ref"
	"haven/internal/remote"
	"haven/internal/repo"
	"haven/internal/workspace"
)

var cmdClone = Command{
	Name:    "clone",
	Summary: "clone a remote repository (--kind team|personal)",
	Run:     runClone,
}

func runClone(args []string, out, errOut io.Writer) error {
	kind, err := flagValue(args, "--kind")
	if err != nil {
		return err
	}
	if kind == "" {
		kind = remote.Team
	}
	pos := positional(args, "--kind")
	if len(pos) < 1 {
		return fmt.Errorf("usage: hv clone <url> [dir] [--kind team|personal]")
	}
	url := pos[0]
	dir := defaultCloneDir(url)
	if len(pos) > 1 {
		dir = pos[1]
	}

	r, err := repo.Init(dir)
	if err != nil {
		return err
	}
	defer r.Close()
	store := object.NewStore(r.DB)

	if err := remote.Add(r.DB, "origin", url, kind); err != nil {
		return err
	}
	client := protocol.NewClient(url)
	info, err := client.Info()
	if err != nil {
		return err
	}
	if err := fetchInto(r, store, client, "origin", out); err != nil {
		return err
	}

	// Mirror remote branches/havens into local refs.
	_, refs, err := remoteRefMap(client)
	if err != nil {
		return err
	}
	for _, rr := range refs {
		if strings.HasPrefix(rr.Name, ref.BranchPrefix) || strings.HasPrefix(rr.Name, ref.HavenPrefix) {
			if err := ref.SetVisible(r.DB, rr.Name, rr.Target, rr.Visibility); err != nil {
				return err
			}
		}
	}

	// Check out the default branch.
	defaultRef := ref.BranchPrefix + info.DefaultBranch
	target, err := ref.Resolve(r.DB, defaultRef)
	if err != nil {
		return err
	}
	if target != "" {
		if err := r.SetHead(defaultRef); err != nil {
			return err
		}
		tree, err := store.TreeOfCommit(target)
		if err != nil {
			return err
		}
		if err := workspace.Checkout(r.Root, store, "", tree); err != nil {
			return err
		}
		if err := resetStaging(r, store, tree); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "cloned into %s\n", dir)
	return nil
}

func defaultCloneDir(url string) string {
	u := strings.TrimRight(url, "/")
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	u = strings.TrimSuffix(u, ".hv")
	if u == "" {
		return "haven-clone"
	}
	return u
}
