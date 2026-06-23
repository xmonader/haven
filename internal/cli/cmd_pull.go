package cli

import (
	"fmt"
	"io"

	"haven/internal/protocol"
	"haven/internal/ref"
	"haven/internal/remote"
)

var cmdPull = Command{
	Name:    "pull",
	Summary: "fetch from a remote and merge into the current branch",
	Run:     runPull,
}

func runPull(args []string, out, errOut io.Writer) error {
	pos := positional(args)
	remoteName := "origin"
	if len(pos) > 0 {
		remoteName = pos[0]
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	rm, err := remote.Get(r.DB, remoteName)
	if err != nil {
		return err
	}
	client := protocol.NewClient(rm.URL)
	if err := fetchInto(r, store, client, remoteName, out); err != nil {
		return err
	}

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	track := remoteTrackingRef(remoteName, headRef)
	target, err := ref.Resolve(r.DB, track)
	if err != nil {
		return err
	}
	if target == "" {
		fmt.Fprintf(out, "nothing to pull for %s\n", ref.ShortName(headRef))
		return nil
	}
	return mergeInto(r, store, target, remoteName+"/"+ref.ShortName(headRef), out)
}
