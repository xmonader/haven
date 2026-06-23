package cli

import (
	"fmt"
	"io"

	"haven/internal/identity"
	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/repo"
)

var cmdGroup = Command{
	Name:    "group",
	Summary: "create|add|list groups of actors",
	Run:     runGroup,
}

func runGroup(args []string, out, errOut io.Writer) error {
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
		if len(rest) != 1 {
			return fmt.Errorf("usage: hv group create <name>")
		}
		return mutateGroup(r, store, func(p *policy.Policy) error {
			if _, ok := p.Groups[rest[0]]; ok {
				return fmt.Errorf("group %q exists", rest[0])
			}
			p.Groups[rest[0]] = []string{}
			return nil
		}, out, "created group "+rest[0])
	case "add":
		if len(rest) < 2 {
			return fmt.Errorf("usage: hv group add <group> <actor>...")
		}
		return mutateGroup(r, store, func(p *policy.Policy) error {
			g, ok := p.Groups[rest[0]]
			if !ok {
				return fmt.Errorf("no such group %q", rest[0])
			}
			p.Groups[rest[0]] = appendUnique(g, rest[1:]...)
			return nil
		}, out, fmt.Sprintf("added %v to %s", rest[1:], rest[0]))
	case "list":
		p, err := policy.Load(r.DB, store)
		if err != nil || p == nil {
			return err
		}
		for name, members := range p.Groups {
			fmt.Fprintf(out, "%s: %v\n", name, members)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (want create|add|list)", sub)
	}
}

func mutateGroup(r *repo.Repo, store *object.Store, fn func(*policy.Policy) error, out io.Writer, msg string) error {
	id, err := identity.Load()
	if err != nil {
		return err
	}
	if err := policy.Mutate(r.DB, store, memberName(r), id.Sign, fn); err != nil {
		return err
	}
	fmt.Fprintln(out, msg)
	return nil
}

func appendUnique(s []string, items ...string) []string {
	seen := map[string]bool{}
	for _, x := range s {
		seen[x] = true
	}
	for _, x := range items {
		if !seen[x] {
			s = append(s, x)
			seen[x] = true
		}
	}
	return s
}
