package issuance

import (
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

// auditActionDeleted mirrors the value the panel's own handler has always
// written - see cancel.go's auditActionCancelled for why it lives here now.
const auditActionDeleted = "deleted"

// Delete soft-deletes an issuance and hard-deletes its linked sale - the rule
// shared by the panel's own DELETE /issuances/{id} and the bot's DELETAR
// message. Same reasoning and same transaction contract as Cancel: db may
// already be a transaction in progress, and the caller is responsible for
// having looked the issuance up first.
//
// Deleting an issuance undoes a mistake - the emission never should have been
// recorded - so there is nothing left to charge anyone for. The sale row goes
// with it, unlike a cancellation, where the sale survives (reopened) because
// the trip really happened and the money has to be given back.
func Delete(db *gorm.DB, issuance data.Issuance, actorID string) error {
	if err := db.Delete(&issuance).Error; err != nil {
		return err
	}

	if err := db.Create(&data.AuditLog{
		IssuanceID:  issuance.ID,
		ChangedByID: actorID,
		Action:      auditActionDeleted,
	}).Error; err != nil {
		return err
	}

	// Whose sale it was, read before it goes: the client's money that sat on it - a lump sum
	// spread there, or a surplus it was carrying - has to land on their other sales or their
	// credit, and the credit it was taking from others has to go back to them.
	var linked data.Sale
	if err := db.Select("id", "client_id").
		Where("issuance_id = ?", issuance.ID).
		Limit(1).
		Find(&linked).Error; err != nil {
		return err
	}
	// The sales its surplus was paying. Their credit lines go with the sale (ON DELETE
	// CASCADE), so the replay below no longer sees them to remove - and would not
	// recalculate sales.paid on those sales by itself.
	var covered []string
	if linked.ID != "" {
		if err := db.Model(&data.SalePayment{}).
			Distinct("sale_id").
			Where("credit_from_sale_id = ? AND sale_id <> ?", linked.ID, linked.ID).
			Pluck("sale_id", &covered).Error; err != nil {
			return err
		}
	}

	if err := db.Where("issuance_id = ?", issuance.ID).Delete(&data.Sale{}).Error; err != nil {
		return err
	}
	if linked.ClientID == nil {
		return nil
	}
	if _, err := sale.RebalanceClient(db, *linked.ClientID); err != nil {
		return err
	}
	for _, saleID := range covered {
		if err := sale.Recalculate(db, saleID); err != nil {
			return err
		}
	}
	return nil
}
