package cli

import (
	"fmt"
	"io"
	"time"

	"haven/internal/config"
	"haven/internal/index"
	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/workspace"
)

var cmdCommit = Command{
	Name:    "commit",
	Summary: "record staged changes as a new commit (-m <message>)",
	Run:     runCommit,
}

func runCommit(args []string, out, errOut io.Writer) error {
	msg, err := flagValue(args, "-m")
	if err != nil {
		return err
	}
	if msg == "" {
		return fmt.Errorf("usage: hv commit -m <message>")
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	staged, err := index.All(r.DB)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		return fmt.Errorf("nothing staged (use 'hv add')")
	}

	// Modes and secret classification come from the current working tree.
	marks, err := marksOf(r)
	if err != nil {
		return err
	}
	scan, err := workspace.Scan(r.Root, marks)
	if err != nil {
		return err
	}
	files := make(map[string]object.FileEntry, len(staged))
	for path, h := range staged {
		mode, typ := object.ModeFile, object.Blob
		if fe, ok := scan[path]; ok {
			mode, typ = fe.Mode, fe.Type
		}
		files[path] = object.FileEntry{Hash: h, Mode: mode, Type: typ}
	}

	treeHash, err := object.BuildTree(store, files)
	if err != nil {
		return err
	}

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	parent, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	if parent != "" {
		if parentTree, err := store.TreeOfCommit(parent); err == nil && parentTree == treeHash {
			return fmt.Errorf("nothing to commit (working tree matches HEAD)")
		}
	}

	name, email := config.Author(r.DB)
	var parents []string
	if parent != "" {
		parents = []string{parent}
	}
	commitHash, err := store.PutCommit(object.CommitObj{
		Tree:    treeHash,
		Parents: parents,
		Author:  name,
		Email:   email,
		When:    time.Now().Unix(),
		Message: msg,
	})
	if err != nil {
		return err
	}
	if err := ref.Set(r.DB, headRef, commitHash); err != nil {
		return err
	}
	if err := armSecretBaseline(r, store, headRef, treeHash); err != nil {
		return err
	}

	fmt.Fprintf(out, "[%s %s] %s\n", ref.ShortName(headRef), commitHash[:10], firstLine(msg))
	return nil
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
