package legacyimport

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RowSelection names one row of the sheet, by the block it sits in and its line number.
// The screen sends these and nothing else: the values themselves are re-read from the
// file, so a client cannot write a number the spreadsheet does not contain.
type RowSelection struct {
	BlockIndex int `json:"block_index"`
	Number     int `json:"number"`
}

// SkippedRow is one selected row that was not written, and why. Every skip is reported -
// an import that quietly does less than it was asked to is worse than one that refuses.
type SkippedRow struct {
	BlockIndex int    `json:"block_index"`
	Number     int    `json:"number"`
	Locator    string `json:"locator"`
	Reason     string `json:"reason"`
}

type CommitResult struct {
	Imported int          `json:"imported"`
	Skipped  []SkippedRow `json:"skipped"`
	// Cancelled counts the issuances an estorno line cancelled - ones that were already
	// in the database, whether the panel or an earlier row of this day put them there.
	Cancelled int `json:"cancelled"`
	// CreatedAndCancelled counts the estorno lines whose issuance had never been
	// imported: it is created and cancelled in one go, so the day's points still add up
	// even though the booking was never live in the panel.
	CreatedAndCancelled int `json:"created_and_cancelled"`
	// RegisteredAliases counts the account aliases the operator's mappings taught the
	// system. They make the next day of the same spreadsheet resolve on its own, and
	// the bot recognise the same word in an EMISSAO.
	RegisteredAliases int `json:"registered_aliases"`
}

// Commit writes the selected rows of one day.
//
// The whole day goes in a single transaction: a half-imported day is the state nobody can
// reason about, because the operator would have to work out by hand which rows made it.
// Rows the resolution refuses are skipped rather than fatal, so one unmappable line does
// not hold back the other thirty-four.
//
// The file is re-parsed and re-resolved by the caller before this runs. The preview the
// operator approved is advisory: between seeing it and pressing the button, somebody else
// may have registered one of these locators through the panel.
func Commit(
	db *gorm.DB,
	day Day,
	mappings Mappings,
	selected []RowSelection,
	actorID string,
) (CommitResult, error) {
	var result CommitResult

	err := db.Transaction(func(tx *gorm.DB) error {
		// Registering the aliases first means the resolution below already sees them,
		// so a row whose only problem was an unregistered alias resolves normally
		// instead of being carried by the mapping alone.
		registered, err := registerMappedAliases(tx, mappings)
		if err != nil {
			return err
		}

		preview, err := Resolve(tx, day, mappings)
		if err != nil {
			return err
		}

		wanted := map[RowSelection]bool{}
		for _, selection := range selected {
			wanted[selection] = true
		}

		agencies, err := resolveRowAgencies(tx, mappings)
		if err != nil {
			return err
		}

		var imported, cancelled, createdAndCancelled int
		var skipped []SkippedRow

		skip := func(row ResolvedRow, reason string) {
			skipped = append(skipped, SkippedRow{
				BlockIndex: row.BlockIndex,
				Number:     row.Number,
				Locator:    row.ProgramLocator,
				Reason:     reason,
			})
		}

		for _, block := range preview.Blocks {
			for _, row := range block.Rows {
				if !wanted[RowSelection{BlockIndex: row.BlockIndex, Number: row.Number}] {
					continue
				}
				if row.Verdict != VerdictOk && row.Verdict != VerdictCancellation {
					skip(row, row.Reason)
					continue
				}

				// The day's agency unless this row was pointed somewhere else.
				agencyID := preview.AgencyID
				if override, ok := agencies[RowSelection{BlockIndex: row.BlockIndex, Number: row.Number}]; ok {
					agencyID = override
				}

				if row.Verdict == VerdictCancellation {
					created, reason, err := commitCancellation(tx, day, agencyID, row, actorID)
					if err != nil {
						return fmt.Errorf("legacyimport: cancelling row %d: %w", row.Number, err)
					}
					switch {
					case reason != "":
						skip(row, reason)
					case created:
						createdAndCancelled++
					default:
						cancelled++
					}
					continue
				}

				if err := createIssuance(tx, day, agencyID, row, actorID, false); err != nil {
					// The locator being taken is the one failure that is somebody
					// else's normal work - a booking registered through the panel
					// between the preview and now - so it is a skip, not an abort.
					if isLocatorTaken(err) {
						skip(row, "Já existe uma emissão ativa com esse localizador")
						continue
					}
					return fmt.Errorf("legacyimport: importing row %d: %w", row.Number, err)
				}
				imported++
			}
		}

		result = CommitResult{
			Imported:            imported,
			Skipped:             skipped,
			Cancelled:           cancelled,
			CreatedAndCancelled: createdAndCancelled,
			RegisteredAliases:   registered,
		}
		return nil
	})

	return result, err
}

