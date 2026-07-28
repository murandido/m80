package bot

import (
	"errors"
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

		s.Bot.handleDeletar(s.event(), s.deletar(), time.Now())

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

		s.Bot.handleDeletar(s.event(), s.deletar(), time.Now())
		assertDeleteRocket(t, s)

		s.Client.reset()
		s.Deleted.Store(0)

		s.Bot.handleDeletar(s.event(), s.deletar(), time.Now())
		assertDeleteRefused(t, s, "não encontrei")
	})
}

// DELETAR only ever targets the active issuance for a locator - an old,
// already-cancelled row that happens to share the (now reusable) locator is
// not a candidate, so this must refuse exactly like "never existed".
func TestHandleDeletar_CancelledIssuanceIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleCancelamento(s.event(), s.cancelamento(), time.Now())
		s.Client.reset()
		s.Cancelled.Store(0)

		s.Bot.handleDeletar(s.event(), s.deletar(), time.Now())

		assertDeleteRefused(t, s, "não encontrei")
	})
}

func TestHandleDeletar_LocatorNotFoundIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleDeletar(s.event(), "DELETAR - NAOEXISTE", time.Now())

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

		s.Bot.handleDeletar(s.event(), s.deletar(), time.Now())

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
