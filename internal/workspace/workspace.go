// Package workspace scans the working tree and writes files back out on
// checkout.
package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"haven/internal/hash"
	"haven/internal/object"
	"haven/internal/repo"
	"haven/internal/secret"
)

// Scan walks the working tree rooted at root and returns a map of forward-slash
// relative paths to file entries (object hash, mode, type). Files matching a
// secret mark get type Secret and an identity hash over their plaintext (stable
// across re-encryption); others are plain blobs. The .haven directory is
// skipped. Hashes are computed in memory; nothing is stored or encrypted here.
func Scan(root string, marks []string) (map[string]object.FileEntry, error) {
	return ScanBaseline(root, marks, nil)
}

// ScanBaseline is Scan with a committed baseline tree: a file already present
// in the baseline takes its secret classification from the committed entry (so
// its identity hash matches what was committed regardless of the current ref's
// marks); new files are classified by marks. This keeps status/clean checks
// correct when a tree carries secret entries onto a ref whose marks differ.
func ScanBaseline(root string, marks []string, baseline map[string]object.FileEntry) (map[string]object.FileEntry, error) {
	out := map[string]object.FileEntry{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == repo.Dir {
				return filepath.SkipDir
			}
			return nil
		}
		isSymlink := d.Type()&fs.ModeSymlink != 0
		// Skip sockets, devices, pipes — only regular files and symlinks track.
		if !d.Type().IsRegular() && !isSymlink {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		var content []byte
		if isSymlink {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			content = []byte(target) // store the link target as the blob
		} else if content, err = os.ReadFile(path); err != nil {
			return err
		}

		typ := object.Blob
		if base, ok := baseline[rel]; ok {
			typ = base.Type
		} else if secret.Match(rel, marks) {
			typ = object.Secret
		}
		out[rel] = object.FileEntry{
			Hash: hash.Of(string(typ), content),
			Mode: modeFor(d, isSymlink),
			Type: typ,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func modeFor(d fs.DirEntry, isSymlink bool) string {
	if isSymlink {
		return object.ModeSymlink
	}
	info, err := d.Info()
	if err == nil && info.Mode()&0o111 != 0 {
		return object.ModeExec
	}
	return object.ModeFile
}

// IsIgnored reports whether a relative path should never be tracked.
func IsIgnored(rel string) bool {
	return rel == repo.Dir || strings.HasPrefix(rel, repo.Dir+"/")
}
