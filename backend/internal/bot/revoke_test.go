package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// revokeOf builds the stanza WhatsApp sends when someone deletes a message for
// everyone: its own id, the "7" edit attribute, and a ProtocolMessage naming the
// message that disappeared. No text - that is the whole point.
func revokeOf(base *events.Message, targetID string, attribute types.EditAttribute) *events.Message {
	revoke := *base
	revoke.Info.ID = "revoke-stanza-id"
	revoke.Info.Edit = attribute
	revoke.Message = &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_REVOKE.Enum(),
			Key:  &waCommon.MessageKey{ID: proto.String(targetID)},
		},
	}
	return &revoke
}

func TestIsMessageRevoke(t *testing.T) {
	base := newChatMessage("1111111111", "m1", "")

	tests := []struct {
		name string
		evt  *events.Message
		want bool
	}{
		{
			name: "sender deleted their own message",
			evt:  revokeOf(base, "original-id", types.EditAttributeSenderRevoke),
			want: true,
		},
		{
			name: "a group admin deleted someone else's",
			evt:  revokeOf(base, "original-id", types.EditAttributeAdminRevoke),
			want: true,
		},
		{
			name: "history sync, which carries no edit attribute",
			evt:  revokeOf(base, "original-id", types.EditAttributeEmpty),
			want: true,
		},
		{
			name: "an edit is not a deletion",
			evt: func() *events.Message {
				edit := *base
				edit.Info.Edit = types.EditAttributeMessageEdit
				return &edit
			}(),
			want: false,
		},
		{
			name: "pinning is not a deletion either",
			evt: func() *events.Message {
				pin := *base
				pin.Info.Edit = types.EditAttributePinInChat
				return &pin
			}(),
			want: false,
		},
		{
			name: "ordinary conversation",
			evt:  newChatMessage("1111111111", "m1", "bom dia"),
			want: false,
		},
		{
			// A ProtocolMessage with no type at all reads back as REVOKE, the
			// enum's zero value, so the target key is what has to carry the
			// decision on the history-sync path.
			name: "an empty protocol message is not a deletion",
			evt: func() *events.Message {
				empty := *base
				empty.Message = &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}
				return &empty
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMessageRevoke(tt.evt); got != tt.want {
				t.Fatalf("isMessageRevoke = %v, want %v", got, tt.want)
			}
		})
	}
}

// The id comes from the ProtocolMessage key when it is there, and from the
// stanza id when it is not - a shape observed live from WhatsApp Web, where the
// deletion carried edit="7" and no readable ProtocolMessage at all.
func TestRevokedMessageID(t *testing.T) {
	base := newChatMessage("1111111111", "m1", "")

	withKey := revokeOf(base, "original-id", types.EditAttributeSenderRevoke)
	if got := revokedMessageID(withKey); got != "original-id" {
		t.Fatalf("revokedMessageID = %q, want the key's id", got)
	}
	if withKey.Info.ID == "original-id" {
		t.Fatal("the stanza's own id must not be mistaken for the key's")
	}

	// No key: the stanza addresses the target by its own id, the way an edit
	// stanza does. Wrong here can only mean "found nothing", never "deleted the
	// wrong one" - nothing else is filed under a stanza id.
	keyless := *base
	keyless.Info.ID = "addressed-by-stanza-id"
	keyless.Info.Edit = types.EditAttributeSenderRevoke
	keyless.Message = &waE2E.Message{}
	if got := revokedMessageID(&keyless); got != "addressed-by-stanza-id" {
		t.Fatalf("revokedMessageID = %q, want the stanza id as the fallback", got)
	}
}

// State 3 of the deletion table: the message had already become an issuance.
// Deleting the message deletes the issuance, exactly as DELETAR would - and
// says nothing, because the message it would react to is gone.
func TestHandleMessageRevoke_DeletesTheIssuanceTheMessageCreated(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		original := s.eventWithID("original-id")
		s.Bot.handleEmissao(context.Background(), original, s.text(), time.Now())
		s.Client.reset()
		s.Deleted.Store(0)

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("the EMISSAO should have created an issuance: %v", err)
		}

		s.Bot.handleMessageRevoke(context.Background(),
			revokeOf(original, "original-id", types.EditAttributeSenderRevoke), time.Now())

		err := tx.First(&data.Issuance{}, "id = ?", issuance.ID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected the issuance to be soft-deleted, got err=%v", err)
		}

		var sales int64
		if err := tx.Model(&data.Sale{}).Where("issuance_id = ?", issuance.ID).Count(&sales).Error; err != nil {
			t.Fatalf("failed to count sales: %v", err)
		}
		if sales != 0 {
			t.Fatalf("sales rows: got %d, want 0 - a deletion takes the sale with it", sales)
		}

		var deletedLogs int64
		if err := tx.Model(&data.AuditLog{}).
			Where("issuance_id = ? AND action = ?", issuance.ID, "deleted").
			Count(&deletedLogs).Error; err != nil {
			t.Fatalf("failed to count audit logs: %v", err)
		}
		if deletedLogs != 1 {
			t.Fatalf("audit logs with action=deleted: got %d, want 1", deletedLogs)
		}

		if got := s.Deleted.Load(); got != 1 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 1", got)
		}
		if got := s.Client.reactions(); len(got) != 0 {
			t.Fatalf("reactions: got %v, want none - there is no message left to react to", got)
		}
		if sends, _ := s.Client.calls(); sends != 0 {
			t.Fatalf("the bot sent %d message(s), want 0 on a deletion that worked", sends)
		}
	})
}

