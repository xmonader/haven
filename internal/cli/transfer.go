package cli

import (
	"database/sql"
	"fmt"
	"strings"

	"haven/internal/object"
	"haven/internal/protocol"
	"haven/internal/ref"
)

// remoteTrackingRef maps a remote ref name to its local tracking ref.
// refs/branches/main on remote "origin" -> refs/remotes/origin/branches/main.
func remoteTrackingRef(remoteName, refName string) string {
	return "refs/remotes/" + remoteName + "/" + strings.TrimPrefix(refName, "refs/")
}

// findRef resolves a user-supplied ref name (short or full) to a stored ref.
func findRef(db *sql.DB, name string) (ref.Ref, error) {
	candidates := []string{name}
	if !strings.HasPrefix(name, "refs/") {
		candidates = []string{ref.BranchPrefix + name, ref.HavenPrefix + name}
	}
	for _, c := range candidates {
		if rf, err := ref.Get(db, c); err == nil {
			return rf, nil
		}
	}
	return ref.Ref{}, fmt.Errorf("%s: no such ref", name)
}

// uploadReachable pushes every object reachable from target to the remote.
// Idempotent: re-uploading an existing object is a no-op server-side.
func uploadReachable(c *protocol.Client, store *object.Store, target string) error {
	objs, err := store.Reachable(target)
	if err != nil {
		return err
	}
	for h := range objs {
		typ, content, err := store.Get(h)
		if err != nil {
			return err
		}
		if err := c.PutObject(h, typ, content); err != nil {
			return err
		}
	}
	return nil
}

// downloadReachable fetches the transitive closure of target from the remote
// into the local store. Objects already present (with their closure) are
// skipped.
func downloadReachable(c *protocol.Client, store *object.Store, target string) error {
	if target == "" {
		return nil
	}
	seen := map[string]bool{}
	queue := []string{target}
	for len(queue) > 0 {
		h := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[h] {
			continue
		}
		seen[h] = true
		if has, err := store.Has(h); err != nil {
			return err
		} else if has {
			continue // assume closure already present from a prior fetch
		}
		typ, content, err := c.GetObject(h)
		if err != nil {
			return err
		}
		if _, err := store.Put(typ, content); err != nil {
			return err
		}
		switch typ {
		case object.Commit:
			com, err := object.ParseCommit(content)
			if err != nil {
				return err
			}
			queue = append(queue, com.Tree)
			queue = append(queue, com.Parents...)
		case object.Tree:
			entries, err := object.ParseTree(content)
			if err != nil {
				return err
			}
			for _, e := range entries {
				queue = append(queue, e.Hash)
			}
		}
	}
	return nil
}

// remoteRefMap fetches the remote ref listing as name -> target.
func remoteRefMap(c *protocol.Client) (map[string]string, []protocol.RefInfo, error) {
	refs, err := c.Refs()
	if err != nil {
		return nil, nil, err
	}
	m := map[string]string{}
	for _, rf := range refs {
		m[rf.Name] = rf.Target
	}
	return m, refs, nil
}
