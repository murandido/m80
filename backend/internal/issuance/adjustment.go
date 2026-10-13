package issuance

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// auditActionUpdated is the enum value an alteration is logged under. audit_action is a
// Postgres ENUM, and adding a value to it is a migration whose down cannot undo it, so the
// specific action goes in details.reason - the same choice the account merge made.
const auditActionUpdated = "updated"

// AdjustmentInput is one alteration to a ticket after it was issued: what the airline
// charged or returned, and how much of it the client pays or gets back. See
// data.IssuanceAdjustment.
type AdjustmentInput struct {
	OccurredOn          time.Time
	Points              int
	Taxes               int
	ValueOfTheThousand  int
	TaxesPassedToClient bool
	Reason              string
}

// CreateAdjustment records an alteration on the issuance. Like Cancel, it does not open a
// transaction of its own: the caller wraps the lookup and this call in one.
//
// The closed-period guard is the caller's, as it is for a cancellation's date: the rule
// lives with the HTTP handler's other period checks, and only the panel records these.
func CreateAdjustment(db *gorm.DB, issuance data.Issuance, in AdjustmentInput, actorID string) (data.IssuanceAdjustment, error) {
	if err := checkAdjustment(issuance, in); err != nil {
		return data.IssuanceAdjustment{}, err
	}

	adjustment := data.IssuanceAdjustment{IssuanceID: issuance.ID, CreatedByID: actorID}
	applyAdjustmentInput(&adjustment, in)
	if err := db.Create(&adjustment).Error; err != nil {
		return data.IssuanceAdjustment{}, err
	}

	if err := afterAdjustment(db, issuance.ID, actorID, "adjustment_created", nil, &adjustment); err != nil {
		return data.IssuanceAdjustment{}, err
	}
	return adjustment, nil
}

// UpdateAdjustment rewrites an alteration in place. It is a correction of what was typed,
// so it replaces every field rather than recording a second movement.
func UpdateAdjustment(db *gorm.DB, issuance data.Issuance, adjustment data.IssuanceAdjustment, in AdjustmentInput, actorID string) (data.IssuanceAdjustment, error) {
	if err := checkAdjustment(issuance, in); err != nil {
		return data.IssuanceAdjustment{}, err
	}

	before := adjustment
	applyAdjustmentInput(&adjustment, in)
	if err := db.Save(&adjustment).Error; err != nil {
		return data.IssuanceAdjustment{}, err
	}

	if err := afterAdjustment(db, issuance.ID, actorID, "adjustment_updated", &before, &adjustment); err != nil {
		return data.IssuanceAdjustment{}, err
	}
	return adjustment, nil
}

// DeleteAdjustment removes an alteration that should never have been recorded. Soft, so the
// audit entry still points at something.
func DeleteAdjustment(db *gorm.DB, issuance data.Issuance, adjustment data.IssuanceAdjustment, actorID string) error {
	if !issuance.Active {
		return ErrAdjustmentOnCancelledIssuance
	}
	if err := db.Delete(&adjustment).Error; err != nil {
		return err
	}
	return afterAdjustment(db, issuance.ID, actorID, "adjustment_deleted", &adjustment, nil)
}

// checkAdjustment holds the rules every write shares.
//
// A cancelled issuance is closed to alterations: its refund was worked out against the
// ticket as it stood, and changing that ticket underneath would make the refund wrong
// without anybody noticing. Reverting the cancellation first is the way through.
func checkAdjustment(issuance data.Issuance, in AdjustmentInput) error {
	if !issuance.Active || issuance.CancellationOnly {
		return ErrAdjustmentOnCancelledIssuance
	}
	if in.Points == 0 && in.Taxes == 0 {
		return ErrAdjustmentEmpty
	}
	if strings.TrimSpace(in.Reason) == "" {
		return ErrAdjustmentReasonRequired
	}
	if in.ValueOfTheThousand < 0 {
		return ErrAdjustmentNegativeRate
	}
	return nil
}

func applyAdjustmentInput(adjustment *data.IssuanceAdjustment, in AdjustmentInput) {
	adjustment.OccurredOn = in.OccurredOn
	adjustment.Points = in.Points
	adjustment.Taxes = in.Taxes
	adjustment.ValueOfTheThousand = in.ValueOfTheThousand
	adjustment.TaxesPassedToClient = in.TaxesPassedToClient
	adjustment.Reason = strings.TrimSpace(in.Reason)
}

// afterAdjustment is what every alteration write is followed by. The sale is worth
// something different now, and the change is logged. Unlike the grid's autosave, an
// alteration is one deliberate action, so it earns its own line in the history.
func afterAdjustment(db *gorm.DB, issuanceID, actorID, reason string, before, after *data.IssuanceAdjustment) error {
	if err := sale.RecalculateByIssuance(db, issuanceID); err != nil {
		return err
	}

	details, err := json.Marshal(map[string]any{
		"before": adjustmentAuditView(before),
		"after":  adjustmentAuditView(after),
		"reason": reason,
	})
	if err != nil {
		return err
	}

	return db.Create(&data.AuditLog{
		IssuanceID:  issuanceID,
		ChangedByID: actorID,
		Action:      auditActionUpdated,
		Details:     datatypes.JSON(details),
	}).Error
}

func adjustmentAuditView(adjustment *data.IssuanceAdjustment) map[string]any {
	if adjustment == nil {
		return nil
	}
	return map[string]any{
		"adjustment_id":          adjustment.ID,
		"occurred_on":            adjustment.OccurredOn.Format(time.DateOnly),
		"points":                 adjustment.Points,
		"taxes":                  adjustment.Taxes,
		"value_of_the_thousand":  adjustment.ValueOfTheThousand,
		"taxes_passed_to_client": adjustment.TaxesPassedToClient,
		"reason":                 adjustment.Reason,
	}
}
