package cli

import (
	"fmt"
	"io"

	"haven/internal/secret"
)

var cmdSecret = Command{
	Name:    "secret",
	Summary: "add|list secret path marks (matching files are encrypted)",
	Run:     runSecret,
}

func runSecret(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub, rest := args[0], args[1:]

	r, _, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "add":
		if len(rest) != 1 {
			return fmt.Errorf("usage: hv secret add <path-glob>")
		}
		if err := secret.AddMark(r.DB, rest[0]); err != nil {
			return err
		}
		fmt.Fprintf(out, "marked %q as secret (matching files will be encrypted on add)\n", rest[0])
		return nil
	case "list":
		marks, err := secret.Marks(r.DB)
		if err != nil {
			return err
		}
		for _, m := range marks {
			fmt.Fprintln(out, m)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (want add|list)", sub)
	}
}
