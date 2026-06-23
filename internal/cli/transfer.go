package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/protocol"
	"haven/internal/ref"
)

// policyParent extracts the parent hash from a serialized policy object,
// without depending on the policy package's full type. A parse error is
// returned (not swallowed) so a corrupt policy object aborts the clone rather
// than silently truncating the policy chain — the chain is the root of trust.
func policyParent(content []byte) (string, error) {
	var p struct {
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(content, &p); err != nil {
		return "", fmt.Errorf("malformed policy object: %w", err)
	}
	return p.Parent, nil
}

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
		// Secret objects are addressed by plaintext hash; store under the given
		// hash without recomputing (we hold only ciphertext). Their integrity is
		// checked at decrypt time (plaintext must hash to this id).
		if typ == object.Secret {
			if err := store.PutRaw(h, typ, content); err != nil {
				return err
			}
		} else {
			// Verify the server returned the object we asked for: Put stores under
			// the recomputed hash, so a mismatch means the content doesn't match h.
			got, err := store.Put(typ, content)
			if err != nil {
				return err
			}
			if got != h {
				return fmt.Errorf("object %s: server returned content hashing to %s", h, got)
			}
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
		case object.Policy:
			parent, err := policyParent(content)
			if err != nil {
				return err
			}
			if parent != "" {
				queue = append(queue, parent)
			}
		}
	}
	return nil
}

// pushPolicy uploads the entire signed policy chain and updates the remote's
// policy ref, so access policy travels with the repository.
func pushPolicy(c *protocol.Client, db *sql.DB, store *object.Store, remoteTargets map[string]string) error {
	head, err := ref.Resolve(db, policy.Ref)
	if err != nil || head == "" {
		return err
	}
	chain, err := policy.ChainHashes(db, store)
	if err != nil {
		return err
	}
	for h := range chain {
		typ, content, err := store.Get(h)
		if err != nil {
			return err
		}
		if err := c.PutObject(h, typ, content); err != nil {
			return err
		}
	}
	return c.UpdateRef(protocol.RefUpdate{
		Name:       policy.Ref,
		Visibility: ref.Policy,
		Target:     head,
		OldTarget:  remoteTargets[policy.Ref],
	})
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
