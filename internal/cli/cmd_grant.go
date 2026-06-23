package cli

import (
	"fmt"
	"io"
	"strconv"

	"haven/internal/identity"
	"haven/internal/policy"
	"haven/internal/ref"
)

var cmdGrant = Command{
	Name:    "grant",
	Summary: "grant <subject> <read|write|force|grant|admin> <ref-pattern>",
	Run:     runGrant,
}

func runGrant(args []string, out, errOut io.Writer) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: hv grant <actor|group> <verb> <ref-pattern>")
	}
	subject, verb, resource := args[0], args[1], args[2]
	switch verb {
	case policy.Read, policy.Write, policy.Force, policy.GrantV, policy.Admin:
	default:
		return fmt.Errorf("verb must be read|write|force|grant|admin")
	}

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()
	id, err := identity.Load()
	if err != nil {
		return err
	}

	err = policy.Mutate(r.DB, store, memberName(r), id.Sign, func(p *policy.Policy) error {
		gid := subject + ":" + verb + ":" + strconv.Itoa(len(p.Grants))
		p.Grants = append(p.Grants, policy.Grant{ID: gid, Subject: subject, Verb: verb, Resource: resource})
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "granted %s %s on %s\n", subject, verb, resource)
	return nil
}

var cmdRevoke = Command{
	Name:    "revoke",
	Summary: "revoke a grant by id",
	Run:     runRevoke,
}

func runRevoke(args []string, out, errOut io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: hv revoke <grant-id>")
	}
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()
	id, err := identity.Load()
	if err != nil {
		return err
	}
	found := false
	err = policy.Mutate(r.DB, store, memberName(r), id.Sign, func(p *policy.Policy) error {
		out := p.Grants[:0]
		for _, g := range p.Grants {
			if g.ID == args[0] {
				found = true
				continue
			}
			out = append(out, g)
		}
		p.Grants = out
		return nil
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no grant with id %q", args[0])
	}
	fmt.Fprintf(out, "revoked grant %s\n", args[0])
	return nil
}

var cmdRestrict = Command{
	Name:    "restrict",
	Summary: "restrict <ref> --read <group> (need-to-know sharing)",
	Run:     runRestrict,
}

func runRestrict(args []string, out, errOut io.Writer) error {
	group, err := flagValue(args, "--read")
	if err != nil {
		return err
	}
	pos := positional(args, "--read")
	if len(pos) != 1 || group == "" {
		return fmt.Errorf("usage: hv restrict <ref> --read <group>")
	}
	refName := ref.BranchPrefix + pos[0]

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()
	id, err := identity.Load()
	if err != nil {
		return err
	}

	// Mark the ref restricted (wildcard grants stop applying) and grant the
	// group read+write on it. Now only the group can reach it.
	err = policy.Mutate(r.DB, store, memberName(r), id.Sign, func(p *policy.Policy) error {
		p.Restricted = append(p.Restricted, refName)
		p.Grants = append(p.Grants,
			policy.Grant{ID: "restrict-read:" + pos[0], Subject: group, Verb: policy.Read, Resource: refName},
			policy.Grant{ID: "restrict-write:" + pos[0], Subject: group, Verb: policy.Write, Resource: refName},
		)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "restricted %s to group %s (public access removed)\n", pos[0], group)
	return nil
}
