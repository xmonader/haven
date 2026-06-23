package cli

import (
	"fmt"
	"io"

	"haven/internal/identity"
	"haven/internal/policy"
)

var cmdMember = Command{
	Name:    "member",
	Summary: "add|list keyring members (signed into policy)",
	Run:     runMember,
}

func runMember(args []string, out, errOut io.Writer) error {
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
	case "add":
		if len(rest) != 3 {
			return fmt.Errorf("usage: hv member add <name> <sign-pub> <age-recipient>")
		}
		name, signPub, encPub := rest[0], rest[1], rest[2]
		id, err := identity.Load()
		if err != nil {
			return err
		}
		signer := memberName(r)
		err = policy.Mutate(r.DB, store, signer, id.Sign, func(p *policy.Policy) error {
			p.Keyring[name] = policy.Member{Sign: signPub, Enc: encPub, Status: "active"}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "added member %s\n", name)
		return nil
	case "revoke":
		if len(rest) != 1 {
			return fmt.Errorf("usage: hv member revoke <name>")
		}
		id, err := identity.Load()
		if err != nil {
			return err
		}
		err = policy.Mutate(r.DB, store, memberName(r), id.Sign, func(p *policy.Policy) error {
			m, ok := p.Keyring[rest[0]]
			if !ok {
				return fmt.Errorf("no such member %q", rest[0])
			}
			m.Status = "revoked"
			p.Keyring[rest[0]] = m
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "revoked %s (rotate secrets they could read)\n", rest[0])
		return nil
	case "list":
		p, err := policy.Load(r.DB, store)
		if err != nil {
			return err
		}
		if p == nil {
			fmt.Fprintln(out, "no policy yet (run 'hv key gen')")
			return nil
		}
		for name, m := range p.Keyring {
			fmt.Fprintf(out, "%s\t%s\t%s\n", name, m.Status, m.Enc)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (want add|revoke|list)", sub)
	}
}