// commitCancellation leaves the issuance in the state the panel's own Cancelar button
// leaves it in, by calling the same rule - internal/issuance.Cancel - so the refund
// figures, the sale recalculation and the audit line are one implementation and not two.
//
// Which of the two paths it takes is decided here and not read off the preview: the
// operator may have ticked the estorno while unticking the issuance it belongs to, and a
// locator can be registered through the panel between the preview and the button.
//
// A non-empty reason means the row was not written and why; err is for the failures that
// have to take the whole day down with them.
func commitCancellation(
	tx *gorm.DB,
	day Day,
	agencyID string,
	row ResolvedRow,
	actorID string,
) (created bool, reason string, err error) {
	issuance, found, err := lockIssuanceByLocator(tx, row.ProgramLocator, true)
	if err != nil {
		return false, "", err
	}

	if !found {
		// Nothing active. If a cancelled one is already there, this day has been
		// imported before: cancelling frees the locator, so without this check a second
		// run would write a second cancelled issuance beside the first.
		if _, alreadyCancelled, err := lockIssuanceByLocator(tx, row.ProgramLocator, false); err != nil {
			return false, "", err
		} else if alreadyCancelled {
			return false, "A emissão desse localizador já está cancelada", nil
		}

		// cancellationOnly: this row is not a booking anybody made through the panel, it is
		// a peg for the estorno to hang off. Counting its points as miles issued would net
		// the day to zero, when the tab being reconciled reads minus the refund.
		if err := createIssuance(tx, day, agencyID, row, actorID, true); err != nil {
			if isLocatorTaken(err) {
				return false, "Já existe uma emissão ativa com esse localizador", nil
			}
			return false, "", err
		}
		issuance, found, err = lockIssuanceByLocator(tx, row.ProgramLocator, true)
		if err != nil {
			return false, "", err
		}
		if !found {
			return false, "", fmt.Errorf("the issuance just created for %q is gone", row.ProgramLocator)
		}
		created = true
	}

	plan := row.Cancellation
	// The sheet's day, not now(). The spreadsheet has no cancellation-date column at all -
	// an estorno is a negative row on the tab of the day it was written, so the tab's own
	// day IS the date. cancelled_at is what the supplier balance reads as the day the miles
	// came back, so an estorno from a March tab stamped with now() puts its refund in
	// whatever month somebody happened to click Importar, and leaves March showing the
	// booking with nothing to offset it.
	cancelledAt := middayOf(day.Date)

	input := issuancesvc.CancelInput{
		RefundedPoints: &plan.RefundedPoints,
		RefundedTaxes:  plan.RefundedTaxes,
		// The spreadsheet has no column for a cancellation fee, and nil says "nothing
		// on that front" rather than "zero".
		CancellationFee: nil,
		CancelledAt:     &cancelledAt,
	}
	if _, err := issuancesvc.Cancel(tx, issuance, input, actorID); err != nil {
		if errors.Is(err, issuancesvc.ErrIssuanceAlreadyCancelled) {
			return created, "A emissão desse localizador já está cancelada", nil
		}
		return created, "", err
	}

	return created, "", nil
}

// lockIssuanceByLocator loads the issuance a locator names, FOR UPDATE. The lock is what
// keeps a cancellation the panel is making at this exact moment from being lost: Cancel
// saves the whole row, so two cancellations racing on stale structs would have one
// overwrite the other's refund figures.
func lockIssuanceByLocator(tx *gorm.DB, locator string, active bool) (data.Issuance, bool, error) {
	var issuance data.Issuance
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("UPPER(program_locator) = ?", strings.ToUpper(strings.TrimSpace(locator))).
		Where("active = ?", active).
		Order("created_at DESC").
		First(&issuance).Error
	if isNotFound(err) {
		return data.Issuance{}, false, nil
	}
	if err != nil {
		return data.Issuance{}, false, fmt.Errorf("legacyimport: locking the issuance of %q: %w", locator, err)
	}
	return issuance, true, nil
}

