package cli

import (
	"fmt"
	"io"

	"haven/internal/secret"
)

var cmdMember = Command{
	Name:    "member",
	Summary: "add|list members (recipients of encrypted secrets)",
	Run:     runMember,
}

func runMember(args []string, out, errOut io.Writer) error {
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
		if len(rest) != 2 {
			return fmt.Errorf("usage: hv member add <name> <recipient>")
		}
		if err := secret.AddMember(r.DB, rest[0], rest[1]); err != nil {
			return err
		}
		fmt.Fprintf(out, "added member %s\n", rest[0])
		return nil
	case "list":
		members, err := secret.Members(r.DB)
		if err != nil {
			return err
		}
		for _, m := range members {
			fmt.Fprintf(out, "%s\t%s\n", m.Name, m.Recipient)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (want add|list)", sub)
	}
}
