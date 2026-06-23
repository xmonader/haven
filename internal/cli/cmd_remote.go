package cli

import (
	"fmt"
	"io"

	"haven/internal/remote"
)

var cmdRemote = Command{
	Name:    "remote",
	Summary: "add|list|remove remotes (--kind team|personal)",
	Run:     runRemote,
}

func runRemote(args []string, out, errOut io.Writer) error {
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
		kind, err := flagValue(rest, "--kind")
		if err != nil {
			return err
		}
		if kind == "" {
			kind = remote.Team
		}
		pos := positional(rest, "--kind")
		if len(pos) != 2 {
			return fmt.Errorf("usage: hv remote add <name> <url> [--kind team|personal]")
		}
		if err := remote.Add(r.DB, pos[0], pos[1], kind); err != nil {
			return err
		}
		fmt.Fprintf(out, "added %s remote %s -> %s\n", kind, pos[0], pos[1])
		return nil
	case "list":
		remotes, err := remote.List(r.DB)
		if err != nil {
			return err
		}
		for _, rm := range remotes {
			fmt.Fprintf(out, "%s\t%s\t(%s)\n", rm.Name, rm.URL, rm.Kind)
		}
		return nil
	case "remove":
		if len(rest) != 1 {
			return fmt.Errorf("usage: hv remote remove <name>")
		}
		return remote.Remove(r.DB, rest[0])
	default:
		return fmt.Errorf("unknown subcommand %q (want add|list|remove)", sub)
	}
}
