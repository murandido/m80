package account

import (
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Passenger is one person on a booking, as the caller knows them. CPF is nil far more often
// than not - only a Traveos sync carries documents.
type Passenger struct {
	Name string
	CPF  *string
}

// ReplaceIssuanceUses makes passengers the complete, current answer to "who did this
// issuance consume", replacing whatever it had consumed before.
//
// Replace rather than append, because both callers are corrections of an earlier guess:
//
//   - the Traveos sync learns the passengers' full names, while the issuance was born
//     holding only main_passenger_name - which on the bot path is the *surname* the airline
//     search used. Appending would leave "MACHADO" and "JOSE MACHADO" as two people.
//   - the panel's PATCH can change the passenger name, or move the issuance to another
//     account entirely, and the slot has to follow it.
//
// Rewriting the uses does not give a slot back: the replacements carry the issuance's own
// date and land in the same transaction. A *cancelled* issuance keeps its uses precisely
// because nothing rewrites them - which is the real rule, since the programs charge the slot
// at issue time and do not refund it.
func ReplaceIssuanceUses(
	tx *gorm.DB,
	accountID string,
	issuanceID string,
	passengers []Passenger,
	at time.Time,
) error {
	previous, err := clearIssuanceUses(tx, issuanceID)
	if err != nil {
		return err
	}

	for _, passenger := range passengers {
		if err := Register(tx, accountID, issuanceID, passenger.Name, passenger.CPF, at); err != nil {
			return err
		}
	}

	return dropOrphanedBeneficiaries(tx, previous)
}

// clearIssuanceUses removes this issuance's slot consumption and reports which beneficiaries
// it had been charged to, so the ones left with nothing can be cleaned up afterwards.
func clearIssuanceUses(tx *gorm.DB, issuanceID string) ([]string, error) {
	var beneficiaryIDs []string
	if err := tx.Model(&data.AccountBeneficiaryUse{}).
		Where("issuance_id = ?", issuanceID).
		Distinct().
		Pluck("beneficiary_id", &beneficiaryIDs).Error; err != nil {
		return nil, fmt.Errorf("account.ReplaceIssuanceUses: reading the previous uses: %w", err)
	}

	if err := tx.Where("issuance_id = ?", issuanceID).
		Delete(&data.AccountBeneficiaryUse{}).Error; err != nil {
		return nil, fmt.Errorf("account.ReplaceIssuanceUses: clearing the previous uses: %w", err)
	}
	return beneficiaryIDs, nil
}

// dropOrphanedBeneficiaries deletes the ones left with nothing to their name after the
// rewrite - in practice the surname-only placeholder the bot path creates, or a passenger
// name the operator has just corrected. Restricted to the candidates whose use was removed,
// so a beneficiary an operator added by hand and has not issued for yet survives.
func dropOrphanedBeneficiaries(tx *gorm.DB, candidates []string) error {
	if len(candidates) == 0 {
		return nil
	}
	err := tx.Where("id IN ?", candidates).
		Where("NOT EXISTS (SELECT 1 FROM account_beneficiary_uses" +
			" WHERE account_beneficiary_uses.beneficiary_id = account_beneficiaries.id)").
		Delete(&data.AccountBeneficiary{}).Error
	if err != nil {
		return fmt.Errorf("account.ReplaceIssuanceUses: dropping orphaned beneficiaries: %w", err)
	}
	return nil
}
