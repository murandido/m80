package statement

import (
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResolvedBooking is the real issuance a statement locator maps to, and whether that mapping
// was pinned by hand rather than found automatically.
type ResolvedBooking struct {
	Issuance data.Issuance
	// Manual is true when the mapping came from a saved AzulBookingLink (an operator override),
	// false when it was matched by locator on the fly.
	Manual bool
}

// ResolveBookingLinks maps each of the given statement locators to a real issuance on the
// account, following the same rule the batch reconciliation uses: match by locator and ignore
// the date. For each locator the resolution is, in order:
//
//  1. a saved manual pin (AzulBookingLink), if its issuance still exists;
//  2. otherwise the most recent issuance whose program_locator equals the locator on this
//     account (soft-deleted issuances are excluded by the default scope; a cancelled-but-kept
//     one still matches, because its points really did leave the account);
//  3. otherwise the locator is absent from the returned map - it is not linked.
//
// Blank and duplicate locators are ignored. The date is deliberately not part of the match:
// the statement's day and the issuance's created_at need not agree, exactly as with the lote.
func ResolveBookingLinks(db *gorm.DB, accountID string, locators []string) (map[string]ResolvedBooking, error) {
	wanted := dedupeNonBlank(locators)
	if len(wanted) == 0 {
		return map[string]ResolvedBooking{}, nil
	}

	// The manual pins for these locators, and the issuances they point at (loaded by id, since a
	// pin's whole purpose is to reach an issuance whose own program_locator differs).
	var links []data.AzulBookingLink
	if err := db.Where("account_id = ? AND locator IN ?", accountID, wanted).
		Find(&links).Error; err != nil {
		return nil, fmt.Errorf("statement.ResolveBookingLinks: loading manual links: %w", err)
	}
	pinnedByLocator := make(map[string]string, len(links))
	pinnedIDs := make([]string, 0, len(links))
	for _, link := range links {
		pinnedByLocator[link.Locator] = link.IssuanceID
		pinnedIDs = append(pinnedIDs, link.IssuanceID)
	}

	pinnedIssuances := make(map[string]data.Issuance)
	if len(pinnedIDs) > 0 {
		var rows []data.Issuance
		if err := db.Where("id IN ?", pinnedIDs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("statement.ResolveBookingLinks: loading pinned issuances: %w", err)
		}
		for _, row := range rows {
			pinnedIssuances[row.ID] = row
		}
	}

	// The automatic match: every issuance on the account whose locator is one we were asked
	// about, most recent first, so the first one seen per locator is the one to keep.
	var autoRows []data.Issuance
	if err := db.Where("account_id = ? AND program_locator IN ?", accountID, wanted).
		Order("created_at DESC").
		Find(&autoRows).Error; err != nil {
		return nil, fmt.Errorf("statement.ResolveBookingLinks: loading matching issuances: %w", err)
	}
	autoByLocator := make(map[string]data.Issuance, len(autoRows))
	for _, row := range autoRows {
		if _, seen := autoByLocator[row.ProgramLocator]; !seen {
			autoByLocator[row.ProgramLocator] = row
		}
	}

	resolved := make(map[string]ResolvedBooking, len(wanted))
	for _, locator := range wanted {
		if issuanceID, pinned := pinnedByLocator[locator]; pinned {
			if issuance, ok := pinnedIssuances[issuanceID]; ok {
				resolved[locator] = ResolvedBooking{Issuance: issuance, Manual: true}
				continue
			}
			// The pinned issuance is gone (soft-deleted); fall through to the auto match rather
			// than report a link to nothing.
		}
		if issuance, ok := autoByLocator[locator]; ok {
			resolved[locator] = ResolvedBooking{Issuance: issuance, Manual: false}
		}
	}
	return resolved, nil
}

// UpsertBookingLink pins a locator on an account to an issuance, replacing any existing pin for
// that (account, locator). createdBy records who made it, and may be nil.
func UpsertBookingLink(db *gorm.DB, accountID, locator, issuanceID string, createdBy *string) error {
	link := data.AzulBookingLink{
		AccountID:   accountID,
		Locator:     locator,
		IssuanceID:  issuanceID,
		CreatedByID: createdBy,
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}, {Name: "locator"}},
		DoUpdates: clause.Assignments(map[string]any{
			"issuance_id": issuanceID,
			"created_by":  createdBy,
			"updated_at":  gorm.Expr("now()"),
		}),
	}).Create(&link).Error
	if err != nil {
		return fmt.Errorf("statement.UpsertBookingLink: %w", err)
	}
	return nil
}

// DeleteBookingLink removes a manual pin, handing the locator back to the automatic match. It
// is a no-op when there is no pin.
func DeleteBookingLink(db *gorm.DB, accountID, locator string) error {
	err := db.Where("account_id = ? AND locator = ?", accountID, locator).
		Delete(&data.AzulBookingLink{}).Error
	if err != nil {
		return fmt.Errorf("statement.DeleteBookingLink: %w", err)
	}
	return nil
}

// dedupeNonBlank returns the distinct non-empty locators, preserving first-seen order.
func dedupeNonBlank(locators []string) []string {
	seen := make(map[string]struct{}, len(locators))
	out := make([]string, 0, len(locators))
	for _, locator := range locators {
		if locator == "" {
			continue
		}
		if _, ok := seen[locator]; ok {
			continue
		}
		seen[locator] = struct{}{}
		out = append(out, locator)
	}
	return out
}
