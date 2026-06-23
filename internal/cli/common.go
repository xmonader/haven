package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"haven/internal/hash"
	"haven/internal/identity"
	"haven/internal/index"
	"haven/internal/lock"
	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/protocol"
	"haven/internal/ref"
	"haven/internal/repo"
	"haven/internal/secret"
	"haven/internal/workspace"
)

// authedClient builds a protocol client for url, attaching signed-request
// credentials when the user has an identity (anonymous otherwise).
func authedClient(url string) *protocol.Client {
	c := protocol.NewClient(url)
	if id := currentIdentity(); id != nil {
		c.WithAuth(id.SignPub(), id.Sign)
	}
	return c
}

// marksOf loads the repository's secret path globs. When the current HEAD ref
// is a secret ref, every file is encrypted at rest, so "**" is appended.
func marksOf(r *repo.Repo) ([]string, error) {
	marks, err := secret.Marks(r.DB)
	if err != nil {
		return nil, err
	}
	headRef, err := r.Head()
	if err != nil {
		return nil, err
	}
	secretRef, err := policy.IsSecretRef(r.DB, object.NewStore(r.DB), headRef)
	if err != nil {
		return nil, err
	}
	if secretRef {
		marks = append(marks, "**")
	}
	return marks, nil
}

// currentIdentity loads the user's identity, or nil if none exists.
func currentIdentity() *identity.Identity {
	return identity.LoadOrNil()
}

// openRepo opens the repository containing the current directory and returns
// it along with an object store.
func openRepo() (*repo.Repo, *object.Store, error) {
	r, err := repo.Open(".")
	if err != nil {
		return nil, nil, err
	}
	return r, object.NewStore(r.DB), nil
}

// switchTo moves HEAD to targetRef: it refuses if the working tree has
// uncommitted changes, materializes the target tree, repoints HEAD, and resets
// the staging area to the new tree.
func switchTo(r *repo.Repo, store *object.Store, targetRef string) error {
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	headCommit, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	headTree, err := store.TreeOfCommit(headCommit)
	if err != nil {
		return err
	}
	marks, err := marksOf(r)
	if err != nil {
		return err
	}
	clean, err := workspace.IsClean(r.Root, store, headTree, marks)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or discard first")
	}

	targetCommit, err := ref.Resolve(r.DB, targetRef)
	if err != nil {
		return err
	}
	targetTree, err := store.TreeOfCommit(targetCommit)
	if err != nil {
		return err
	}
	if err := workspace.Checkout(r.Root, store, headTree, targetTree, currentIdentity()); err != nil {
		return err
	}
	if err := r.SetHead(targetRef); err != nil {
		return err
	}
	return resetStaging(r, store, targetTree)
}

// resolveCommit turns a revision spec into a commit hash. Supported specs:
// "HEAD"/"@", a short branch/haven name, a full ref name, or a commit hash.
func resolveCommit(r *repo.Repo, spec string) (string, error) {
	switch spec {
	case "HEAD", "@":
		headRef, err := r.Head()
		if err != nil {
			return "", err
		}
		return ref.Resolve(r.DB, headRef)
	}
	if hash.IsHash(spec) {
		return spec, nil
	}
	for _, name := range []string{spec, ref.BranchPrefix + spec, ref.HavenPrefix + spec} {
		if rf, err := ref.Get(r.DB, name); err == nil {
			return rf.Target, nil
		}
	}
	return "", fmt.Errorf("%s: unknown revision", spec)
}

// resolveTree turns a revision spec into a tree hash.
func resolveTree(r *repo.Repo, store *object.Store, spec string) (string, error) {
	commit, err := resolveCommit(r, spec)
	if err != nil {
		return "", err
	}
	return store.TreeOfCommit(commit)
}

// workingTree snapshots the current working tree into objects and returns its
// root tree hash, without creating a commit. Plain files are stored as blobs;
// secret files keep the identity hash from the scan (their ciphertext object is
// expected to already exist from a prior add/commit).
func workingTree(r *repo.Repo, store *object.Store) (string, error) {
	marks, err := marksOf(r)
	if err != nil {
		return "", err
	}
	headRef, err := r.Head()
	if err != nil {
		return "", err
	}
	headCommit, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return "", err
	}
	headTree, err := store.TreeOfCommit(headCommit)
	if err != nil {
		return "", err
	}
	baseline, err := object.FlattenFull(store, headTree)
	if err != nil {
		return "", err
	}
	scan, err := workspace.ScanBaseline(r.Root, marks, baseline)
	if err != nil {
		return "", err
	}
	files := make(map[string]object.FileEntry, len(scan))
	for path, fe := range scan {
		if fe.Type == object.Secret {
			files[path] = fe // identity hash; ciphertext object already stored
			continue
		}
		full := filepath.Join(r.Root, filepath.FromSlash(path))
		var content []byte
		if fe.Mode == object.ModeSymlink {
			target, err := os.Readlink(full)
			if err != nil {
				return "", err
			}
			content = []byte(target)
		} else if content, err = os.ReadFile(full); err != nil {
			return "", err
		}
		h, err := store.Put(object.Blob, content)
		if err != nil {
			return "", err
		}
		files[path] = object.FileEntry{Hash: h, Mode: fe.Mode, Type: object.Blob}
	}
	return object.BuildTree(store, files)
}

// resetStaging makes the staging area mirror a tree exactly.
func resetStaging(r *repo.Repo, store *object.Store, treeHash string) error {
	if err := index.Clear(r.DB); err != nil {
		return err
	}
	files, err := object.Flatten(store, treeHash)
	if err != nil {
		return err
	}
	for path, h := range files {
		if err := index.Add(r.DB, path, h); err != nil {
			return err
		}
	}
	return nil
}
