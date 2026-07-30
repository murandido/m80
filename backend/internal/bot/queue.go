package bot

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

// Capacities of the two channels every monitored chat gets.
//
// intake is sized for burst tolerance rather than throughput: EventHandler runs
// on whatsmeow's dispatch goroutine and must never block, so it does a
// non-blocking send and a full channel means a dropped message. 256 is far more
// than a human group produces while the bot is still reading the first message.
//
// ready is smaller because falling behind there is not an emergency: the intake
// goroutine blocks on it, which simply stops handing out "queued" acknowledgements
// until the worker catches up - backpressure, not loss.
const (
	intakeCapacity = 256
	readyCapacity  = 64
)

// triggerKind is what an incoming message turned out to be. Resolved once, in
// the intake stage, so the worker never re-parses a message to find out which
// handler owns it.
type triggerKind int

const (
	kindNone triggerKind = iota
	kindCommand
	kindEmissao
	kindAlteracao
	kindCancelamento
	kindDeletar
	kindEdit
	kindRevoke
)

// queuedMessage is a message as it left EventHandler: still wrapped, still
// unread, not yet classified. isBacklog and startedAt are captured at dispatch
// time because neither can be recovered later - see EventHandler.
type queuedMessage struct {
	evt       *events.Message
	isBacklog bool
	startedAt time.Time
}

// job is an admitted message on its way to a handler.
type job struct {
	evt       *events.Message
	text      string
	kind      triggerKind
	startedAt time.Time

	// locator is the PNR the message names, or "" for a command and for a
	// message whose first line did not parse. It is what the per-PNR lock and
	// DELETAR index on - see jobRegistry.
	locator string

	// acked closes once every visible acknowledgement this message earned has
	// been sent. The worker waits on it so the handler's own reaction can never
	// overtake them and leave the wrong glyph showing - see admit.
	acked chan struct{}

	// finished closes when the handler returned, whatever it did. Whoever
	// cancels a job waits on this before looking at the database: cancelling a
	// running job aborts its statement, but the rollback is not instantaneous,
	// and a read taken too early would see neither the row nor the rollback.
	finished chan struct{}

	// mu guards everything below, which the worker and whoever is cancelling
	// the job touch from different goroutines.
	mu        sync.Mutex
	state     jobState
	cancelled bool
	// closeIndicator is set by whoever cancelled the job when no one else will
	// answer on the cancelled message - see job.cancel.
	closeIndicator bool
	done           bool
	cancelFunc     context.CancelFunc
}

// dispatcher owns one queue per chat, created the first time that chat sends
// anything. Chats are independent of each other on purpose: serializing all of
// them behind a single worker would make one slow group block every other one,
// and nothing about two different groups needs ordering between them. The one
// thing that does cross chats - two messages about the same PNR - is guarded by
// the per-PNR lock instead.
type dispatcher struct {
	bot *Bot

	mu     sync.Mutex
	chats  map[string]*chatQueue
	closed bool

	// wg counts every goroutine the queue is responsible for - the intake and
	// worker of each chat, plus the short-lived one each message spawns for its
	// visible actions - so Stop waits for the job actually running to finish
	// rather than cutting it off mid-transaction, and so a read receipt or a
	// reaction already on its way still lands.
	wg sync.WaitGroup
}

// spawn runs f in a goroutine the dispatcher will wait for on shutdown. Used
// for the per-message acknowledgement goroutine, which is deliberately not on
// the intake path - see admit - but must still be drained before the process
// exits.
func (d *dispatcher) spawn(f func()) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		f()
	}()
}

func newDispatcher(bot *Bot) *dispatcher {
	return &dispatcher{bot: bot, chats: make(map[string]*chatQueue)}
}

