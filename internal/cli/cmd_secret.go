package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"haven/internal/config"
	"haven/internal/identity"
	"haven/internal/object"
	"haven/internal/policy"
	"haven/internal/ref"
	"haven/internal/repo"
	"haven/internal/secret"
)

var cmdSecret = Command{
	Name:    "secret",
	Summary: "add|list|ref|rotate|status — manage secrets and secret refs",
	Run:     runSecret,
}

func runSecret(args []string, out, errOut io.Writer) error {
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
		if len(rest) != 1 {
			return fmt.Errorf("usage: hv secret add <path-glob>")
		}
		if err := secret.AddMark(r.DB, rest[0]); err != nil {
			return err
		}
		fmt.Fprintf(out, "marked %q as secret (matching files will be encrypted on add)\n", rest[0])
		return nil
	case "list":
		marks, err := secret.Marks(r.DB)
		if err != nil {
			return err
		}
		for _, m := range marks {
			fmt.Fprintln(out, m)
		}
		return nil
	case "ref":
		return secretRef(r, store, rest, out)
	case "rotate":
		return secretRotate(r, store, rest, out)
	case "status":
		return secretStatus(r, store, out)
	default:
		return fmt.Errorf("unknown subcommand %q (want add|list|ref|rotate|status)", sub)
	}
}

// secretRef marks a whole ref as secret: every file in its commits is encrypted
// at rest. Recorded in the signed policy.
func secretRef(r *repo.Repo, store *object.Store, rest []string, out io.Writer) error {
	if len(rest) != 1 {
		return fmt.Errorf("usage: hv secret ref <branch-or-haven>")
	}
	refName := resolveRefName(r, rest[0])
	id, err := identity.Load()
	if err != nil {
		return err
	}
	err = policy.Mutate(r.DB, store, memberName(r), id.Sign, func(p *policy.Policy) error {
		for _, ex := range p.SecretRefs {
			if ex == refName {
				return nil
			}
		}
		p.SecretRefs = append(p.SecretRefs, refName)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "marked %s secret (whole tree encrypted at rest)\n", ref.ShortName(refName))
	return nil
}

// secretRotate re-encrypts every secret object reachable from a ref to the
// current recipient set. The plaintext-addressed object hash is unchanged, so
// no new commit is created — only the ciphertext at rest is rewritten. Run this
// after granting or revoking read access.
func secretRotate(r *repo.Repo, store *object.Store, rest []string, out io.Writer) error {
	refName, err := r.Head()
	if err != nil {
		return err
	}
	if len(rest) == 1 {
		refName = resolveRefName(r, rest[0])
	}
	id, err := identity.Load()
	if err != nil {
		return err
	}
	recips, err := policy.RecipientsFor(r.DB, store, refName)
	if err != nil {
		return err
	}
	if len(recips) == 0 {
		return fmt.Errorf("%s has no readers; nothing to rotate to", ref.ShortName(refName))
	}
	target, err := ref.Resolve(r.DB, refName)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("%s has no commits", ref.ShortName(refName))
	}
	tree, err := store.TreeOfCommit(target)
	if err != nil {
		return err
	}
	files, err := object.FlattenFull(store, tree)
	if err != nil {
		return err
	}
	rotated, seen := 0, map[string]bool{}
	for _, fe := range files {
		if fe.Type != object.Secret || seen[fe.Hash] {
			continue
		}
		seen[fe.Hash] = true
		_, ciphertext, err := store.Get(fe.Hash)
		if err != nil {
			return err
		}
		plaintext, err := secret.Decrypt(ciphertext, id.X25519)
		if err != nil {
			return fmt.Errorf("cannot rotate %s: you are not a current recipient", fe.Hash[:10])
		}
		fresh, err := secret.Encrypt(plaintext, recips)
		if err != nil {
			return err
		}
		if err := store.PutRaw(fe.Hash, object.Secret, fresh); err != nil {
			return err
		}
		rotated++
	}
	if err := config.Set(r.DB, rotatedKey(refName), recipientsFingerprint(recips)); err != nil {
		return err
	}
	fmt.Fprintf(out, "rotated %d secret object(s) on %s to %d recipient(s)\n", rotated, ref.ShortName(refName), len(recips))
	return nil
}

// secretStatus reports recipient drift: refs whose current readers differ from
// the recipient set the secrets were last encrypted to.
func secretStatus(r *repo.Repo, store *object.Store, out io.Writer) error {
	refs, err := ref.List(r.DB)
	if err != nil {
		return err
	}
	drift := false
	for _, rf := range refs {
		recips, err := policy.RecipientsFor(r.DB, store, rf.Name)
		if err != nil || len(recips) == 0 {
			continue
		}
		want := recipientsFingerprint(recips)
		got, _, _ := config.Get(r.DB, rotatedKey(rf.Name))
		if got != "" && got != want {
			fmt.Fprintf(out, "DRIFT %s: readers changed since last rotation — run 'hv secret rotate %s'\n",
				ref.ShortName(rf.Name), ref.ShortName(rf.Name))
			drift = true
		}
	}
	if !drift {
		fmt.Fprintln(out, "no secret drift")
	}
	return nil
}

// resolveRefName maps a short name to a full ref: an existing branch/haven if
// one matches, else a branch by default. Full "refs/..." names pass through.
func resolveRefName(r *repo.Repo, name string) string {
	if strings.HasPrefix(name, "refs/") {
		return name
	}
	for _, prefix := range []string{ref.BranchPrefix, ref.HavenPrefix} {
		if t, _ := ref.Resolve(r.DB, prefix+name); t != "" {
			return prefix + name
		}
	}
	return ref.BranchPrefix + name
}

// armSecretBaseline records the recipient fingerprint the first time secrets
// appear on a ref, so `secret status` has a baseline to detect drift against.
// It never overwrites an existing baseline (set by a prior commit or rotate).
func armSecretBaseline(r *repo.Repo, store *object.Store, refName, treeHash string) error {
	if got, _, _ := config.Get(r.DB, rotatedKey(refName)); got != "" {
		return nil
	}
	files, err := object.FlattenFull(store, treeHash)
	if err != nil {
		return err
	}
	hasSecret := false
	for _, fe := range files {
		if fe.Type == object.Secret {
			hasSecret = true
			break
		}
	}
	if !hasSecret {
		return nil
	}
	recips, err := policy.RecipientsFor(r.DB, store, refName)
	if err != nil || len(recips) == 0 {
		return err
	}
	return config.Set(r.DB, rotatedKey(refName), recipientsFingerprint(recips))
}

func rotatedKey(refName string) string { return "secret.rotated." + refName }

// recipientsFingerprint is a stable hash of a recipient set, order-independent.
func recipientsFingerprint(recips []string) string {
	sorted := append([]string(nil), recips...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}
