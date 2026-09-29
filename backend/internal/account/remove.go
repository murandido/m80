package account

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Deleting from the accounts screen used to mean archiving, always - and an archived account
// keeps naming its holder (accounts.holder_id is ON DELETE RESTRICT), so a holder the bot had
// created by mistake ("119,08") could never be deleted at all. The rule now is: whatever
// nothing refers to is deleted for real; whatever carries history is archived, and the
// panel stops showing it either way.

// accountHistory are the tables whose rows make an account worth keeping: issuances (in the
// trash too), point batches and closed inventory periods hold it with RESTRICT; statement
// lines and the beneficiary ledger would be wiped by the cascade, and they are history too.
var accountHistory = []string{
	"issuances",
	"point_batches",
	"inventory_periods",
	"azul_statement_lines",
	"smiles_statement_lines",
	"account_beneficiaries",
}

// accountHasHistory reports whether anything worth keeping refers to the account.
func accountHasHistory(tx *gorm.DB, accountID string) (bool, error) {
	for _, table := range accountHistory {
		var found bool
		// The table name is a constant from the list above; only the id is bound.
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM "+table+" WHERE account_id = ?)", accountID).
			Scan(&found).Error; err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// RemoveAccount deletes the account for real when nothing refers to it and archives it
// otherwise, and reports which. An account that is already archived and still has history is
// left as it is. Its aliases go either way - deleted with it, or archived with it so the bot
// stops routing to it and the names are free for another account.
func RemoveAccount(tx *gorm.DB, accountID string) (deleted bool, err error) {
	var account data.Account
	if err := tx.Unscoped().First(&account, "id = ?", accountID).Error; err != nil {
		return false, err
	}

	hasHistory, err := accountHasHistory(tx, accountID)
	if err != nil {
		return false, err
	}

	if !hasHistory {
		if err := tx.Unscoped().Where("account_id = ?", accountID).Delete(&data.AccountNickname{}).Error; err != nil {
			return false, err
		}
		return true, tx.Unscoped().Delete(&data.Account{}, "id = ?", accountID).Error
	}

	if account.DeletedAt.Valid {
		return false, nil
	}
	if err := tx.Model(&data.AccountNickname{}).Where("account_id = ?", accountID).
		UpdateColumn("deleted_at", time.Now()).Error; err != nil {
		return false, err
	}
	return false, tx.Delete(&data.Account{}, "id = ?", accountID).Error
}

// AdministersOthersError is a person who still administers somebody else's live accounts:
// removing them would leave those accounts pointing at a removed record, so the operator
// picks another administrator first.
type AdministersOthersError struct {
	Holders []string
}

func (e *AdministersOthersError) Error() string {
	return fmt.Sprintf("Esta pessoa ainda é administradora das contas de %s. Troque o administrador antes de excluir.",
		strings.Join(e.Holders, ", "))
}

// PersonRemoval is what removing a holder did, or would do.
type PersonRemoval struct {
	AccountsDeleted  int
	AccountsArchived int
	// PersonDeleted is true when the person went for real; false means archived.
	PersonDeleted bool
}

// administeredHolders names the holders whose live accounts the person administers.
func administeredHolders(tx *gorm.DB, personID string) ([]string, error) {
	var names []string
	err := tx.Model(&data.Account{}).
		Joins("JOIN people AS holders ON holders.id = accounts.holder_id").
		Where("accounts.administrator_id = ? AND accounts.holder_id <> ?", personID, personID).
		Distinct("holders.name").
		Pluck("holders.name", &names).Error
	return names, err
}

// RemovePerson removes a holder and every account they hold, account by account under
// RemoveAccount's rule. The person is deleted for real when no account (archived ones
// included) names them any more, and archived otherwise - with their aliases deleted, so the
// bot no longer resolves those names to them and is free to register the names again.
//
// With dryRun it only works out what would happen, for the panel's confirmation.
func RemovePerson(tx *gorm.DB, personID string, dryRun bool) (PersonRemoval, error) {
	var removal PersonRemoval

	if err := tx.First(&data.Person{}, "id = ?", personID).Error; err != nil {
		return removal, err
	}

	others, err := administeredHolders(tx, personID)
	if err != nil {
		return removal, err
	}
	if len(others) > 0 {
		return removal, &AdministersOthersError{Holders: others}
	}

	var accounts []data.Account
	if err := tx.Unscoped().Where("holder_id = ?", personID).Find(&accounts).Error; err != nil {
		return removal, err
	}

	remaining := 0
	for _, account := range accounts {
		if dryRun {
			hasHistory, err := accountHasHistory(tx, account.ID)
			if err != nil {
				return removal, err
			}
			if hasHistory {
				remaining++
				if !account.DeletedAt.Valid {
					removal.AccountsArchived++
				}
			} else {
				removal.AccountsDeleted++
			}
			continue
		}

		deleted, err := RemoveAccount(tx, account.ID)
		if err != nil {
			return removal, err
		}
		if deleted {
			removal.AccountsDeleted++
		} else {
			remaining++
			if !account.DeletedAt.Valid {
				removal.AccountsArchived++
			}
		}
	}

	// Archived accounts of other holders may still name the person as their administrator.
	var administered int64
	if err := tx.Unscoped().Model(&data.Account{}).
		Where("administrator_id = ? AND holder_id <> ?", personID, personID).
		Count(&administered).Error; err != nil {
		return removal, err
	}

	removal.PersonDeleted = remaining == 0 && administered == 0
	if dryRun {
		return removal, nil
	}

	if err := tx.Where("person_id = ?", personID).Delete(&data.PersonNickname{}).Error; err != nil {
		return removal, err
	}
	if removal.PersonDeleted {
		return removal, tx.Unscoped().Delete(&data.Person{}, "id = ?", personID).Error
	}
	return removal, tx.Delete(&data.Person{}, "id = ?", personID).Error
}

// IsRemovalRefusal reports whether err is a rule the operator can act on, as opposed to a
// failure.
func IsRemovalRefusal(err error) bool {
	var administers *AdministersOthersError
	return errors.As(err, &administers)
}
