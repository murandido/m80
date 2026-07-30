package bot

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func (s correctionScenario) deletar() string {
	return "DELETAR - " + s.Locator
}

func assertDeleteRocket(t *testing.T, s correctionScenario) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
		t.Fatalf("reactions = %v, want a single 🚀", reactions)
	}
	if sends, _ := s.Client.calls(); sends != 1 {
		t.Fatalf("the bot sent %d message(s) on a successful deletion, want 1", sends)
	}
	if got := s.Deleted.Load(); got != 1 {
		t.Fatalf("notifyIssuanceDeleted called %d time(s), want 1", got)
	}
}

func assertDeleteRefused(t *testing.T, s correctionScenario, mentions string) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
		t.Fatalf("reactions = %v, want a single ❌", reactions)
	}
	if got := s.Deleted.Load(); got != 0 {
		t.Fatalf("notifyIssuanceDeleted called %d time(s) for a refused message, want 0", got)
	}
	if mentions != "" && !strings.Contains(strings.ToLower(s.Client.lastText()), strings.ToLower(mentions)) {
		t.Fatalf("the refusal %q does not mention %q", s.Client.lastText(), mentions)
	}
}

func TestHandleDeletar_SoftDeletesAndRemovesSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleDeletar(context.Background(), s.event(), s.deletar(), time.Now())

		assertDeleteRocket(t, s)

		var issuance data.Issuance
		err := tx.First(&issuance, "id = ?", s.Issuance.ID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected the default scope to hide the deleted issuance, got err=%v", err)
		}

		var sale data.Sale
		err = tx.First(&sale, "issuance_id = ?", s.Issuance.ID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected the sale to be deleted, got err=%v sale=%+v", err, sale)
		}

		var logs int64
		if err := tx.Model(&data.AuditLog{}).
			Where("issuance_id = ? AND action = 'deleted'", s.Issuance.ID).
			Count(&logs).Error; err != nil {
			t.Fatalf("failed to count audit logs: %v", err)
		}
		if logs != 1 {
			t.Fatalf("audit logs with action=deleted: got %d, want 1", logs)
		}
	})
}

func TestHandleDeletar_AlreadyDeletedIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleDeletar(context.Background(), s.event(), s.deletar(), time.Now())
		assertDeleteRocket(t, s)

		s.Client.reset()
		s.Deleted.Store(0)

		s.Bot.handleDeletar(context.Background(), s.event(), s.deletar(), time.Now())
		assertDeleteRefused(t, s, "não encontrei")
	})
}

// DELETAR only ever targets the active issuance for a locator - an old,
// already-cancelled row that happens to share the (now reusable) locator is
// not a candidate, so this must refuse exactly like "never existed".
func TestHandleDeletar_CancelledIssuanceIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleCancelamento(context.Background(), s.event(), s.cancelamento(), time.Now())
		s.Client.reset()
		s.Cancelled.Store(0)

		s.Bot.handleDeletar(context.Background(), s.event(), s.deletar(), time.Now())

		assertDeleteRefused(t, s, "não encontrei")
	})
}

func TestHandleDeletar_LocatorNotFoundIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleDeletar(context.Background(), s.event(), "DELETAR - NAOEXISTE", time.Now())

		assertDeleteRefused(t, s, "não encontrei")
	})
}

// The same silence every other trigger gets: someone typing in a group the
// bot merely happens to be in must not discover that it reads.
func TestHandleDeletar_IgnoresChatsThatWereNotApproved(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		if err := tx.Model(&data.ListenedChat{}).Where("chat_jid = ?", s.ChatJID).
			Update("active", false).Error; err != nil {
			t.Fatalf("failed to stop monitoring the chat: %v", err)
		}

		s.Bot.handleDeletar(context.Background(), s.event(), s.deletar(), time.Now())

		if sends, _ := s.Client.calls(); sends != 0 {
			t.Fatalf("the bot sent %d message(s), want none at all", sends)
		}
		if got := s.Deleted.Load(); got != 0 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 0", got)
		}

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", s.Issuance.ID).Error; err != nil {
			t.Fatalf("the bot deleted an issuance from a chat it may not act on: %v", err)
		}
	})
}

// State 1 of the three: the job is still queued, so DELETAR stops it before it
// writes anything. Nothing to soft-delete afterwards, and nothing to broadcast -
// the panel never saw an issuance appear.
func TestHandleDeletar_InterruptsAQueuedJob(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		locator := "PL" + s.Suffix

		queued := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent(s.text()),
			startedAt: time.Now(),
		})
		<-queued.acked
		if queued.kind != kindEmissao || queued.locator != locator {
			t.Fatalf("admitted job: kind=%v locator=%q, want an EMISSAO for %s", queued.kind, queued.locator, locator)
		}
		s.Client.reset()

		deletar := s.eventWithID("test-deletar-id")
		s.Bot.handleDeletar(context.Background(), deletar, "DELETAR - "+locator, time.Now())

		if !queued.isCancelled() {
			t.Fatal("the queued job should have been cancelled")
		}
		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
			t.Fatalf("reactions = %v, want a single 🚀", reactions)
		}
		if got := s.Deleted.Load(); got != 0 {
			t.Fatalf("notifyIssuanceDeleted called %d time(s), want 0 - nothing ever reached the database", got)
		}
		assertNoIssuance(t, tx, locator)

		// And the cancelled job stays cancelled: the worker refuses to start it,
		// and takes the 🕐 off the message it interrupted on the way out.
		s.Bot.executeJob(queued)
		assertNoIssuance(t, tx, locator)

		if reactions := s.Client.reactions(); !reflect.DeepEqual(reactions, []string{"🚀", "❌"}) {
			t.Fatalf("reactions = %v, want 🚀 on the DELETAR and ❌ closing the interrupted message", reactions)
		}
	})
}

// State 3, unchanged: a locator that only exists as a row is deleted from the
// row. Guards the fall-through - cancelling nothing must not be mistaken for
// "stopped it in time".
func TestHandleDeletar_UnknownLocatorIsStillRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleDeletar(context.Background(), s.event(), "DELETAR - NAOEXISTE", time.Now())

		assertDeleteRefused(t, s, "não encontrei")
	})
}

// DELETAR must not wait its turn. A message sitting behind the work it is meant
// to interrupt could only ever arrive after that work finished, by which point
// there is nothing left to interrupt - so it leaves the queue at intake and
// never shows a clock.
func TestChatQueue_DeletarSkipsTheQueue(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		j := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent("DELETAR - PL" + s.Suffix),
			startedAt: time.Now(),
		})
		<-j.acked

		if j.kind != kindDeletar {
			t.Fatalf("kind: got %v, want kindDeletar", j.kind)
		}
		if !j.skipsQueue() {
			t.Fatal("a DELETAR must skip its chat's queue")
		}
		if got := s.Client.reactions(); !reflect.DeepEqual(got, []string{"✅"}) {
			t.Fatalf("reactions: got %v, want [✅] only - a 🕐 would claim a queue it skipped", got)
		}
	})
}
