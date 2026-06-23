package workspace

import (
	"os"
	"path/filepath"

	"haven/internal/identity"
	"haven/internal/object"
	"haven/internal/secret"
)

// lockedNotice is written in place of a secret file the current user cannot
// decrypt (they are not a recipient).
const lockedNotice = "<haven: encrypted secret; you are not a recipient>\n"

// Checkout makes the working tree match newTree. Files present in oldTree but
// not in newTree are removed; files in newTree are written with their stored
// mode. Secret entries are decrypted with id (nil = not a recipient, written as
// a locked notice). Untracked files are left untouched.
func Checkout(root string, store *object.Store, oldTree, newTree string, id *identity.Identity) error {
	oldFiles, err := object.FlattenFull(store, oldTree)
	if err != nil {
		return err
	}
	newFiles, err := object.FlattenFull(store, newTree)
	if err != nil {
		return err
	}

	// Remove files that were tracked and are gone in the new tree.
	for path := range oldFiles {
		if _, ok := newFiles[path]; !ok {
			full := filepath.Join(root, filepath.FromSlash(path))
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return err
			}
			removeEmptyParents(root, full)
		}
	}

	// Write the new tree's files.
	for path, fe := range newFiles {
		if err := WriteEntry(root, store, path, fe, id); err != nil {
			return err
		}
	}
	return nil
}

// WriteEntry materializes one tree entry into the working tree: secrets are
// decrypted (or written as a locked notice for non-recipients) and symlinks are
// recreated as links rather than regular files.
func WriteEntry(root string, store *object.Store, path string, fe object.FileEntry, id *identity.Identity) error {
	_, stored, err := store.Get(fe.Hash)
	if err != nil {
		return err
	}
	content := stored
	if fe.Type == object.Secret {
		content = decryptOrLock(stored, id)
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if fe.Mode == object.ModeSymlink {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(string(content), full)
	}
	mode := os.FileMode(0o644)
	if fe.Mode == object.ModeExec {
		mode = 0o755
	}
	return os.WriteFile(full, content, mode)
}

// decryptOrLock returns the plaintext if id can decrypt the ciphertext,
// otherwise a locked notice.
func decryptOrLock(ciphertext []byte, id *identity.Identity) []byte {
	if id == nil {
		return []byte(lockedNotice)
	}
	plain, err := secret.Decrypt(ciphertext, id.X25519)
	if err != nil {
		return []byte(lockedNotice)
	}
	return plain
}

// removeEmptyParents deletes now-empty parent directories up to (not including)
// root.
func removeEmptyParents(root, full string) {
	dir := filepath.Dir(full)
	for dir != root && len(dir) > len(root) {
		if err := os.Remove(dir); err != nil {
			return // not empty or gone
		}
		dir = filepath.Dir(dir)
	}
}

// IsClean reports whether tracked files in the working tree match a reference
// tree (untracked files are ignored). marks classify secret files for hashing.
func IsClean(root string, store *object.Store, treeHash string, marks []string) (bool, error) {
	tracked, err := object.FlattenFull(store, treeHash)
	if err != nil {
		return false, err
	}
	work, err := ScanBaseline(root, marks, tracked)
	if err != nil {
		return false, err
	}
	for path, fe := range tracked {
		w, ok := work[path]
		if !ok || w.Hash != fe.Hash {
			return false, nil // modified or deleted
		}
	}
	return true, nil
}
