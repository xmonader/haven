package protocol

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"haven/internal/object"
)

// TestConcurrentObjectLoad drives many clients at one server/db simultaneously
// and asserts that under contention every object round-trips correctly and the
// store ends up holding every object, hash-valid. Run with -race (CI does) this
// also exercises the server's concurrent-write path for data races.
func TestConcurrentObjectLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("load test skipped in -short mode")
	}
	c, srvStore := newServer(t, KindTeam)

	// Enough concurrency to trip the race detector and exercise the single-writer
	// SQLite contention path, sized to stay fast under -race in CI.
	const workers, perWorker = 8, 24
	var wg sync.WaitGroup
	errCh := make(chan error, workers*perWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				content := []byte(fmt.Sprintf("worker-%d-item-%d-payload", w, i))
				h := objectHash(content)
				if err := c.PutObject(h, object.Blob, content); err != nil {
					errCh <- fmt.Errorf("put w%d i%d: %w", w, i, err)
					return
				}
				typ, got, err := c.GetObject(h)
				if err != nil {
					errCh <- fmt.Errorf("get w%d i%d: %w", w, i, err)
					return
				}
				if typ != object.Blob || string(got) != string(content) {
					errCh <- fmt.Errorf("roundtrip mismatch w%d i%d: got (%s,%q)", w, i, typ, got)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	// Every distinct object must be present and hash-valid. Contents are unique
	// per (worker,item), so the store must hold exactly workers*perWorker blobs.
	count := 0
	if err := srvStore.Each(func(h string, typ object.Type, content []byte) error {
		count++
		if got := objectHash(content); got != h {
			t.Errorf("corrupt object %s hashes to %s", h, got)
		}
		return nil
	}); err != nil {
		t.Fatalf("scanning store: %v", err)
	}
	if want := workers * perWorker; count != want {
		t.Errorf("expected %d distinct objects in store, got %d", want, count)
	}
}

// TestConcurrentRefCASNoLostUpdate proves the compare-and-swap ref update is
// safe under contention: when many clients race to create the same ref from the
// unborn state, exactly one must win — never zero (deadlock/total failure) and
// never more than one (lost update / split-brain).
func TestConcurrentRefCASNoLostUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("load test skipped in -short mode")
	}
	c, _ := newServer(t, KindTeam)

	const racers = 24
	var wg sync.WaitGroup
	var wins int64
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := fmt.Sprintf("%064x", i+1)
			err := c.UpdateRef(RefUpdate{
				Name:       "refs/branches/race",
				Visibility: "public",
				Target:     target,
				OldTarget:  "", // all racers try to create from the unborn state
			})
			if err == nil {
				atomic.AddInt64(&wins, 1)
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("expected exactly 1 CAS winner creating the ref, got %d (lost-update or total-failure bug)", wins)
	}

	// A second wave with stale OldTarget="" must now all lose (the ref exists).
	wins = 0
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := c.UpdateRef(RefUpdate{
				Name:       "refs/branches/race",
				Visibility: "public",
				Target:     fmt.Sprintf("%064x", 1000+i),
				OldTarget:  "", // stale: ref is no longer unborn
			})
			if err == nil {
				atomic.AddInt64(&wins, 1)
			}
		}(i)
	}
	wg.Wait()
	if wins != 0 {
		t.Errorf("expected 0 winners with stale old_target, got %d (CAS not enforcing precondition)", wins)
	}
}

// BenchmarkObjectPut reports server-side object PUT throughput over HTTP with
// signing disabled (anonymous), a coarse sanity number for the write path.
func BenchmarkObjectPut(b *testing.B) {
	c, _ := newServer(b, KindTeam)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		content := []byte(fmt.Sprintf("bench-payload-%d", i))
		h := objectHash(content)
		if err := c.PutObject(h, object.Blob, content); err != nil {
			b.Fatalf("put: %v", err)
		}
	}
}
