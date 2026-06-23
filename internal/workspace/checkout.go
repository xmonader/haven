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
		mode := os.FileMode(0o644)
		if fe.Mode == object.ModeExec {
			mode = 0o755
		}
		if err := os.WriteFile(full, content, mode); err != nil {
			return err
		}
	}
	return nil
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
	tracked, err := object.Flatten(store, treeHash)
	if err != nil {
		return false, err
	}
	work, err := Scan(root, marks)
	if err != nil {
		return false, err
	}
	for path, h := range tracked {
		fe, ok := work[path]
		if !ok || fe.Hash != h {
			return false, nil // modified or deleted
		}
	}
	return true, nil
}