func createIssuance(
	tx *gorm.DB,
	day Day,
	agencyID string,
	row ResolvedRow,
	actorID string,
	cancellationOnly bool,
) error {
	// The sheet's day, not now(). created_at is what every date filter and the grid
	// itself read as the day of the issuance.
	createdAt := middayOf(day.Date)

	input := issuancesvc.CreateInput{
		LoyaltyProgramID:    row.LoyaltyProgramID,
		ProgramLocator:      row.ProgramLocator,
		AirlineLocator:      optional(row.AirlineLocator),
		MainPassengerName:   optional(row.MainPassengerName),
		OriginIATACode:      optional(row.OriginIATACode),
		DestinationIATACode: optional(row.DestIATACode),
		Points:              row.Points,
		ValueOfTheThousand:  row.ValueOfTheThousand,
		Taxes:               row.Taxes,
		AccountID:           row.AccountID,
		AccountNicknameID:   row.AccountNicknameID,
		Observation:         optional(row.Observation),
		AgencyID:            &agencyID,
		IssuerID:            &row.IssuerID,
		FlightDate:          row.FlightDate,
		// Never complete: the spreadsheet has no passenger list and no itinerary, so
		// an imported issuance is exactly as unfinished as one the bot writes.
		Complete:         false,
		Imported:         true,
		CreatedAt:        &createdAt,
		CancellationOnly: cancellationOnly,
	}

	_, err := issuancesvc.Create(tx, input, actorID)
	return err
}

// resolveRowAgencies collects the per-row agency choices that name an agency that really
// exists. A row with no entry here takes the day's agency; an id that names nothing is
// left out rather than failing the import - the same laxity the account mappings get, and
// issuances.agency_id is a foreign key that would refuse it anyway.
func resolveRowAgencies(tx *gorm.DB, mappings Mappings) (map[RowSelection]string, error) {
	chosen := map[RowSelection]string{}
	if len(mappings.RowAgencies) == 0 {
		return chosen, nil
	}

	wanted := make([]string, 0, len(mappings.RowAgencies))
	for _, mapping := range mappings.RowAgencies {
		if mapping.AgencyID != "" {
			wanted = append(wanted, mapping.AgencyID)
		}
	}

	var existing []data.Agency
	if err := tx.Where("id IN ?", wanted).Find(&existing).Error; err != nil {
		return nil, fmt.Errorf("legacyimport: loading the chosen agencies: %w", err)
	}
	known := map[string]bool{}
	for _, agency := range existing {
		known[agency.ID] = true
	}

	for _, mapping := range mappings.RowAgencies {
		key := RowSelection{BlockIndex: mapping.BlockIndex, Number: mapping.Number}
		if known[mapping.AgencyID] {
			chosen[key] = mapping.AgencyID
		}
	}
	return chosen, nil
}

// registerMappedAliases turns each mapping the operator made into a real alias of the
// account they picked, unless the account already answers to that word. Without this the
// same alias is mapped again on every one of the spreadsheet's days; with it, the second
// day resolves on its own - and the bot learns the word too, which is what
// account_nicknames is for.
func registerMappedAliases(tx *gorm.DB, mappings Mappings) (int, error) {
	programs, err := loadPrograms(tx)
	if err != nil {
		return 0, err
	}

	registered := 0
	for _, mapping := range mappings.Accounts {
		label := strings.TrimSpace(mapping.AccountLabel)
		if label == "" || mapping.AccountID == "" {
			continue
		}
		program, ok := programs[programKey(mapping.ProgramCode)]
		if !ok {
			continue
		}

		// Compared without case or accents, and among living aliases only: writing
		// "WESLEY" next to an existing "Wesley" would pass the unique index (it is
		// over the exact string) and leave the account answering to the same word
		// twice.
		var existing int64
		err := tx.Model(&data.AccountNickname{}).
			Where("loyalty_program_id = ?", program.ID).
			Where("unaccent(nickname) ILIKE unaccent(?)", label).
			Count(&existing).Error
		if err != nil {
			return 0, fmt.Errorf("legacyimport: checking existing aliases: %w", err)
		}
		if existing > 0 {
			continue
		}

		alias := data.AccountNickname{
			AccountID:        mapping.AccountID,
			LoyaltyProgramID: program.ID,
			Nickname:         label,
		}
		if err := tx.Create(&alias).Error; err != nil {
			return 0, fmt.Errorf("legacyimport: registering the alias %q: %w", label, err)
		}
		registered++
	}

	return registered, nil
}

// listTimezone is the zone the listings resolve date_from/date_to in (see the handlers'
// parseDateRange). It has to be the same one here, or an imported issuance lands in the
// wrong bucket.
const listTimezone = "America/Sao_Paulo"

// middayOf turns a calendar day into an instant that reads as that day everywhere. The
// sheet's date arrives as midnight UTC, which in São Paulo is 21:00 of the *previous*
// day - so a row from 09/08 would be filed under 08/08 by every screen in the panel.
// Midday is the safe hour: it is the same calendar day in both zones, so even the
// tzdata-missing fallback below lands on the right one.
func middayOf(day time.Time) time.Time {
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc).UTC()
}

func isLocatorTaken(err error) bool {
	return errors.Is(err, issuancesvc.ErrProgramLocatorTaken)
}

func optional(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}
