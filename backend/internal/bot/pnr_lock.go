package bot

import (
	"context"
	"sync"
	"time"
)

// jobState is where a job is in its life. There is no terminal state: a job
// that finished is removed from the registry rather than marked done, because
// the whole point of the registry is to describe work still in progress.
type jobState int

const (
	// jobQueued: admitted, waiting its turn behind the rest of the chat.
	jobQueued jobState = iota
	// jobRunning: the worker is executing it right now.
	jobRunning
)

// Lock order, wherever both are held: jobRegistry.mu first, then job.mu. No
// path takes them the other way round, and none needs to.
//
// jobRegistry is the bot's view of the work it has in progress, indexed the two
// ways something needs to find it:
//
//   - byMessage, for a native edit or deletion of the WhatsApp message that
//     started the job. That is the map the product doc calls
//     "message_id → {issuance_id, pnr}", and like it this one lives only as long
//     as the job does - it is proportional to work in flight, not to history, so
//     it cannot grow without bound. The separate issued map (issued_messages.go)
//     is the equivalent index for work that already finished.
//   - byLocator, for DELETAR, which names a PNR rather than a message, and for
//     the lock itself.
//
// The lock is the second half of this: at most one job may be *executing* a
// given locator at a time. It is not taken when the job is queued, only when it
// starts - two messages about the same PNR in the same chat are already
// serialized by that chat's worker, and refusing the second one there would
// break the very ordering the queue exists to provide. What it does catch is the
// same PNR arriving in two different chats at once, which nothing else guards
// against.
type jobRegistry struct {
	mu        sync.Mutex
	byMessage map[string]*job
	byLocator map[string][]*job
	// held maps a locator to the job currently executing it. Separate from
	// byLocator because being tracked and holding the lock are different things:
	// a queued job is tracked but holds nothing.
	held map[string]*job
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{
		byMessage: make(map[string]*job),
		byLocator: make(map[string][]*job),
		held:      make(map[string]*job),
	}
}

// track records a job as work in progress. Called by intake, before the job
// reaches the worker, so an edit or a deletion arriving while it is still
// queued can still find it.
func (r *jobRegistry) track(j *job) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.indexLocked(j)
}

// indexLocked adds a job to both indexes. Caller holds r.mu.
func (r *jobRegistry) indexLocked(j *job) {
	if id := j.evt.Info.ID; id != "" {
		r.byMessage[string(id)] = j
	}

	j.mu.Lock()
	locator := j.locator
	j.mu.Unlock()

	if locator != "" {
		r.byLocator[locator] = append(r.byLocator[locator], j)
	}
}

// dropLocated removes a job from the locator index. Caller holds r.mu.
func (r *jobRegistry) dropLocated(j *job, locator string) {
	if locator == "" {
		return
	}
	if r.held[locator] == j {
		delete(r.held, locator)
	}

	remaining := r.byLocator[locator][:0]
	for _, tracked := range r.byLocator[locator] {
		if tracked != j {
			remaining = append(remaining, tracked)
		}
	}
	if len(remaining) == 0 {
		delete(r.byLocator, locator)
		return
	}
	r.byLocator[locator] = remaining
}

// untrack forgets a job that finished, and releases its lock if it still held
// one. Always paired with track through a defer, so a panicking handler cannot
// leave a locator locked forever.
func (r *jobRegistry) untrack(j *job) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only if it is still this job's entry. A native edit of a running job
	// cancels it and enqueues a replacement under the same message id, and the
	// cancelled one finishing later must not evict its successor.
	if id := string(j.evt.Info.ID); r.byMessage[id] == j {
		delete(r.byMessage, id)
	}

	j.mu.Lock()
	locator := j.locator
	j.mu.Unlock()

	r.dropLocated(j, locator)
}

// replaceIfQueued rewrites a job that has not started yet, in place.
//
// This is what a native edit of a queued message does. Keeping the same job -
// rather than cancelling it and enqueueing a replacement - is what keeps its
// place in the chat's queue, and that position is load-bearing: an EMISSAO with
// an ALTERACAO already queued behind it would otherwise be moved after the
// correction, which would then find nothing to correct.
//
// Returns false when the job is already running or over, which is the caller's
// cue to cancel it and let the edit run on its own instead.
func (r *jobRegistry) replaceIfQueued(j *job, text string, kind triggerKind, locator string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	j.mu.Lock()
	if j.state != jobQueued || j.cancelled || j.done {
		j.mu.Unlock()
		return false
	}
	previous := j.locator
	j.text, j.kind, j.locator = text, kind, locator
	j.mu.Unlock()

	if previous != locator {
		r.dropLocated(j, previous)
		if locator != "" {
			r.byLocator[locator] = append(r.byLocator[locator], j)
		}
	}
	return true
}