// submit hands a message to its chat's queue. It is called straight from
// whatsmeow's dispatch goroutine, so it must not block and must not do I/O:
// everything expensive (opening an edit's envelope, marking as read, hitting the
// database) happens in the intake goroutine on the other side of the channel.
//
// The send is what preserves ordering. whatsmeow dispatches messages of one chat
// in the order they arrived, and appending to the channel here - rather than
// spawning a goroutine per message, as this used to - keeps that order all the
// way to the handler. Before, every message slept a random 0.5-1.5s before doing
// anything, so two sent seconds apart could be processed backwards: an ALTERACAO
// could reach the database ahead of the EMISSAO it corrects.
func (d *dispatcher) submit(m queuedMessage) {
	q := d.queueFor(m.evt.Info.Chat.String())
	if q == nil {
		return
	}

	select {
	case q.intake <- m:
	default:
		// Unreachable in practice at this capacity, and silently dropping a
		// message the operator can see in the group would be the worst possible
		// failure - so it is loud.
		d.bot.Logger.Errorf("fila: intake for chat %s is full (%d messages), dropping message %s",
			m.evt.Info.Chat, intakeCapacity, m.evt.Info.ID)
	}
}

// queueFor returns the chat's queue, starting it on first use. Returns nil once
// the dispatcher is stopped, so a message arriving during shutdown is dropped
// rather than sent on a closed channel.
func (d *dispatcher) queueFor(chatJID string) *chatQueue {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	if q, ok := d.chats[chatJID]; ok {
		return q
	}

	q := &chatQueue{
		bot:    d.bot,
		jid:    chatJID,
		intake: make(chan queuedMessage, intakeCapacity),
		ready:  make(chan *job, readyCapacity),
	}
	d.chats[chatJID] = q

	d.wg.Add(2)
	go func() {
		defer d.wg.Done()
		q.runIntake()
	}()
	go func() {
		defer d.wg.Done()
		q.runWorker()
	}()

	return q
}

// Stop closes every queue and waits for the work already accepted to drain, or
// for ctx to expire - whichever comes first. Called from cmd/bot's shutdown
// path so a job mid-transaction gets to commit instead of dying with the
// process.
func (d *dispatcher) Stop(ctx context.Context) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	for _, q := range d.chats {
		close(q.intake)
	}
	d.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		d.bot.Logger.Warnf("fila: shutdown timed out with work still in flight")
	}
}

// chatQueue is one chat's pipeline, in two stages.
//
// They are separate because they answer to different clocks. Intake is what the
// group sees straight away - it decides whether the bot may act at all and says
// so with a reaction - and it must stay responsive even while a long job runs.
// The worker is where the actual work happens, one job at a time, in order.
// Collapsed into a single goroutine, the second message in a burst would sit
// unacknowledged until the first one finished; split like this, it is
// acknowledged immediately and still executed in turn.
type chatQueue struct {
	bot    *Bot
	jid    string
	intake chan queuedMessage
	ready  chan *job
}

// runIntake admits messages in arrival order and hands the admitted ones to the
// worker. Serial on purpose: the order jobs enter ready is the order they run.
func (q *chatQueue) runIntake() {
	// Closing ready here, rather than in Stop, is what makes the worker exit
	// exactly once everything already admitted has been handed over.
	defer close(q.ready)

	for m := range q.intake {
		j := q.bot.admit(m)
		if j.kind == kindNone {
			continue
		}

		// DELETAR and a native deletion must not wait their turn. Their whole
		// job is to interrupt work that is queued or already running, and a
		// message sitting behind that work could only ever arrive after it
		// finished - by which point there is nothing left to interrupt.
		if j.skipsQueue() {
			q.bot.queue().spawn(func() { q.bot.executeJob(j) })
			continue
		}

		q.ready <- j
	}
}

// skipsQueue reports whether this job bypasses its chat's queue. Both of them
// are interruptions: waiting behind the work they are meant to stop would mean
// arriving only after it finished.
func (j *job) skipsQueue() bool {
	return j.kind == kindDeletar || j.kind == kindRevoke
}

// runWorker executes one job at a time, in the order intake admitted them.
func (q *chatQueue) runWorker() {
	for j := range q.ready {
		<-j.acked
		q.bot.executeJob(j)
	}
}
