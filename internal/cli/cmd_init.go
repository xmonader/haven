package cli

import (
	"fmt"
	"io"
	"os"

	"haven/internal/repo"
	"haven/internal/secret"
)

var cmdInit = Command{
	Name:    "init",
	Summary: "create a new repository in the current directory",
	Run:     runInit,
}

func runInit(args []string, out, errOut io.Writer) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	r, err := repo.Init(dir)
	if err != nil {
		return err
	}
	defer r.Close()
	// Seed default secret marks so common credential files (.env, *.pem, ...)
	// are encrypted by default.
	if err := secret.SeedDefaultMarks(r.DB); err != nil {
		return err
	}
	fmt.Fprintf(out, "initialized empty haven repository in %s/%s\n", r.Root, repo.Dir)
	return nil
}
