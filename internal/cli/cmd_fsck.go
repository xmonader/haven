package cli

import (
	"fmt"
	"io"

	"haven/internal/hash"
	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
)

var cmdFsck = Command{
	Name:    "fsck",
	Summary: "verify object integrity and ref consistency",
	Run:     runFsck,
}

func runFsck(args []string, out, errOut io.Writer) error {
	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	var checked, secrets, problems int
	// Content addressing: every non-secret object's content must hash to its key.
	// Secret objects are addressed by plaintext hash, so their ciphertext cannot
	// be re-verified here — they are counted but not hashed.
	err = store.Each(func(h string, t object.Type, content []byte) error {
		checked++
		if t == object.Secret {
			secrets++
			return nil
		}
		if got := hash.Of(string(t), content); got != h {
			problems++
			fmt.Fprintf(out, "corrupt %s object: stored as %s but hashes to %s\n", t, h, got)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Every ref must point at an existing commit, and its history must resolve.
	refs, err := ref.List(r.DB)
	if err != nil {
		return err
	}
	for _, rf := range refs {
		if rf.Target == "" || rf.Visibility == ref.Policy {
			continue
		}
		if _, err := store.Reachable(rf.Target); err != nil {
			problems++
			fmt.Fprintf(out, "ref %s: unresolvable (%v)\n", rf.Name, err)
		}
	}

	// If a policy exists, its signature chain must verify.
	if n, err := policy.VerifyChain(r.DB, store); err != nil {
		problems++
		fmt.Fprintf(out, "policy chain invalid: %v\n", err)
	} else if n > 0 {
		fmt.Fprintf(out, "policy chain: %d version(s) verified\n", n)
	}

	fmt.Fprintf(out, "checked %d objects (%d encrypted secrets), %d refs\n", checked, secrets, len(refs))
	if problems > 0 {
		return fmt.Errorf("%d problem(s) found", problems)
	}
	fmt.Fprintln(out, "ok: no corruption detected")
	return nil
}
