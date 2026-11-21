package sale

import (
	"time"

	"github.com/murandido/m-80-backend/internal/businessday"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// NotePaidOutsideSystem marks the line a cancellation-only estorno writes on its sale. It is
// how UncancelIssuance finds that line again to take it back.
const NotePaidOutsideSystem = "Pago fora do sistema (emissão lançada só para o estorno)"

// RecordPaidOutsideSystem writes on the issuance's sale what the client paid for a booking
// the panel never had. An issuance registered only to carry an estorno is a ticket the
// client already paid for elsewhere; without this line the ledger knows nothing of that
// money, the cancelled sale is worth zero, and the client's credit silently disappears.
// Once the cancellation lands, the replay (RebalanceClient) reads the line as the sale's
// surplus and carries it to the client's open sales or to their credit, like any other
// cancelled ticket that had been paid.
//
// It has to run while the issuance is still live: on a cancelled sale AmountOf is the fee,
// not the price. Only the part the sale's own lines do not already cover is written, so
// running it twice (cancel, revert, cancel again) never pays the ticket twice.
func RecordPaidOutsideSystem(tx *gorm.DB, issuanceID string, issuedAt time.Time, actorID string) error {
	var found data.Sale
	if err := tx.Select("id").Where("issuance_id = ?", issuanceID).Limit(1).Find(&found).Error; err != nil || found.ID == "" {
		return err
	}

	amount, err := AmountOf(tx, found.ID)
	if err != nil {
		return err
	}
	var ownCents *int
	if err := tx.Model(&data.SalePayment{}).
		Where("sale_id = ? AND "+ownPaymentLine, found.ID).
		Select("SUM(amount_cents)").
		Scan(&ownCents).Error; err != nil {
		return err
	}
	missing := amount.Cents
	if ownCents != nil {
		missing -= *ownCents
	}
	if missing <= 0 {
		return nil
	}

	note := NotePaidOutsideSystem
	return tx.Create(&data.SalePayment{
		SaleID:      found.ID,
		AmountCents: missing,
		// The money changed hands when the booking was made, not when it was estornado.
		PaidOn:      businessday.Of(issuedAt),
		Note:        &note,
		CreatedByID: actorID,
	}).Error
}

// RemovePaidOutsideSystem takes back the line RecordPaidOutsideSystem wrote. Reverting the
// cancellation says the booking is live after all, the opposite of what cancellation_only
// claimed, so the payment that claim implied goes with it - the same way the flag itself is
// cleared. The caller recalculates afterwards.
func RemovePaidOutsideSystem(tx *gorm.DB, issuanceID string) error {
	return tx.Where("sale_id IN (SELECT id FROM sales WHERE issuance_id = ?) AND note = ? AND "+ownPaymentLine,
		issuanceID, NotePaidOutsideSystem).
		Delete(&data.SalePayment{}).Error
}
