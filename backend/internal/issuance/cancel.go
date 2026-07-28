package issuance

import (
	"encoding/json"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
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

	now := time.Now()
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

	// Only touch a paid sale: scoping on paid=true keeps this a no-op when there is
	// no sale linked or it was already unpaid. RowsAffected records whether we actually
	// flipped it, so an uncancel can restore the exact prior state on a revert.
	res := db.Model(&data.Sale{}).
		Where("issuance_id = ? AND paid = ?", issuance.ID, true).
		Update("paid", false)
	if res.Error != nil {
		return data.Issuance{}, res.Error
	}

	details, err := json.Marshal(map[string]any{
		"refunded_points":  in.RefundedPoints,
		"refunded_taxes":   in.RefundedTaxes,
		"cancellation_fee": in.CancellationFee,
		"cancelled_at":     now,
		"sale_paid_reset":  res.RowsAffected > 0,
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
