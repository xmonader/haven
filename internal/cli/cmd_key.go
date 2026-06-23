package cli

import (
	"fmt"
	"io"

	"haven/internal/config"
	"haven/internal/identity"
	"haven/internal/repo"
	"haven/internal/secret"
)

var cmdKey = Command{
	Name:    "key",
	Summary: "gen|show your encryption identity (private key stays off the repo)",
	Run:     runKey,
}

func runKey(args []string, out, errOut io.Writer) error {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "gen":
		id, err := identity.Generate()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "generated identity at %s\n", identity.Path())
		fmt.Fprintf(out, "recipient: %s\n", id.Recipient())
		// Register yourself as a member of the current repo, if in one.
		if r, openErr := repo.Open("."); openErr == nil {
			defer r.Close()
			name := memberName(r)
			if err := secret.AddMember(r.DB, name, id.Recipient()); err != nil {
				return err
			}
			fmt.Fprintf(out, "added you (%q) as a member of this repository\n", name)
		}
		return nil
	case "show":
		id, err := identity.Load()
		if err != nil {
			return err
		}
		fmt.Fprintln(out, id.Recipient())
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (want gen|show)", sub)
	}
}

// memberName picks an actor id for the current user.
func memberName(r *repo.Repo) string {
	if v, ok, _ := config.Get(r.DB, "user.name"); ok && v != "" {
		return v
	}
	name, _ := config.Author(r.DB)
	return name
}
