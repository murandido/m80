package handlers

import (
	"encoding/json"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Folding one loyalty account into another: what the row merge (person_merge_row.go) does
// with each pair of accounts the two holders have in the same program. The issuances and the
// aliases move onto the surviving account, which empties the duplicate and lets it be removed
// for real. There is no standalone endpoint any more - a holder cannot have two live accounts
// in one program (migration 76), so the only duplicates left are across two holders.

// auditActionUpdated is what a merge writes on every issuance it moves. There is no
// "merged" action and adding one would mean ALTER TYPE on the audit_action enum, which
// Postgres cannot take back in a down migration; "updated" is also honest - a column on
// the issuance changed - and the panel's history already renders the before/after shape.
const auditActionUpdated = "updated"

// mergeAccountsTx folds drop into keep: aliases, issuances and the administrator keep is
// missing, and then deletes drop for real. The caller has already locked both rows and checked
// they belong to the same program (planRowAccounts pairs them by program).
func mergeAccountsTx(tx *gorm.DB, keep, drop data.Account, actorID string) error {
	if err := archiveCollidingNicknames(tx, keep.ID, drop.ID); err != nil {
		return err
	}

	// Unscoped: the aliases just archived above move too. They are what
	// issuances.account_nickname_id points at, so leaving them on a row that is about
	// to be deleted would take the record of which alias the EMISSAO came in by.
	if err := tx.Unscoped().Model(&data.AccountNickname{}).
		Where("account_id = ?", drop.ID).
		UpdateColumn("account_id", keep.ID).Error; err != nil {
		return err
	}

	if err := moveIssuances(tx, keep.ID, drop.ID, actorID); err != nil {
		return err
	}

	// UpdateColumns, not Save: Account.BeforeSave re-encrypts the password, and this
	// write has no business anywhere near it.
	// Only the administrator can be missing: every account has a holder since migration 75.
	updates := map[string]any{}
	if keep.AdministratorID == nil && drop.AdministratorID != nil {
		keep.AdministratorID = drop.AdministratorID
		updates["administrator_id"] = *drop.AdministratorID
	}
	if len(updates) > 0 {
		updates["complete"] = keep.ComputeComplete()
		if err := tx.Model(&data.Account{}).Where("id = ?", keep.ID).
			UpdateColumns(updates).Error; err != nil {
			return err
		}
	}

	// Unscoped: without it GORM would archive the duplicate, which is the state the
	// operator is trying to get out of. Nothing points at the row by now.
	return tx.Unscoped().Delete(&data.Account{}, "id = ?", drop.ID).Error
}

// collidingNicknames are the absorbed account's live aliases that the surviving one
// already answers to. Compared case-insensitively because that is how the bot matches
// them (internal/bot/emissao_resolve.go), which is stricter than the unique index and is
// what actually decides where an EMISSAO lands.
func collidingNicknames(db *gorm.DB, keepID, dropID string) ([]string, error) {
	var names []string
	err := db.Model(&data.AccountNickname{}).
		Where("account_id = ?", dropID).
		Where("LOWER(nickname) IN (?)", db.Model(&data.AccountNickname{}).
			Select("LOWER(nickname)").Where("account_id = ?", keepID)).
		Pluck("nickname", &names).Error
	if names == nil {
		names = []string{}
	}
	return names, err
}

// archiveCollidingNicknames retires the duplicated aliases before the rest are moved.
// Archiving rather than deleting: an issuance points at the alias row it came in by, and a
// delete would blank that (ON DELETE SET NULL). The uniqueness index only covers the
// living, so archiving is already enough to clear the way.
func archiveCollidingNicknames(tx *gorm.DB, keepID, dropID string) error {
	return tx.Model(&data.AccountNickname{}).
		Where("account_id = ?", dropID).
		Where("LOWER(nickname) IN (?)", tx.Model(&data.AccountNickname{}).
			Select("LOWER(nickname)").Where("account_id = ?", keepID)).
		UpdateColumn("deleted_at", time.Now()).Error
}

// moveIssuances repoints every issuance of the absorbed account, soft-deleted ones
// included - they hold account_id exactly like the live ones, which is the whole reason an
// account that ever issued cannot be deleted (see migration 29).
//
// Each one gets a history entry: the account on an issuance changed, and without a line
// saying why, whoever opens it later sees a field that moved on its own.
func moveIssuances(tx *gorm.DB, keepID, dropID, actorID string) error {
	var moved []string
	if err := tx.Unscoped().Model(&data.Issuance{}).
		Where("account_id = ?", dropID).
		Pluck("id", &moved).Error; err != nil {
		return err
	}
	if len(moved) == 0 {
		return nil
	}

	if err := tx.Unscoped().Model(&data.Issuance{}).
		Where("account_id = ?", dropID).
		UpdateColumn("account_id", keepID).Error; err != nil {
		return err
	}

	details, err := json.Marshal(map[string]any{
		"before": map[string]any{"account_id": dropID},
		"after":  map[string]any{"account_id": keepID},
		"reason": "account_merge",
	})
	if err != nil {
		return err
	}

	entries := make([]data.AuditLog, 0, len(moved))
	for _, issuanceID := range moved {
		entries = append(entries, data.AuditLog{
			IssuanceID:  issuanceID,
			ChangedByID: actorID,
			Action:      auditActionUpdated,
			Details:     datatypes.JSON(details),
		})
	}
	return tx.CreateInBatches(&entries, 100).Error
}
