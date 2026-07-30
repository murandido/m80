package bot

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// newQueueBot builds a Bot with no database at all. Every test below drives
// "!ping", the one trigger that reaches a reply without touching a single
// table, so these exercise the queue itself rather than any handler.
func newQueueBot(client *fakeClientBase) *Bot {
	return &Bot{
		Logger: waLog.Noop,
		Status: NewStatusStore(func() {}),
		session: deviceSession{
			Client: client,
			GetJID: func() *types.JID { return nil },
		},
	}
}

// newChatMessage builds a message from a chat/id pair. The id is what makes two
// otherwise identical "!ping"s tellable apart: ReplyMessage quotes it, and the
// fake records the quoted id, so the order of replies is observable.
func newChatMessage(chatUser, id, text string) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   types.NewJID(chatUser, types.GroupServer),
				Sender: types.NewJID("5511999999999", types.DefaultUserServer),
			},
			ID: types.MessageID(id),
		},
		Message: &waE2E.Message{Conversation: proto.String(text)},
	}
}

func submitPings(b *Bot, chatUser string, ids ...string) {
	for _, id := range ids {
		b.queue().submit(queuedMessage{
			evt:       newChatMessage(chatUser, id, "!ping"),
			startedAt: time.Now(),
		})
	}
}

// stopAndWait drains the queues, failing the test rather than hanging forever if
// they don't.
func stopAndWait(t *testing.T, b *Bot) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		b.queue().Stop(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(35 * time.Second):
		t.Fatal("dispatcher.Stop did not return - a queue goroutine is stuck")
	}
	if ctx.Err() != nil {
		t.Fatal("dispatcher.Stop timed out with work still in flight")
	}
}

// This is the bug the queue exists to fix. Before it, every message got its own
// goroutine and slept a random 0.5-1.5s before doing anything, so three sent in
// a row could be handled in any order at all - which is how an ALTERACAO could
// reach the database ahead of the EMISSAO it corrects.
func TestChatQueue_RunsJobsInTheOrderTheyArrived(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	submitPings(b, "1111111111", "m1", "m2", "m3")
	stopAndWait(t, b)

	got := client.traceMatching("reply:")
	want := []string{"reply:m1", "reply:m2", "reply:m3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reply order: got %v, want %v", got, want)
	}
}

// One job at a time within a chat. cmdPing announces itself with a typing
// presence before pausing to "compose", so two jobs running concurrently would
// show both presences before either reply.
func TestChatQueue_RunsOneJobAtATimeWithinAChat(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	submitPings(b, "2222222222", "m1", "m2")
	stopAndWait(t, b)

	got := client.traceMatching("presence", "reply:")
	want := []string{"presence", "reply:m1", "presence", "reply:m2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trace: got %v, want %v - jobs of one chat overlapped", got, want)
	}
}

// Chats are independent: one slow group must never hold up another. The
// mechanism is one queue per chat JID, so that is what this asserts - plus that
// both chats actually got their reply.
func TestChatQueue_GivesEachChatItsOwnQueue(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	submitPings(b, "3333333333", "a1")
	submitPings(b, "4444444444", "b1")
	stopAndWait(t, b)

	b.queue().mu.Lock()
	chats := len(b.queue().chats)
	b.queue().mu.Unlock()

	if chats != 2 {
		t.Fatalf("chat queues: got %d, want 2 - two chats must not share a worker", chats)
	}

	replies := client.traceMatching("reply:")
	if len(replies) != 2 {
		t.Fatalf("replies: got %v, want one per chat", replies)
	}
}

// submit runs on whatsmeow's dispatch goroutine, so it must return promptly
// however far behind the queue is. Ordinary conversation is used here because it
// is admitted and then dropped as kindNone, which keeps the burst cheap while
// still filling intake past its capacity.
func TestDispatcher_SubmitNeverBlocks(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	const burst = intakeCapacity * 4

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < burst; i++ {
			b.queue().submit(queuedMessage{
				evt:       newChatMessage("5555555555", "chatter", "bom dia"),
				startedAt: time.Now(),
			})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("submit blocked while pushing %d messages - it runs on whatsmeow's dispatch goroutine and must not", burst)
	}

	stopAndWait(t, b)
}

// Shutdown finishes what it accepted. A job killed halfway would leave the
// operator with a message the bot reacted to and never acted on.
func TestDispatcher_StopDrainsAcceptedWork(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	submitPings(b, "6666666666", "m1", "m2")
	stopAndWait(t, b)

	if got := client.traceMatching("reply:"); len(got) != 2 {
		t.Fatalf("replies after Stop: got %v, want both m1 and m2", got)
	}
}

func TestDispatcher_StopIsIdempotentAndDropsLaterMessages(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	submitPings(b, "7777777777", "m1")
	stopAndWait(t, b)
	stopAndWait(t, b)

	// Sending on a closed channel panics; the point of queueFor returning nil
	// after Stop is that this is simply ignored.
	submitPings(b, "7777777777", "m2")

	if got := client.traceMatching("reply:"); !reflect.DeepEqual(got, []string{"reply:m1"}) {
		t.Fatalf("replies: got %v, want only reply:m1 - messages after Stop must be dropped", got)
	}
}

