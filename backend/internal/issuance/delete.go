package issuance

import (
	"github.com/murandido/m-80-backend/internal/data"
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

	return db.Where("issuance_id = ?", issuance.ID).Delete(&data.Sale{}).Error
}