// State 1: the job had not run yet, so the deletion stops it and there is
// nothing in the database to undo afterwards.
func TestHandleMessageRevoke_CancelsAJobStillQueued(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		locator := "PL" + s.Suffix

		queued := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent(s.text()),
			startedAt: time.Now(),
		})
		<-queued.acked
		s.Client.reset()

		s.Bot.handleMessageRevoke(context.Background(),
			revokeOf(queued.evt, "test-emissao-id", types.EditAttributeSenderRevoke), time.Now())

		if !queued.isCancelled() {
			t.Fatal("the queued job should have been cancelled")
		}
		assertNoIssuance(t, tx, locator)
		if got := s.Deleted.Load(); got != 0 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 0 - nothing was ever saved", got)
		}

		// And the worker refuses to start it afterwards.
		s.Bot.executeJob(queued)
		assertNoIssuance(t, tx, locator)
	})
}

// State 2: the message is deleted while its job is already running, which has to
// end exactly like state 1 - the work stops and nothing is left behind. The
// difference is only in how: a queued job is told not to start, a running one has
// its context pulled, which aborts the statement it is inside and rolls the
// transaction back.
//
// The job is driven by hand rather than through executeJob because the real
// handler commits in milliseconds: racing it would test the scheduler, not this.
// What the deletion has to do is cancel, and then wait for the handler to be
// finished before reading the database - the rollback lands a moment after the
// statement is aborted, and a read taken too early sees neither outcome.
func TestHandleMessageRevoke_CancelsAJobAlreadyRunning(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		locator := "PL" + s.Suffix

		running := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent(s.text()),
			startedAt: time.Now(),
		})
		<-running.acked
		s.Client.reset()

		ctx, live := running.begin(context.Background())
		if !live {
			t.Fatal("the job should still be startable")
		}

		// Stands in for the handler: it is inside its transaction, and it returns
		// when the context is pulled - which is what rolling back looks like from
		// the outside.
		handlerReturned := make(chan struct{})
		go func() {
			defer close(handlerReturned)
			<-ctx.Done()
			running.finish()
		}()

		s.Bot.handleMessageRevoke(context.Background(),
			revokeOf(running.evt, "test-emissao-id", types.EditAttributeSenderRevoke), time.Now())

		select {
		case <-handlerReturned:
		case <-time.After(2 * time.Second):
			t.Fatal("handleMessageRevoke returned before the running job was cancelled and finished")
		}

		if ctx.Err() == nil {
			t.Fatal("the running job's context should have been cancelled")
		}
		assertNoIssuance(t, tx, locator)
		if got := s.Deleted.Load(); got != 0 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 0 - the work never committed", got)
		}
		if got := s.Client.reactions(); len(got) != 0 {
			t.Fatalf("reactions: got %v, want none - the message that would carry one is gone", got)
		}
	})
}

// A revoke carries no PNR, so a message the bot does not remember is a message
// it cannot connect to anything. Guessing would be worse than leaving it be.
func TestHandleMessageRevoke_UnknownMessageDoesNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.handleMessageRevoke(context.Background(),
			revokeOf(s.event(), "a-message-nobody-remembers", types.EditAttributeSenderRevoke), time.Now())

		if got := s.Deleted.Load(); got != 0 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 0", got)
		}
		if sends, _ := s.Client.calls(); sends != 0 {
			t.Fatalf("the bot sent %d message(s), want 0", sends)
		}
	})
}

// The gate applies here as everywhere else: a deletion in a chat the bot was
// never approved for is not the bot's business.
func TestHandleMessageRevoke_IgnoresUnapprovedChats(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		original := s.eventWithID("original-id")
		s.Bot.handleEmissao(context.Background(), original, s.text(), time.Now())
		s.Client.reset()
		s.Deleted.Store(0)

		revoke := revokeOf(original, "original-id", types.EditAttributeSenderRevoke)
		revoke.Info.Chat = types.NewJID("999000111", types.GroupServer)

		s.Bot.handleMessageRevoke(context.Background(), revoke, time.Now())

		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&data.Issuance{}).Error; err != nil {
			t.Fatalf("the issuance must survive a deletion from a chat the bot may not act on: %v", err)
		}
		if got := s.Deleted.Load(); got != 0 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 0", got)
		}
	})
}

// A deletion never enters the queue and never reacts, both for the same reason:
// there is no message left in the chat to wait for or to mark.
func TestAdmit_RevokeSkipsTheQueueAndSaysNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		j := s.Bot.admit(queuedMessage{
			evt:       revokeOf(s.event(), "original-id", types.EditAttributeSenderRevoke),
			startedAt: time.Now(),
		})
		<-j.acked

		if j.kind != kindRevoke {
			t.Fatalf("kind: got %v, want kindRevoke", j.kind)
		}
		if !j.skipsQueue() {
			t.Fatal("a deletion must skip the queue - it exists to interrupt what is in it")
		}
		if got := s.Client.reactions(); len(got) != 0 {
			t.Fatalf("reactions: got %v, want none", got)
		}
	})
}
