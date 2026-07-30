package bot

import (
	"context"
	"testing"
	"time"
)

func newTrackedJob(messageID, locator string, kind triggerKind) *job {
	return &job{
		evt:       newChatMessage("1234567890", messageID, ""),
		kind:      kind,
		locator:   locator,
		startedAt: time.Now(),
		acked:     make(chan struct{}),
		finished:  make(chan struct{}),
	}
}

// The lock is taken when a job starts, not when it is admitted, so two messages
// about the same PNR queued in one chat are not in conflict - the chat's worker
// already runs them one after the other.
func TestJobRegistry_AcquireIsExclusivePerLocator(t *testing.T) {
	r := newJobRegistry()

	first := newTrackedJob("m1", "ABC123", kindEmissao)
	second := newTrackedJob("m2", "ABC123", kindAlteracao)
	r.track(first)
	r.track(second)

	if !r.acquire(first, first.locator) {
		t.Fatal("the first job must get the lock")
	}
	if r.acquire(second, second.locator) {
		t.Fatal("a second job must not hold the same locator at the same time")
	}

	r.untrack(first)

	if !r.acquire(second, second.locator) {
		t.Fatal("the lock must be free once the holder finished")
	}
}

func TestJobRegistry_DifferentLocatorsDoNotContend(t *testing.T) {
	r := newJobRegistry()

	first := newTrackedJob("m1", "ABC123", kindEmissao)
	second := newTrackedJob("m2", "XYZ789", kindEmissao)
	r.track(first)
	r.track(second)

	if !r.acquire(first, first.locator) || !r.acquire(second, second.locator) {
		t.Fatal("two different PNRs must run at the same time")
	}
}

// A command has no PNR, and nothing about it is exclusive.
func TestJobRegistry_JobsWithoutALocatorNeverContend(t *testing.T) {
	r := newJobRegistry()

	first := newTrackedJob("m1", "", kindCommand)
	second := newTrackedJob("m2", "", kindCommand)
	r.track(first)
	r.track(second)

	if !r.acquire(first, first.locator) || !r.acquire(second, second.locator) {
		t.Fatal("jobs with no locator must never block each other")
	}
}

func TestJobRegistry_LooksJobsUpBothWays(t *testing.T) {
	r := newJobRegistry()

	emissao := newTrackedJob("m1", "ABC123", kindEmissao)
	alteracao := newTrackedJob("m2", "ABC123", kindAlteracao)
	r.track(emissao)
	r.track(alteracao)

	if found, ok := r.lookupByMessage("m1"); !ok || found != emissao {
		t.Fatal("lookupByMessage must find the job a message started")
	}
	// Both queued at once is normal: an EMISSAO and the ALTERACAO correcting it
	// can be sitting in the same chat's queue before either runs.
	if got := r.lookupByLocator("ABC123"); len(got) != 2 {
		t.Fatalf("lookupByLocator: got %d jobs, want 2", len(got))
	}
}

// The registry describes work in progress and nothing else, so a finished job
// leaves no trace in it. That is what keeps it proportional to what is in flight
// rather than to history.
func TestJobRegistry_UntrackRemovesEveryIndex(t *testing.T) {
	r := newJobRegistry()

	j := newTrackedJob("m1", "ABC123", kindEmissao)
	r.track(j)
	r.untrack(j)

	if _, ok := r.lookupByMessage("m1"); ok {
		t.Fatal("a finished job must not be findable by message id")
	}
	if got := r.lookupByLocator("ABC123"); len(got) != 0 {
		t.Fatalf("lookupByLocator: got %v, want nothing", got)
	}

	// Untracking one of two must leave the other alone.
	first := newTrackedJob("m1", "ABC123", kindEmissao)
	second := newTrackedJob("m2", "ABC123", kindAlteracao)
	r.track(first)
	r.track(second)
	r.untrack(first)

	if got := r.lookupByLocator("ABC123"); len(got) != 1 || got[0] != second {
		t.Fatalf("lookupByLocator after untracking one: got %v, want only the second job", got)
	}
}

// State 1 of the three: cancelled while queued. The job never starts, so nothing
// it would have written exists.
func TestJob_CancelWhileQueuedStopsItFromStarting(t *testing.T) {
	j := newTrackedJob("m1", "ABC123", kindEmissao)

	if !j.cancel(true) {
		t.Fatal("cancelling a queued job must succeed")
	}
	if _, live := j.begin(context.Background()); live {
		t.Fatal("a cancelled job must never begin")
	}
}

// State 2: cancelled while running. The context it was handed is pulled, which
// is what aborts the statement it is inside and rolls its transaction back.
func TestJob_CancelWhileRunningCancelsItsContext(t *testing.T) {
	j := newTrackedJob("m1", "ABC123", kindEmissao)

	ctx, live := j.begin(context.Background())
	if !live {
		t.Fatal("a fresh job must begin")
	}
	if ctx.Err() != nil {
		t.Fatalf("a running job's context must start alive, got %v", ctx.Err())
	}

	if !j.cancel(true) {
		t.Fatal("cancelling a running job must succeed")
	}
	if ctx.Err() == nil {
		t.Fatal("cancelling a running job must cancel the context its handler is using")
	}
	if !j.isCancelled() {
		t.Fatal("a cancelled job must say so")
	}
}

// State 3, from the cancelling side: too late. The caller has to undo the row in
// the database instead, which is exactly what DELETAR falls through to.
func TestJob_CancelAfterFinishReportsItIsTooLate(t *testing.T) {
	j := newTrackedJob("m1", "ABC123", kindEmissao)

	if _, live := j.begin(context.Background()); !live {
		t.Fatal("a fresh job must begin")
	}
	j.finish()

	if j.cancel(true) {
		t.Fatal("cancelling a finished job must report failure - the work is already written")
	}
}
