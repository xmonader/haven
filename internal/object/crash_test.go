package object

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"haven/internal/hash"
	"haven/internal/ref"
	"haven/internal/store"
)

// TestCrashRecovery is a crash-injection test: it repeatedly SIGKILLs a child
// process while that child is in a tight commit loop, then reopens the same
// repository and asserts it is never corrupt.
//
// The invariant under test comes from the write ordering in the commit path
// (PutCommit and the other object writes happen BEFORE the ref is advanced):
// after a crash the repository must be at either the old or a new consistent
// state, never a torn one. Concretely, on reopen:
//
//  1. every stored object's content must still hash to its key, and
//  2. the branch ref must resolve to a fully present commit graph (or be unborn
//     if the crash landed before the first ref update).
//
// A crash may legitimately leave orphan objects (a commit written whose ref
// update never landed); gc collects those and they do not violate either rule.
//
// SIGKILL (not SIGTERM) is used deliberately: it gives the process no chance to
// flush or clean up, so recovery rests entirely on SQLite's WAL crash recovery
// and our transaction boundaries — exactly what we want to stress.
func TestCrashRecovery(t *testing.T) {
	// Re-exec hook: when the env var is set, this process IS the child. It runs
	// the write loop forever and is expected to be killed by the parent.
	if dbPath := os.Getenv("HV_CRASH_DB"); dbPath != "" {
		crashChildLoop(dbPath)
		return
	}
	if testing.Short() {
		t.Skip("crash-injection test skipped in -short mode")
	}

	const iterations = 20
	maxCommits := 0
	for iter := 0; iter < iterations; iter++ {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "haven.db")

		cmd := exec.Command(os.Args[0], "-test.run=^TestCrashRecovery$")
		cmd.Env = append(os.Environ(), "HV_CRASH_DB="+dbPath)
		if err := cmd.Start(); err != nil {
			t.Fatalf("iter %d: start child: %v", iter, err)
		}
		// Vary the kill delay so SIGKILL lands at different points in the write
		// loop across iterations, frequently mid-transaction.
		time.Sleep(time.Duration(15+iter*5) * time.Millisecond)
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait() // reaps the killed child; non-nil error is expected

		if c := verifyRepoIntact(t, dbPath, iter); c > maxCommits {
			maxCommits = c
		}
	}
	// Non-vacuity guard: if the child never managed to write a commit chain
	// before being killed, the test proved nothing. Require that at least one
	// iteration survived with a real chain, so the SIGKILLs are landing during
	// active writing rather than before any work happened.
	if maxCommits < 2 {
		t.Fatalf("test was vacuous: deepest surviving chain had %d commits; kills landed before any meaningful writes", maxCommits)
	}
	t.Logf("crash-injection: %d iterations, deepest surviving commit chain = %d", iterations, maxCommits)
}

// TestReachableRejectsDanglingRef is the negative control for TestCrashRecovery:
// it proves the integrity check actually has teeth by confirming Reachable
// errors on a ref target whose object is absent. If this ever passes silently,
// the crash test's Rule 2 assertion would be worthless.
func TestReachableRejectsDanglingRef(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewStore(db)
	const bogus = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if err := ref.Set(db, "refs/branches/dangling", bogus); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reachable(bogus); err == nil {
		t.Fatal("Reachable accepted a target whose object is absent; integrity check has no teeth")
	}
}

// crashChildLoop opens the repo and writes commit chains as fast as it can,
// never returning. The parent kills it. It exits non-zero on any setup error so
// a broken child surfaces rather than masquerading as a clean crash.
func crashChildLoop(dbPath string) {
	db, err := store.Open(dbPath)
	if err != nil {
		os.Exit(10)
	}
	s := NewStore(db)
	parent := ""
	for i := 0; ; i++ {
		blob, err := s.Put(Blob, []byte("payload-"+strconv.Itoa(i)))
		if err != nil {
			os.Exit(11)
		}
		tree, err := s.PutTree([]TreeEntry{{Mode: ModeFile, Type: Blob, Hash: blob, Name: "f"}})
		if err != nil {
			os.Exit(12)
		}
		var parents []string
		if parent != "" {
			parents = []string{parent}
		}
		commit, err := s.PutCommit(CommitObj{
			Tree: tree, Parents: parents,
			Author: "t", Email: "t@t", When: int64(i), Message: "c" + strconv.Itoa(i),
		})
		if err != nil {
			os.Exit(13)
		}
		if err := ref.Set(db, "refs/branches/main", commit); err != nil {
			os.Exit(14)
		}
		parent = commit
	}
}

// verifyRepoIntact reopens the (possibly crash-damaged) repo and asserts the two
// integrity rules. A failure here is a real durability bug, not flakiness. It
// returns the number of commits reachable from main, so the caller can confirm
// the kills landed during active writing.
func verifyRepoIntact(t *testing.T, dbPath string, iter int) int {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("iter %d: reopen after crash failed (corrupt db / failed migration?): %v", iter, err)
	}
	defer db.Close()
	s := NewStore(db)

	// Rule 1: every object hashes to its key.
	if err := s.Each(func(h string, typ Type, content []byte) error {
		if typ == Secret {
			return nil // ciphertext, addressed by plaintext hash; can't re-verify here
		}
		if got := hash.Of(string(typ), content); got != h {
			t.Errorf("iter %d: corrupt %s object stored as %s but hashes to %s", iter, typ, h, got)
		}
		return nil
	}); err != nil {
		t.Fatalf("iter %d: scanning objects: %v", iter, err)
	}

	// Rule 2: the branch ref resolves to a fully present commit graph (or is unborn).
	target, err := ref.Resolve(db, "refs/branches/main")
	if err != nil {
		t.Fatalf("iter %d: resolve main: %v", iter, err)
	}
	if target == "" {
		return 0 // crashed before the first ref update — a valid empty state
	}
	reachable, err := s.Reachable(target)
	if err != nil {
		t.Errorf("iter %d: ref main -> %s is dangling after crash: %v", iter, target, err)
		return 0
	}
	// Count commits in the reachable set by walking parents from the tip.
	commits := 0
	for h := target; h != ""; {
		c, err := s.GetCommit(h)
		if err != nil {
			t.Errorf("iter %d: walking history hit missing commit %s: %v", iter, h, err)
			break
		}
		commits++
		if len(c.Parents) == 0 {
			break
		}
		h = c.Parents[0]
	}
	_ = reachable
	return commits
}
