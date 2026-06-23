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
)

// Scan walks the working tree rooted at root and returns a map of
// forward-slash relative paths to file entries (blob hash + mode). The .haven
// directory is skipped. Hashes are computed in memory; nothing is stored.
func Scan(root string) (map[string]object.FileEntry, error) {
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
		// Skip symlinks and other non-regular files for v1.
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = object.FileEntry{
			Hash: hash.Of(string(object.Blob), content),
			Mode: modeFor(d),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func modeFor(d fs.DirEntry) string {
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
