package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// cancelamento builds a CANCELAMENTO message for this scenario's issuance,
// reusing correctionScenario - the same "issue a real EMISSAO first" fixture
// ALTERACAO's tests build on, since a cancellation starts from the same place.
func (s correctionScenario) cancelamento(refundLine ...string) string {
	return strings.Join(append([]string{"CANCELAMENTO - " + s.Locator}, refundLine...), "\n")
}

func assertCancelRocket(t *testing.T, s correctionScenario) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
		t.Fatalf("reactions = %v, want a single 🚀", reactions)
	}
	if sends, _ := s.Client.calls(); sends != 1 {
		t.Fatalf("the bot sent %d message(s) on a successful cancellation, want 1", sends)
	}
	if got := s.Cancelled.Load(); got != 1 {
		t.Fatalf("notifyIssuanceCancelled called %d time(s), want 1", got)
	}
}

func assertCancelRefused(t *testing.T, s correctionScenario, mentions string) {
	t.Helper()

	if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
		t.Fatalf("reactions = %v, want a single ❌", reactions)
	}
	if got := s.Cancelled.Load(); got != 0 {
		t.Fatalf("notifyIssuanceCancelled called %d time(s) for a refused message, want 0", got)
	}
	if mentions != "" && !strings.Contains(s.Client.lastText(), mentions) {
		t.Fatalf("the refusal %q does not mention %q", s.Client.lastText(), mentions)
	}
}

func assertCancelSilence(t *testing.T, s correctionScenario) {
	t.Helper()

	if sends, _ := s.Client.calls(); sends != 0 {
		t.Fatalf("the bot sent %d message(s), want none at all", sends)
	}
	if got := s.Cancelled.Load(); got != 0 {
		t.Fatalf("notifyIssuanceCancelled called %d time(s), want 0", got)
	}
}

func TestHandleCancelamento_FullRefundWhenNoLine2(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleCancelamento(s.event(), s.cancelamento(), time.Now())

		assertCancelRocket(t, s)

		cancelled := s.reload(t, tx)
		if cancelled.Active {
			t.Fatal("expected active = false")
		}
		if cancelled.CancelledAt == nil {
			t.Fatal("expected cancelled_at to be set")
		}
		if cancelled.RefundedPoints == nil || *cancelled.RefundedPoints != s.Issuance.Points {
			t.Fatalf("refunded_points = %v, want the full original %d", cancelled.RefundedPoints, s.Issuance.Points)
		}
		if cancelled.RefundedTaxes == nil || s.Issuance.Taxes == nil || *cancelled.RefundedTaxes != *s.Issuance.Taxes {
			t.Fatalf("refunded_taxes = %v, want the full original %v", cancelled.RefundedTaxes, s.Issuance.Taxes)
		}
		if cancelled.CancellationFee != nil {
			t.Fatalf("cancellation_fee = %v, want nil on a full refund", *cancelled.CancellationFee)
		}
	})
}

func TestHandleCancelamento_PartialRefund(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleCancelamento(s.event(), s.cancelamento("0k + 50 tx"), time.Now())

		assertCancelRocket(t, s)

		cancelled := s.reload(t, tx)
		if cancelled.RefundedPoints == nil || *cancelled.RefundedPoints != 0 {
			t.Fatalf("refunded_points = %v, want 0", cancelled.RefundedPoints)
		}
		if cancelled.RefundedTaxes == nil || *cancelled.RefundedTaxes != 5000 {
			t.Fatalf("refunded_taxes = %v, want 5000", cancelled.RefundedTaxes)
		}
		if cancelled.CancellationFee != nil {
			t.Fatalf("cancellation_fee = %v, want nil (omitted = 0)", *cancelled.CancellationFee)
		}
	})
}

func TestHandleCancelamento_ResetsPaidSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})
		if err := tx.Model(&data.Sale{}).Where("issuance_id = ?", s.Issuance.ID).
			Update("paid", true).Error; err != nil {
			t.Fatalf("failed to mark the sale paid: %v", err)
		}

		s.Bot.handleCancelamento(s.event(), s.cancelamento("30k + 50 multa"), time.Now())

		assertCancelRocket(t, s)

		var sale data.Sale
		if err := tx.First(&sale, "issuance_id = ?", s.Issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if sale.Paid {
			t.Fatal("expected sale.paid to be reset to false")
		}

		cancelled := s.reload(t, tx)
		if cancelled.CancellationFee == nil || *cancelled.CancellationFee != 5000 {
			t.Fatalf("cancellation_fee = %v, want 5000", cancelled.CancellationFee)
		}
	})
}

// findIssuanceToCancel only ever looks at active issuances - same scope as
// ALTERACAO and DELETAR - so a second CANCELAMENTO for the same locator reads
// back as "not found", not "already cancelled": the row is still there, but
// it is no longer a candidate. ErrIssuanceAlreadyCancelled (see cancel_test.go
// in internal/issuance) only fires on the panel's by-id lookup, which has no
// such filter.
func TestHandleCancelamento_SecondCancelOnSameLocatorIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleCancelamento(s.event(), s.cancelamento(), time.Now())
		assertCancelRocket(t, s)

		s.Client.reset()
		s.Cancelled.Store(0)

		s.Bot.handleCancelamento(s.event(), s.cancelamento(), time.Now())
		assertCancelRefused(t, s, "Não encontrei")
	})
}

func TestHandleCancelamento_LocatorNotFoundIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleCancelamento(s.event(), "CANCELAMENTO - NAOEXISTE", time.Now())

		assertCancelRefused(t, s, "Não encontrei")
	})
}

// The same silence an EMISSAO/ALTERACAO gets: someone typing in a group the
// bot merely happens to be in must not discover that it reads.
func TestHandleCancelamento_IgnoresChatsThatWereNotApproved(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		if err := tx.Model(&data.ListenedChat{}).Where("chat_jid = ?", s.ChatJID).
			Update("active", false).Error; err != nil {
			t.Fatalf("failed to stop monitoring the chat: %v", err)
		}

		s.Bot.handleCancelamento(s.event(), s.cancelamento(), time.Now())

		assertCancelSilence(t, s)
		if !s.reload(t, tx).Active {
			t.Fatal("the bot cancelled an issuance from a chat it may not act on")
		}
	})
}

func TestHandleCancelamento_Refusals(t *testing.T) {
	cases := []struct {
		name     string
		message  func(s correctionScenario) string
		mentions string
	}{
		{
			name:     "malformed header",
			message:  func(correctionScenario) string { return "CANCELAMENTO" },
			mentions: "linha 1 precisa ser",
		},
		{
			name:     "two locators",
			message:  func(correctionScenario) string { return "CANCELAMENTO - SMILES/IWBEQP" },
			mentions: "dois localizadores",
		},
		{
			name:     "too many lines",
			message:  func(s correctionScenario) string { return s.cancelamento("0k", "0k") },
			mentions: "no máximo 2 linhas",
		},
		{
			name:     "refund line without valid points",
			message:  func(s correctionScenario) string { return s.cancelamento("tx") },
			mentions: "número válido",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				s := newCorrectionScenario(t, tx, data.Airline{})

				s.Bot.handleCancelamento(s.event(), tt.message(s), time.Now())

				assertCancelRefused(t, s, tt.mentions)
				if !s.reload(t, tx).Active {
					t.Fatal("a refused cancellation still cancelled the issuance")
				}
			})
		})
	}
}
