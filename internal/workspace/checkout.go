package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"haven/internal/hash"
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
//
// Secrets are written 0600 (exec 0700) — never world-readable — and their
// decrypted plaintext is verified against the object's content hash before it
// touches disk, so forged ciphertext cannot substitute attacker-chosen content.
// Regular-file writes are atomic (temp file + rename) so an interrupted
// checkout never leaves a truncated or partially-written file.
func WriteEntry(root string, store *object.Store, path string, fe object.FileEntry, id *identity.Identity) error {
	_, stored, err := store.Get(fe.Hash)
	if err != nil {
		return err
	}
	content := stored
	if fe.Type == object.Secret {
		content, err = secretContent(stored, fe.Hash, id)
		if err != nil {
			return err
		}
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), dirMode(fe)); err != nil {
		return err
	}
	if fe.Mode == object.ModeSymlink {
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(string(content), full)
	}
	return writeFileAtomic(full, content, fileMode(fe))
}

// secretContent decrypts a stored secret for id, or returns the locked notice
// when id is nil or simply not a recipient. A decryption error that is NOT
// "not a recipient" (corrupt/truncated ciphertext) is returned as an error
// rather than silently masked as a locked notice — masking it would overwrite
// the working file and could lose data. On success the plaintext is verified
// against the secret's content hash (the secret's identity is the hash of its
// plaintext); a mismatch means forged/substituted ciphertext and is refused.
func secretContent(stored []byte, wantHash string, id *identity.Identity) ([]byte, error) {
	if id == nil {
		return []byte(lockedNotice), nil
	}
	plain, err := secret.Decrypt(stored, id.X25519)
	if err != nil {
		if errors.Is(err, secret.ErrNotRecipient) {
			return []byte(lockedNotice), nil
		}
		return nil, fmt.Errorf("decrypt secret %s: %w", wantHash, err)
	}
	if got := hash.Of(string(object.Secret), plain); got != wantHash {
		return nil, fmt.Errorf("secret %s failed integrity check (decrypts to %s): refusing to write forged content", wantHash, got)
	}
	return plain, nil
}

// fileMode is the permission for a materialized entry. Secrets are private
// (0600, or 0700 if executable); everything else uses the conventional 0644/0755.
func fileMode(fe object.FileEntry) os.FileMode {
	switch {
	case fe.Type == object.Secret && fe.Mode == object.ModeExec:
		return 0o700
	case fe.Type == object.Secret:
		return 0o600
	case fe.Mode == object.ModeExec:
		return 0o755
	default:
		return 0o644
	}
}

// dirMode is the permission for directories created to hold an entry. Newly
// created directories that will hold a secret are private (0700).
func dirMode(fe object.FileEntry) os.FileMode {
	if fe.Type == object.Secret {
		return 0o700
	}
	return 0o755
}

// writeFileAtomic writes content to a temp file in the target's directory, syncs
// it, sets its mode, and renames it over the target. os.CreateTemp makes the
// temp file 0600, so secret plaintext is never briefly world-readable before the
// final mode is applied. The rename is atomic on POSIX, so a crash leaves either
// the old file or the new one — never a truncated mix.
func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".haven-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
