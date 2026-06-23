package cli

import (
	"fmt"
	"io"

	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
	"haven/internal/repo"
)

var cmdPolicy = Command{
	Name:    "policy",
	Summary: "show|log|verify|access <ref> — inspect the signed access policy",
	Run:     runPolicy,
}

func runPolicy(args []string, out, errOut io.Writer) error {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "show":
		p, err := policy.Load(r.DB, store)
		if err != nil || p == nil {
			fmt.Fprintln(out, "no policy yet (run 'hv key gen')")
			return err
		}
		fmt.Fprintf(out, "policy v%d (signed by %s)\n", p.Version, p.Signer)
		fmt.Fprintf(out, "members:\n")
		for name, m := range p.Keyring {
			fmt.Fprintf(out, "  %s [%s]\n", name, m.Status)
		}
		if len(p.Groups) > 0 {
			fmt.Fprintf(out, "groups:\n")
			for g, members := range p.Groups {
				fmt.Fprintf(out, "  %s: %v\n", g, members)
			}
		}
		fmt.Fprintf(out, "grants:\n")
		for _, g := range p.Grants {
			fmt.Fprintf(out, "  [%s] %s %s %s\n", g.ID, g.Subject, g.Verb, g.Resource)
		}
		if len(p.Restricted) > 0 {
			fmt.Fprintf(out, "restricted: %v\n", p.Restricted)
		}
		return nil
	case "log":
		return policyLog(r, store, out)
	case "verify":
		n, err := policy.VerifyChain(r.DB, store)
		if err != nil {
			return fmt.Errorf("policy chain INVALID: %w", err)
		}
		fmt.Fprintf(out, "policy chain valid: %d version(s)\n", n)
		return nil
	case "access":
		if len(args) != 2 {
			return fmt.Errorf("usage: hv policy access <ref>")
		}
		return policyAccess(r, store, args[1], out)
	default:
		return fmt.Errorf("unknown subcommand %q (want show|log|verify|access)", sub)
	}
}

func policyLog(r *repo.Repo, store *object.Store, out io.Writer) error {
	hashes, err := policy.ChainHashes(r.DB, store)
	if err != nil {
		return err
	}
	for h := range hashes {
		fmt.Fprintln(out, h)
	}
	return nil
}

func policyAccess(r *repo.Repo, store *object.Store, refSpec string, out io.Writer) error {
	p, err := policy.Load(r.DB, store)
	if err != nil || p == nil {
		return err
	}
	refName := refSpec
	if !hasRefPrefix(refSpec) {
		refName = ref.BranchPrefix + refSpec
	}
	readers, public := p.Readers(refName)
	fmt.Fprintf(out, "%s:\n", refName)
	if public {
		fmt.Fprintln(out, "  read: everyone (public)")
	}
	for name := range readers {
		fmt.Fprintf(out, "  read: %s\n", name)
	}
	return nil
}

func hasRefPrefix(s string) bool {
	return len(s) >= 5 && s[:5] == "refs/"
}
