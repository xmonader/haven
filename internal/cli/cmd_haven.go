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

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "create":
		pos := positional(rest)
		if err := refCreate(r, pos, ref.HavenPrefix, ref.Private, out); err != nil {
			return err
		}
		if hasFlag(rest, "--secret") {
			if len(pos) != 1 {
				return fmt.Errorf("usage: hv haven create <name> --secret")
			}
			return secretRef(r, store, []string{ref.HavenPrefix + pos[0]}, out)
		}
		return nil
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
