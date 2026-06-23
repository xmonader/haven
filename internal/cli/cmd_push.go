package cli

import (
	"fmt"
	"io"

	"haven/internal/protocol"
	"haven/internal/ref"
	"haven/internal/remote"
)

var cmdPush = Command{
	Name:    "push",
	Summary: "push branches to a remote (refuses private havens)",
	Run:     runPush,
}

func runPush(args []string, out, errOut io.Writer) error {
	iKnow := hasFlag(args, "--i-know")
	pos := positional(args)

	remoteName := "origin"
	var refArgs []string
	if len(pos) > 0 {
		remoteName, refArgs = pos[0], pos[1:]
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
	remoteTargets, _, err := remoteRefMap(client)
	if err != nil {
		return err
	}

	// Determine which refs to push.
	var toPush []ref.Ref
	if len(refArgs) == 0 {
		headRef, err := r.Head()
		if err != nil {
			return err
		}
		rf, err := ref.Get(r.DB, headRef)
		if err != nil {
			return fmt.Errorf("current ref has no commits to push")
		}
		toPush = append(toPush, rf)
	} else {
		for _, name := range refArgs {
			rf, err := findRef(r.DB, name)
			if err != nil {
				return err
			}
			toPush = append(toPush, rf)
		}
	}

	for _, rf := range toPush {
		// Structural refusal: push never leaks a private haven.
		if rf.Visibility == ref.Private && !iKnow {
			return fmt.Errorf("refusing to push private haven %q; use 'hv sync' to a personal remote, or --i-know to override",
				ref.ShortName(rf.Name))
		}
		if rf.Target == "" {
			return fmt.Errorf("%s has no commits", ref.ShortName(rf.Name))
		}
		if err := uploadReachable(client, store, rf.Target); err != nil {
			return err
		}
		if err := client.UpdateRef(protocol.RefUpdate{
			Name:       rf.Name,
			Visibility: rf.Visibility,
			Target:     rf.Target,
			OldTarget:  remoteTargets[rf.Name],
		}); err != nil {
			return err
		}
		fmt.Fprintf(out, "pushed %s -> %s (%s)\n", ref.ShortName(rf.Name), remoteName, rf.Target[:10])
	}

	// Carry the signed access policy so the remote can enforce and verify it.
	if err := pushPolicy(client, r.DB, store, remoteTargets); err != nil {
		return fmt.Errorf("push policy: %w", err)
	}
	return nil
}
