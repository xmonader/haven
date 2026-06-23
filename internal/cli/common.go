package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"haven/internal/hash"
	"haven/internal/index"
	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/repo"
	"haven/internal/workspace"
)

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
	clean, err := workspace.IsClean(r.Root, store, headTree)
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
	if err := workspace.Checkout(r.Root, store, headTree, targetTree); err != nil {
		return err
	}
	if err := r.SetHead(targetRef); err != nil {
		return err
	}
	return resetStaging(r, store, targetTree)
}

// resolveTree turns a revision spec into a tree hash. Supported specs:
// "HEAD"/"@", a short branch/haven name, a full ref name, or a commit hash.
func resolveTree(r *repo.Repo, store *object.Store, spec string) (string, error) {
	switch spec {
	case "HEAD", "@":
		headRef, err := r.Head()
		if err != nil {
			return "", err
		}
		commit, err := ref.Resolve(r.DB, headRef)
		if err != nil {
			return "", err
		}
		return store.TreeOfCommit(commit)
	}
	if hash.IsHash(spec) {
		return store.TreeOfCommit(spec)
	}
	for _, name := range []string{spec, ref.BranchPrefix + spec, ref.HavenPrefix + spec} {
		if rf, err := ref.Get(r.DB, name); err == nil {
			return store.TreeOfCommit(rf.Target)
		}
	}
	return "", fmt.Errorf("%s: unknown revision", spec)
}

// workingTree snapshots the current working tree into objects (storing blobs)
// and returns its root tree hash, without creating a commit.
func workingTree(r *repo.Repo, store *object.Store) (string, error) {
	scan, err := workspace.Scan(r.Root)
	if err != nil {
		return "", err
	}
	files := make(map[string]object.FileEntry, len(scan))
	for path, fe := range scan {
		content, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		h, err := store.Put(object.Blob, content)
		if err != nil {
			return "", err
		}
		files[path] = object.FileEntry{Hash: h, Mode: fe.Mode}
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
