package handlers

import (
	"errors"
	"slices"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// errUnknownStatementAccount is a grant naming an account that does not exist, was deleted or
// is not an Azul/Smiles account - the only programs whose statement the portal shows.
var errUnknownStatementAccount = errors.New("unknown statement account")

// errAccountsNeedStatementClient is account_ids sent for a role that is not statement_client.
// Staff reach accounts through their role; a grant on them would never be read.
var errAccountsNeedStatementClient = errors.New("account grants are only for statement clients")

// grantableStatementPrograms are the loyalty programs whose statement is synced, and so the only
// accounts worth granting to a statement client.
var grantableStatementPrograms = []string{"AZUL", "SMILES"}

// loadUserAccountIDs returns each user's granted accounts. Users without grants are absent.
func loadUserAccountIDs(db *gorm.DB, userIDs []string) (map[string][]string, error) {
	if len(userIDs) == 0 {
		return map[string][]string{}, nil
	}
	var grants []data.UserAccount
	if err := db.Where("user_id IN ?", userIDs).Order("created_at, account_id").Find(&grants).Error; err != nil {
		return nil, err
	}
	byUser := make(map[string][]string, len(userIDs))
	for _, grant := range grants {
		byUser[grant.UserID] = append(byUser[grant.UserID], grant.AccountID)
	}
	return byUser, nil
}

// replaceUserAccounts makes the user's account grants exactly accountIDs. Run it inside the
// same transaction as the user write, so a bad account id leaves the user untouched.
func replaceUserAccounts(tx *gorm.DB, userID string, accountIDs []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&data.UserAccount{}).Error; err != nil {
		return err
	}
	unique := slices.Compact(slices.Sorted(slices.Values(accountIDs)))
	if len(unique) == 0 {
		return nil
	}

	// Checked up front rather than left to the foreign key: the key would accept a deleted
	// account or a LATAM one, and neither has a statement to show.
	var found int64
	err := tx.Table("accounts").
		Joins("JOIN loyalty_programs ON loyalty_programs.id = accounts.loyalty_program_id").
		Where("accounts.id IN ? AND accounts.deleted_at IS NULL AND loyalty_programs.code IN ?", unique, grantableStatementPrograms).
		Count(&found).Error
	if err != nil {
		return err
	}
	if int(found) != len(unique) {
		return errUnknownStatementAccount
	}

	grants := make([]data.UserAccount, 0, len(unique))
	for _, accountID := range unique {
		grants = append(grants, data.UserAccount{UserID: userID, AccountID: accountID})
	}
	return tx.Create(&grants).Error
}

// applyRoleGrants keeps each grant table consistent with the user's final role: agencies
// belong to staff below dev, accounts to statement clients, and whatever the role does not
// use is cleared so it cannot come back into force on a later role change.
// agencyIDs/accountIDs nil means "leave as is" for the grants the role does use.
func applyRoleGrants(tx *gorm.DB, user data.User, agencyIDs, accountIDs *[]string) error {
	if accountIDs != nil && user.Role != data.RoleStatementClient {
		return errAccountsNeedStatementClient
	}

	switch user.Role {
	case data.RoleDev:
		if err := replaceUserAgencies(tx, user.ID, nil); err != nil {
			return err
		}
		return replaceUserAccounts(tx, user.ID, nil)
	case data.RoleStatementClient:
		if err := replaceUserAgencies(tx, user.ID, nil); err != nil {
			return err
		}
		if accountIDs != nil {
			return replaceUserAccounts(tx, user.ID, *accountIDs)
		}
		return nil
	default:
		if err := replaceUserAccounts(tx, user.ID, nil); err != nil {
			return err
		}
		if agencyIDs != nil {
			return replaceUserAgencies(tx, user.ID, *agencyIDs)
		}
		return nil
	}
}