// acquire takes the locator's lock for a job about to run. It returns false when
// another job is already executing that locator - necessarily from another chat,
// since one chat runs one job at a time - and the caller refuses the message
// rather than waiting: two operators issuing the same PNR at the same moment is
// a mistake, and making the second one wait for the first would only turn it
// into a duplicate a few seconds later.
//
// A job with no locator (a command, or a message whose first line did not
// parse) locks nothing and always proceeds.
func (r *jobRegistry) acquire(j *job, locator string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if locator == "" {
		return true
	}
	if holder, taken := r.held[locator]; taken && holder != j {
		return false
	}
	r.held[locator] = j
	return true
}

// lookupByMessage returns the live job a WhatsApp message started, if it has not
// finished yet.
func (r *jobRegistry) lookupByMessage(messageID string) (*job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	j, found := r.byMessage[messageID]
	return j, found
}

// lookupByLocator returns every live job for a locator. More than one is normal:
// an EMISSAO and the ALTERACAO correcting it can both be queued in the same chat
// before either runs.
func (r *jobRegistry) lookupByLocator(locator string) []*job {
	r.mu.Lock()
	defer r.mu.Unlock()

	if locator == "" {
		return nil
	}
	return append([]*job(nil), r.byLocator[locator]...)
}

// jobs returns the registry, starting it on first use - same lazy pattern as
// the dispatcher, and for the same reason: every Bot has one, including the
// struct literals the tests build directly.
func (b *Bot) jobs() *jobRegistry {
	b.jobsOnce.Do(func() { b.jobRegistry = newJobRegistry() })
	return b.jobRegistry
}

// cancel stops a job wherever it happens to be.
//
// A queued job is marked and never starts - the worker checks on the way in, and
// nothing it would have written exists yet. A running job has its context
// cancelled, which aborts the statement it is inside and rolls its transaction
// back; the handler sees a cancelled context rather than a database failure and
// answers nothing, because whoever cancelled it is the one who owes the group an
// answer.
//
// closeIndicator settles what happens to the ✅ → 🕐 already sitting on the
// cancelled message. A reaction is only ever replaced, never cleared, so a job
// that just stops leaves a clock on screen for good. Callers pass false only when
// something else is going to answer on that very message - an edit that will be
// processed in its place - and true when nobody else will, which is a DELETAR
// naming the PNR from a message of its own.
//
// Returns false when the job had already finished, which is the caller's cue
// that the work is a row in the database now and has to be undone there instead.
func (j *job) cancel(closeIndicator bool) bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.done {
		return false
	}
	j.cancelled = true
	j.closeIndicator = closeIndicator
	if j.cancelFunc != nil {
		j.cancelFunc()
	}
	return true
}

// isCancelled reports whether this job was cancelled before or during its run.
func (j *job) isCancelled() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cancelled
}

// wasInterrupted reports a cancellation that owes the message a closing
// reaction: stopped, and with nothing else lined up to answer in its place.
func (j *job) wasInterrupted() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cancelled && j.closeIndicator
}

// payload reads what the job is currently about. Read through this rather than
// off the fields: a native edit can rewrite all three while the job waits its
// turn - see jobRegistry.replaceIfQueued.
func (j *job) payload() (text string, kind triggerKind, locator string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.text, j.kind, j.locator
}

// begin marks the job as running and installs the context that cancel will
// pull. Returns false when the job was already cancelled while queued, which is
// how a DELETAR or a native deletion stops work that never started.
func (j *job) begin(parent context.Context) (context.Context, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.cancelled {
		return nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	j.state = jobRunning
	j.cancelFunc = cancel
	return ctx, true
}

// finish marks the job as over, so a cancellation racing the end of the run is
// told the truth: too late, it is already written.
func (j *job) finish() {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.done {
		return
	}
	j.done = true
	if j.cancelFunc != nil {
		j.cancelFunc()
	}
	if j.finished != nil {
		close(j.finished)
	}
}

// awaitFinish blocks until the job's handler returned, or until the deadline.
// Cancelling a running job aborts the statement it is inside, but the rollback
// lands a moment later - reading the database before that would see neither the
// row nor its absence reliably.
func (j *job) awaitFinish(timeout time.Duration) {
	j.mu.Lock()
	finished := j.finished
	j.mu.Unlock()

	if finished == nil {
		return
	}
	select {
	case <-finished:
	case <-time.After(timeout):
	}
}

// cancelLocator stops every live job for a locator, skipping the one whose own
// message id is exceptMessageID - a DELETAR is tracked under the same locator it
// is about to cancel, and would otherwise cancel itself.
//
// It returns the jobs it actually stopped. A job that had already finished is
// not one of them: its work is a row in the database now, and the caller has to
// undo it there instead.
func (r *jobRegistry) cancelLocator(locator, exceptMessageID string) []*job {
	var stopped []*job

	for _, j := range r.lookupByLocator(locator) {
		if string(j.evt.Info.ID) == exceptMessageID {
			continue
		}
		// The cancelled message is a different message from the one asking for
		// the cancellation, and it is still sitting in the chat under a 🕐 that
		// nothing else will ever take off it.
		if j.cancel(true) {
			stopped = append(stopped, j)
		}
	}
	return stopped
}
