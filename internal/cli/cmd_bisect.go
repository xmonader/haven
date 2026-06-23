package cli

import (
	"fmt"
	"io"
	"strings"

	"haven/internal/config"
	"haven/internal/lock"
	"haven/internal/object"
	"haven/internal/ref"
	"haven/internal/repo"
	"haven/internal/workspace"
)

const bisectRef = "refs/bisect/work"

var cmdBisect = Command{
	Name:    "bisect",
	Summary: "start|good|bad|reset — binary-search history for a bad commit",
	Run:     runBisect,
}

func runBisect(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hv bisect start|good|bad|reset [<rev>]")
	}
	sub, rest := args[0], args[1:]

	r, store, err := openRepo()
	if err != nil {
		return err
	}
	defer r.Close()

	switch sub {
	case "start":
		return bisectStart(r, out)
	case "good", "bad":
		return bisectMark(r, store, sub, rest, out)
	case "reset":
		return bisectReset(r, store, out)
	default:
		return fmt.Errorf("unknown subcommand %q (want start|good|bad|reset)", sub)
	}
}

func bisectActive(r *repo.Repo) bool {
	v, _, _ := config.Get(r.DB, "bisect.active")
	return v == "1"
}

func bisectStart(r *repo.Repo, out io.Writer) error {
	if bisectActive(r) {
		return fmt.Errorf("a bisect is already in progress; run 'hv bisect reset' first")
	}
	headRef, err := r.Head()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(headRef, ref.BranchPrefix) && !strings.HasPrefix(headRef, ref.HavenPrefix) {
		return fmt.Errorf("start bisect from a branch or haven")
	}
	for k, v := range map[string]string{
		"bisect.active": "1",
		"bisect.orig":   headRef,
		"bisect.bad":    "",
		"bisect.good":   "",
	} {
		if err := config.Set(r.DB, k, v); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "bisect started; mark commits with 'hv bisect good' / 'hv bisect bad'")
	return nil
}

func bisectMark(r *repo.Repo, store *object.Store, kind string, rest []string, out io.Writer) error {
	if !bisectActive(r) {
		return fmt.Errorf("no bisect in progress; run 'hv bisect start'")
	}
	spec := "HEAD"
	if len(rest) == 1 {
		spec = rest[0]
	}
	commit, err := resolveCommit(r, spec)
	if err != nil {
		return err
	}
	if commit == "" {
		return fmt.Errorf("%s has no commit", spec)
	}

	if kind == "bad" {
		if err := config.Set(r.DB, "bisect.bad", commit); err != nil {
			return err
		}
	} else {
		good, _, _ := config.Get(r.DB, "bisect.good")
		if !strings.Contains(" "+good+" ", " "+commit+" ") {
			good = strings.TrimSpace(good + " " + commit)
		}
		if err := config.Set(r.DB, "bisect.good", good); err != nil {
			return err
		}
	}

	bad, _, _ := config.Get(r.DB, "bisect.bad")
	goodCSV, _, _ := config.Get(r.DB, "bisect.good")
	if bad == "" || strings.TrimSpace(goodCSV) == "" {
		fmt.Fprintln(out, "need at least one good and one bad commit to bisect")
		return nil
	}

	suspects, err := suspectSet(store, bad, strings.Fields(goodCSV))
	if err != nil {
		return err
	}
	// Choose the next commit to test (the most balanced split, excluding the
	// known-bad tip).
	test, remaining := bisectPick(store, suspects, bad)
	if test == "" {
		fmt.Fprintf(out, "first bad commit is %s\n", short(bad))
		if c, err := store.GetCommit(bad); err == nil {
			fmt.Fprintf(out, "  %s\n", firstLine(c.Message))
		}
		fmt.Fprintln(out, "run 'hv bisect reset' to finish")
		return nil
	}

	// Check out the test commit on the bisect work ref.
	if err := bisectCheckout(r, store, test); err != nil {
		return err
	}
	fmt.Fprintf(out, "bisecting: %d commit(s) left; testing %s\n", remaining, short(test))
	return nil
}

// bisectCheckout materializes target on the bisect work ref, moving HEAD there.
// It resolves the current location BEFORE repointing the work ref, so the
// clean-tree check compares against where the tree actually is.
func bisectCheckout(r *repo.Repo, store *object.Store, target string) error {
	wc, err := lock.Acquire(r.Root)
	if err != nil {
		return err
	}
	defer wc.Release()

	headRef, err := r.Head()
	if err != nil {
		return err
	}
	cur, err := ref.Resolve(r.DB, headRef)
	if err != nil {
		return err
	}
	curTree, err := store.TreeOfCommit(cur)
	if err != nil {
		return err
	}
	marks, err := marksOf(r)
	if err != nil {
		return err
	}
	if clean, err := workspace.IsClean(r.Root, store, curTree, marks); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or discard first")
	}
	toTree, err := store.TreeOfCommit(target)
	if err != nil {
		return err
	}
	if err := workspace.Checkout(r.Root, store, curTree, toTree, currentIdentity()); err != nil {
		return err
	}
	if err := ref.SetVisible(r.DB, bisectRef, target, ref.Public); err != nil {
		return err
	}
	if headRef != bisectRef {
		if err := r.SetHead(bisectRef); err != nil {
			return err
		}
	}
	return resetStaging(r, store, toTree)
}

func bisectReset(r *repo.Repo, store *object.Store, out io.Writer) error {
	if !bisectActive(r) {
		return fmt.Errorf("no bisect in progress")
	}
	orig, _, _ := config.Get(r.DB, "bisect.orig")
	if orig != "" {
		if err := switchTo(r, store, orig); err != nil {
			return err
		}
	}
	if t, _ := ref.Resolve(r.DB, bisectRef); t != "" {
		if err := ref.Delete(r.DB, bisectRef); err != nil {
			return err
		}
	}
	if err := config.Set(r.DB, "bisect.active", ""); err != nil {
		return err
	}
	fmt.Fprintf(out, "bisect reset; back on %s\n", ref.ShortName(orig))
	return nil
}

// suspectSet is the commits reachable from bad but from no good commit — the
// range that may contain the first bad commit.
func suspectSet(store *object.Store, bad string, goods []string) (map[string]bool, error) {
	suspects, err := ancestorSet(store, bad)
	if err != nil {
		return nil, err
	}
	for _, g := range goods {
		ga, err := ancestorSet(store, g)
		if err != nil {
			return nil, err
		}
		for h := range ga {
			delete(suspects, h)
		}
	}
	return suspects, nil
}

// bisectPick returns the suspect (other than the known-bad tip) that most evenly
// splits the suspect set, plus the suspect count. Returns "" when bad is the
// only suspect (the culprit is found).
func bisectPick(store *object.Store, suspects map[string]bool, bad string) (string, int) {
	best, bestScore := "", -1
	for c := range suspects {
		if c == bad {
			continue
		}
		anc, err := ancestorSet(store, c)
		if err != nil {
			continue
		}
		below := 0
		for s := range suspects {
			if anc[s] {
				below++
			}
		}
		above := len(suspects) - below
		score := below
		if above < below {
			score = above
		}
		if score > bestScore {
			best, bestScore = c, score
		}
	}
	return best, len(suspects)
}

// ancestorSet returns start and all commits reachable from it via parents.
func ancestorSet(store *object.Store, start string) (map[string]bool, error) {
	seen := map[string]bool{}
	stack := []string{start}
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		c, err := store.GetCommit(h)
		if err != nil {
			return nil, err
		}
		stack = append(stack, c.Parents...)
	}
	return seen, nil
}
