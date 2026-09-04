package utils

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestHashAndCheckPassword_Roundtrip(t *testing.T) {
	hash, err := HashPassword("uma-senha-qualquer")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !CheckPassword(hash, "uma-senha-qualquer") {
		t.Error("the password should have matched its own hash")
	}
	if CheckPassword(hash, "outra-senha") {
		t.Error("a wrong password should not match")
	}
}

// The gate is the whole point of the package, so the test asserts the property that
// makes it worth having: no matter how many logins arrive at once, only so many hashes
// run concurrently, and one core is left for everything that is not a login.
func TestHashGate_BoundsConcurrency(t *testing.T) {
	limit := cap(hashGate)
	if want := maxConcurrentHashes(); limit != want {
		t.Fatalf("gate capacity is %d, want %d", limit, want)
	}

	var running, peak int64
	var wg sync.WaitGroup

	for i := 0; i < limit*8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hashGate <- struct{}{}
			now := atomic.AddInt64(&running, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if now <= old || atomic.CompareAndSwapInt64(&peak, old, now) {
					break
				}
			}
			// Long enough that the goroutines genuinely overlap; a hash on the
			// production hardware takes far longer than this.
			runtime.Gosched()
			atomic.AddInt64(&running, -1)
			<-hashGate
		}()
	}
	wg.Wait()

	if peak > int64(limit) {
		t.Errorf("%d hashes ran at once, gate allows %d", peak, limit)
	}
}

func TestMaxConcurrentHashes_LeavesACoreFree(t *testing.T) {
	got := maxConcurrentHashes()
	if got < 1 {
		t.Fatalf("the gate must admit at least one hasher, got %d", got)
	}
	if procs := runtime.GOMAXPROCS(0); procs > 1 && got >= procs {
		t.Errorf("gate admits %d hashers on %d cores, leaving none free", got, procs)
	}
}
