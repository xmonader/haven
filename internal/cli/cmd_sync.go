package cli

import (
	"fmt"
	"io"
	"strings"

	"haven/internal/protocol"
	"haven/internal/ref"
	"haven/internal/remote"
)

var cmdSync = Command{
	Name:    "sync",
	Summary: "sync branches AND havens with a personal remote (carries privates)",
	Run:     runSync,
}

func runSync(args []string, out, errOut io.Writer) error {
	pos := positional(args)
	remoteName := "personal"
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
	if rm.Kind != remote.Personal {
		return fmt.Errorf("sync requires a personal remote; %q is %s (use 'hv push' for team)", remoteName, rm.Kind)
	}
	client := authedClient(rm.URL)

	// Push every local branch and haven (personal remotes carry privates).
	remoteTargets, _, err := remoteRefMap(client)
	if err != nil {
		return err
	}
	locals, err := ref.List(r.DB)
	if err != nil {
		return err
	}
	for _, rf := range locals {
		if rf.Target == "" {
			continue
		}
		if !strings.HasPrefix(rf.Name, ref.BranchPrefix) && !strings.HasPrefix(rf.Name, ref.HavenPrefix) {
			continue // skip tracking refs, policy, etc.
		}
		if err := uploadReachable(client, store, rf.Target); err != nil {
			return err
		}
		err := client.UpdateRef(protocol.RefUpdate{
			Name:       rf.Name,
			Visibility: rf.Visibility,
			Target:     rf.Target,
			OldTarget:  remoteTargets[rf.Name],
		})
		if err != nil {
			return fmt.Errorf("%s diverged on %s; pull/merge before syncing: %w", ref.ShortName(rf.Name), remoteName, err)
		}
		fmt.Fprintf(out, "synced %s -> %s\n", ref.ShortName(rf.Name), remoteName)
	}

	// Carry the signed policy too.
	if err := pushPolicy(client, r.DB, store, remoteTargets); err != nil {
		return fmt.Errorf("sync policy: %w", err)
	}

	// Bring down anything new from the personal remote as tracking refs.
	return fetchInto(r, store, client, remoteName, out)
}
