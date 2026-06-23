package cli

import (
	"fmt"
	"io"

	"haven/internal/object"
	"haven/internal/protocol"
	"haven/internal/ref"
	"haven/internal/remote"
	"haven/internal/repo"
)

var cmdFetch = Command{
	Name:    "fetch",
	Summary: "download refs and objects from a remote into tracking refs",
	Run:     runFetch,
}

func runFetch(args []string, out, errOut io.Writer) error {
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
	client := authedClient(rm.URL)
	return fetchInto(r, store, client, remoteName, out)
}

// fetchInto downloads every remote ref's objects and records local tracking
// refs. Shared by fetch, pull, clone, and sync.
func fetchInto(r *repo.Repo, store *object.Store, client *protocol.Client, remoteName string, out io.Writer) error {
	_, refs, err := remoteRefMap(client)
	if err != nil {
		return err
	}
	for _, rr := range refs {
		if err := downloadReachable(client, store, rr.Target); err != nil {
			return err
		}
		track := remoteTrackingRef(remoteName, rr.Name)
		if err := ref.SetVisible(r.DB, track, rr.Target, rr.Visibility); err != nil {
			return err
		}
		fmt.Fprintf(out, "fetched %s\n", rr.Name)
	}
	return nil
}
