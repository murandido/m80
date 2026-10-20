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

	// CancelledAt backdates the cancellation, mirroring CreateInput.CreatedAt.
	//
	// cancelled_at is what the costing, the supplier balance and the dashboard read as
	// "the day the miles came back", so a refund stamped with now() lands in whatever
	// month somebody happened to record it in. Two callers set it: the spreadsheet
	// importer (internal/legacyimport), which uses the day of the sheet tab the estorno
	// was written on, and the panel's Cancelar dialog, where the operator says the day
	// when the cancellation is being recorded after the fact.
	//
	// Nil is now(), which is the truth for a cancellation happening as it is recorded -
	// the bot's CANCELAMENTO, and the dialog left on today.
	CancelledAt *time.Time

	// CancellationOnly says this cancellation is an estorno for a booking the panel
	// never had: the row exists only to carry the refund, so the supplier balance must
	// count its refund and not its points - see data.Issuance.CancellationOnly.
	//
	// The spreadsheet importer sets it when it creates such a row
	// (internal/legacyimport), and the panel sets it when an operator registers the
	// booking by hand in order to estornar it. Without it the only way to make the day
	// read minus the refund is to type twice the points into the refund, which leaves
	// the statement claiming miles that were never issued here.
	CancellationOnly bool
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
	// Only ever turned on here: a plain cancellation of a booking this system really made
	// must leave the flag alone, and reverting a cancellation clears it (UncancelIssuance).
	columns := map[string]any{
		"active":           false,
		"cancelled_at":     issuance.CancelledAt,
		"cancelled_by_id":  issuance.CancelledByID,
		"refunded_points":  issuance.RefundedPoints,
		"refunded_taxes":   issuance.RefundedTaxes,
		"cancellation_fee": issuance.CancellationFee,
	}
	if in.CancellationOnly {
		issuance.CancellationOnly = true
		columns["cancellation_only"] = true
	}

	// A conditional write of the cancellation columns alone, never a Save of the whole
	// row: every caller looked the issuance up before this without a lock, so the struct
	// may already be stale. "active = TRUE" in the WHERE is the guard that actually holds -
	// two cancels racing each other both pass the check above, and Postgres re-evaluates
	// the condition for the one that waited on the row, which then matches nothing. Writing
	// only these columns also keeps a stale struct from reverting an edit committed in
	// between.
	result := db.Model(&data.Issuance{}).Where("id = ? AND active = TRUE", issuance.ID).Updates(columns)
	if result.Error != nil {
		return data.Issuance{}, result.Error
	}
	if result.RowsAffected == 0 {
		return data.Issuance{}, ErrIssuanceAlreadyCancelled
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
		"refunded_points":   in.RefundedPoints,
		"refunded_taxes":    in.RefundedTaxes,
		"cancellation_fee":  in.CancellationFee,
		"cancelled_at":      now,
		"cancellation_only": in.CancellationOnly,
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