// The reaction protocol, end to end on a path with no database behind it: ✅
// the moment the bot decides it may act, 🕐 once the message is waiting its
// turn, whatever the handler answers, and 🚀 to close the indicator - a reaction
// is only ever replaced, never cleared, so something has to take the clock off.
func TestAdmit_AcknowledgesThenMarksQueued(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	submitPings(b, "8888888888", "m1")
	stopAndWait(t, b)

	got := client.traceMatching("react:", "reply:")
	want := []string{"react:✅:m1", "react:🕐:m1", "reply:m1", "react:🚀:m1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trace: got %v, want %v", got, want)
	}
}

// The handler's own answer must never overtake the acknowledgements, or the
// group is left looking at a 🕐 on a job that already finished. job.acked is
// what orders them - per message, not globally: three messages in flight
// acknowledge themselves in parallel, and only each one's own sequence has to
// hold.
func TestAdmit_HandlerAnswerNeverOvertakesTheAcknowledgements(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	ids := []string{"m1", "m2", "m3", "m4", "m5"}
	submitPings(b, "9999999999", ids...)
	stopAndWait(t, b)

	trace := client.traceMatching("react:", "reply:")
	for _, id := range ids {
		var own []string
		for _, entry := range trace {
			if strings.HasSuffix(entry, ":"+id) {
				own = append(own, entry)
			}
		}
		want := []string{"react:✅:" + id, "react:🕐:" + id, "reply:" + id, "react:🚀:" + id}
		if !reflect.DeepEqual(own, want) {
			t.Fatalf("trace for %s: got %v, want %v (full trace: %v)", id, own, want, trace)
		}
	}
}

// A job stopped by something that answers on a different message - a DELETAR
// naming the PNR - has to close its own indicator. A reaction is only ever
// replaced, never cleared, so without this the interrupted message keeps the 🕐
// intake put on it for as long as the chat exists.
func TestExecuteJob_AnInterruptedJobClosesItsIndicator(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	j := b.admit(queuedMessage{evt: newChatMessage("1414141414", "m1", "!ping"), startedAt: time.Now()})
	<-j.acked
	client.reset()

	if !j.cancel(true) {
		t.Fatal("a job that has not run yet must be cancellable")
	}
	b.executeJob(j)
	stopAndWait(t, b)

	if got := client.reactions(); !reflect.DeepEqual(got, []string{"❌"}) {
		t.Fatalf("reactions: got %v, want a single ❌ - the clock must not be left on screen", got)
	}
}

// The other cancellation is silent, and has to be: an edit of a running message
// cancels its job and is then processed in its place, acknowledging and
// answering on that same message all over again. An ❌ here would flash a
// refusal for work that is about to be redone.
func TestExecuteJob_ASupersededJobSaysNothing(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	j := b.admit(queuedMessage{evt: newChatMessage("1515151515", "m1", "!ping"), startedAt: time.Now()})
	<-j.acked
	client.reset()

	if !j.cancel(false) {
		t.Fatal("a job that has not run yet must be cancellable")
	}
	b.executeJob(j)
	stopAndWait(t, b)

	if got := client.reactions(); len(got) != 0 {
		t.Fatalf("reactions: got %v, want none - the replacement job answers on this message", got)
	}
}

// Nothing the bot will not answer earns a reaction. An unknown command used to
// be dropped by RouteCommand's default branch, which is too late now: intake
// would already have promised a reply with ✅ and 🕐 that nothing ever resolves.
func TestAdmit_UnknownCommandIsNeverAcknowledged(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	b.queue().submit(queuedMessage{
		evt:       newChatMessage("1010101010", "m1", "!naoexiste"),
		startedAt: time.Now(),
	})
	stopAndWait(t, b)

	if got := client.reactions(); len(got) != 0 {
		t.Fatalf("reactions: got %v, want none for an unknown command", got)
	}
}

func TestAdmit_OrdinaryConversationIsNeverAcknowledged(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	b.queue().submit(queuedMessage{
		evt:       newChatMessage("1212121212", "m1", "alguém viu o PNR do cliente?"),
		startedAt: time.Now(),
	})
	stopAndWait(t, b)

	if got := client.reactions(); len(got) != 0 {
		t.Fatalf("reactions: got %v, want none - small talk is not addressed at the bot", got)
	}
	if _, markReadCalls := client.calls(); markReadCalls != 1 {
		t.Fatalf("markReadCalls: got %d, want 1 - read receipts are not gated", markReadCalls)
	}
}

// A backlog message is marked read and nothing else: replying to a command sent
// hours ago while the bot was down is not useful, and a 🕐 on it even less so.
func TestAdmit_BacklogIsReadButNeverAcknowledged(t *testing.T) {
	client := &fakeClientBase{}
	b := newQueueBot(client)

	b.queue().submit(queuedMessage{
		evt:       newChatMessage("1313131313", "m1", "!ping"),
		isBacklog: true,
		startedAt: time.Now(),
	})
	stopAndWait(t, b)

	if got := client.reactions(); len(got) != 0 {
		t.Fatalf("reactions: got %v, want none during backlog redelivery", got)
	}
	if _, markReadCalls := client.calls(); markReadCalls != 1 {
		t.Fatalf("markReadCalls: got %d, want 1", markReadCalls)
	}
}
