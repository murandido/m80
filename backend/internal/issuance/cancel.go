package issuance

import (
	"encoding/json"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// auditActionCancelled mirrors the value the panel's own handler has always
// written - see the package doc for why this lives here now.
const auditActionCancelled = "cancelled"

// CancelInput carries the refund amounts a cancellation records. All three are
// optional: a nil field means "nothing to refund on this front", not "unknown".
type CancelInput struct {
	RefundedPoints  *int
	RefundedTaxes   *int
	CancellationFee *int

	// CancelledAt backdates the cancellation, mirroring CreateInput.CreatedAt and
	// used by the same single caller: the spreadsheet importer
	// (internal/legacyimport), which sets it to the day of the sheet tab the estorno
	// was written on. That is the only cancellation date the sheet carries - there is
	// no such column, the tab's own day is the date - and cancelled_at is what the
	// supplier balance and the dashboard read as "the day the miles came back". An
	// estorno from a March tab stamped with now() lands its refund in whatever month
	// somebody happened to click Importar.
	//
	// Nil everywhere else. The panel's Cancelar button and the bot's CANCELAMENTO are
	// cancelling something as it happens, so for them now() is the truth.
	CancelledAt *time.Time
}

// Cancel marks an issuance cancelled - the rule shared by the panel's own
// POST /issuances/{id}/cancel and the bot's CANCELAMENTO message, extracted
// here per backend/CLAUDE.md's rule that an issuance action needed by both
// processes belongs in this package, not in either caller's handler.
//
// db may be a plain *gorm.DB or a transaction already in progress - unlike
// Create, this does not open its own nested transaction: the caller has
// already looked issuance up (by id in the panel, by locator in the bot) and
// is expected to wrap that lookup and this call in one transaction, the same
// way alteracao.go's applyCorrection does for a correction.
func Cancel(db *gorm.DB, issuance data.Issuance, in CancelInput, actorID string) (data.Issuance, error) {
	if !issuance.Active {
		return data.Issuance{}, ErrIssuanceAlreadyCancelled
	}

	// One variable, deliberately: this timestamp is written both onto the row and into
	// the audit details below, and two separate reads of the clock would let a caller
	// backdate one and not the other.
	now := time.Now()
	if in.CancelledAt != nil {
		now = *in.CancelledAt
	}

	issuance.Active = false
	issuance.CancelledAt = &now
	// Denormalize the canceller onto the row so the list filter can surface it in the
	// canceller's flow (audit_logs also records it, but isn't joined by the list query).
	cancelledBy := actorID
	issuance.CancelledByID = &cancelledBy
	issuance.RefundedPoints = in.RefundedPoints
	issuance.RefundedTaxes = in.RefundedTaxes
	issuance.CancellationFee = in.CancellationFee

	if err := db.Save(&issuance).Error; err != nil {
		return data.Issuance{}, err
	}

	// The sale's ledger is left alone - the money the client handed over does not stop
	// existing because the booking was cancelled. What changes is what is owed: a cancelled
	// ticket is not a debt, so the balance stops being "the client owes the price" and
	// becomes "the agency owes back whatever came in". Recalculating is what moves
	// sales.paid to match; see internal/sale.BalanceCents.
	//
	// This used to flip paid to false instead, which made sense while paid was a switch an
	// operator set by hand.
	if err := sale.RecalculateByIssuance(db, issuance.ID); err != nil {
		return data.Issuance{}, err
	}

	details, err := json.Marshal(map[string]any{
		"refunded_points":  in.RefundedPoints,
		"refunded_taxes":   in.RefundedTaxes,
		"cancellation_fee": in.CancellationFee,
		"cancelled_at":     now,
	})
	if err != nil {
		return data.Issuance{}, err
	}

	if err := db.Create(&data.AuditLog{
		IssuanceID:  issuance.ID,
		ChangedByID: actorID,
		Action:      auditActionCancelled,
		Details:     datatypes.JSON(details),
	}).Error; err != nil {
		return data.Issuance{}, err
	}

	return issuance, nil
}
