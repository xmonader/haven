package cli

import (
	"fmt"
	"io"

	"haven/internal/ref"
)

var cmdHaven = Command{
	Name:    "haven",
	Summary: "create|switch|list|delete PRIVATE branches (never pushed)",
	Run:     runHaven,
}

func runHaven(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub, rest := args[0], args[1:]

	if sub == "create" && hasFlag(rest, "--secret") {
		return fmt.Errorf("--secret (encrypted-at-rest havens) lands in M8")
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "create":
		return refCreate(r, positional(rest), ref.HavenPrefix, ref.Private, out)
	case "switch":
		return refSwitch(r, store, positional(rest), ref.HavenPrefix, out)
	case "list":
		return refList(r, ref.HavenPrefix, out)
	case "delete":
		return refDelete(r, positional(rest), ref.HavenPrefix, out)
	default:
		return fmt.Errorf("unknown subcommand %q (want create|switch|list|delete)", sub)
	}
}
