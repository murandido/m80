package account

import (
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// A holder's accounts share one administrator, the same way they share one list of aliases
// (nicknames.go): the operators think of "Fulana's administrator", not of one per program, and
// the supplier payment report groups by administrator - a Smiles under Wilmara and an Azul
// under somebody else for the same holder is always a mistake someone has to find.
//
// accounts.administrator_id stays per account in the schema; what this file keeps is the
// invariant that every live account of a holder carries the same value.

// HolderAdministrator is the administrator the holder's live accounts agree on. conflict is
// true when two accounts name different people - the panel flags that row until an operator
// picks one. Accounts with no administrator do not count as disagreeing.
func HolderAdministrator(tx *gorm.DB, holderID string) (adminID *string, conflict bool, err error) {
	var distinct []string
	if err := tx.Model(&data.Account{}).
		Where("holder_id = ? AND administrator_id IS NOT NULL", holderID).
		Distinct("administrator_id").
		Pluck("administrator_id", &distinct).Error; err != nil {
		return nil, false, err
	}
	switch len(distinct) {
	case 0:
		return nil, false, nil
	case 1:
		return &distinct[0], false, nil
	default:
		return nil, true, nil
	}
}

// SetHolderAdministrator writes the administrator on every live account of the holder, and
// recomputes each one's completeness (an administrator is one of the fields it needs).
// UpdateColumns, not Save: Account.BeforeSave re-encrypts the password, and this write has no
// business near it.
func SetHolderAdministrator(tx *gorm.DB, holderID string, adminID *string) error {
	var accounts []data.Account
	if err := tx.Where("holder_id = ?", holderID).Find(&accounts).Error; err != nil {
		return err
	}
	for _, account := range accounts {
		account.AdministratorID = adminID
		if err := tx.Model(&data.Account{}).Where("id = ?", account.ID).UpdateColumns(map[string]any{
			"administrator_id": adminID,
			"complete":         account.ComputeComplete(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// AlignHolderAdministrator fills in the administrator on the holder's accounts that have none,
// when the others agree on one. It never picks between two different people - that is the
// conflict the panel shows - so it is safe to run after anything that brings accounts together.
func AlignHolderAdministrator(tx *gorm.DB, holderID string) error {
	adminID, conflict, err := HolderAdministrator(tx, holderID)
	if err != nil || conflict || adminID == nil {
		return err
	}
	return SetHolderAdministrator(tx, holderID, adminID)
}
