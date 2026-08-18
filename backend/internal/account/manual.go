package account

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The operator-driven half of the ledger. The automatic half matches passengers by name
// whenever no document arrived, and on the bot path that name is a surname - so it both
// merges people who are not the same and splits people who are. These are the corrections.

var (
	// ErrInvalidCPF is a document that failed the check digits, or one of the repeated-digit
	// placeholders. Surfaced rather than silently ignored: an operator who typed a CPF is
	// asserting something, and quietly filing it under the name instead would be a lie.
	ErrInvalidCPF = errors.New("account: invalid CPF")
	// ErrBeneficiaryNotFound is a merge naming a row that is not on this account.
	ErrBeneficiaryNotFound = errors.New("account: beneficiary not found")
)

// RegisterManual records a person the operator knows was issued on this account even though
// the issuance is not in the system. It consumes a slot like any other use, which is the
// point of typing it in.
func RegisterManual(tx *gorm.DB, accountID, name string, cpf *string, usedAt time.Time) error {
	if err := rejectBadCPF(cpf); err != nil {
		return err
	}

	normalizedName := utils.NormalizeName(name)
	if normalizedName == "" {
		return fmt.Errorf("account.RegisterManual: %w", errors.New("a beneficiary needs a name"))
	}

	fingerprint, masked := fingerprintOf(cpf)
	beneficiary, err := resolve(tx, accountID, normalizedName, name, fingerprint, masked)
	if err != nil {
		return err
	}

	// No issuance to hang it on, so this use is not deduplicated: registering the same
	// person twice by hand records two uses, which is what per-issuance programs count and
	// what the operator asked for by doing it twice.
	use := data.AccountBeneficiaryUse{
		BeneficiaryID: beneficiary.ID,
		UsedAt:        usedAt,
		Source:        data.BeneficiaryUseSourceManual,
	}
	if err := tx.Create(&use).Error; err != nil {
		return fmt.Errorf("account.RegisterManual: recording the use: %w", err)
	}
	return nil
}

// Correct rewrites what is known about a beneficiary: a fixed spelling, a CPF the sync never
// brought, or the fact that this is the account's own holder and should not be counted.
//
// Attaching a CPF here is the interesting case, and the reason this is not a plain UPDATE:
// the document may reveal that this row and another one are the same person, and the unique
// index would refuse the write. Merging is the right answer, not an error.
func Correct(tx *gorm.DB, beneficiary *data.AccountBeneficiary, name, cpf *string, isHolder *bool) error {
	if err := rejectBadCPF(cpf); err != nil {
		return err
	}

	updates := map[string]any{}

	if name != nil {
		normalized := utils.NormalizeName(*name)
		if normalized == "" {
			return fmt.Errorf("account.Correct: %w", errors.New("a beneficiary needs a name"))
		}
		updates["name"] = *name
		updates["normalized_name"] = normalized
	}
	if isHolder != nil {
		updates["is_holder"] = *isHolder
	}

	if cpf != nil && *cpf != "" {
		fingerprint, masked := fingerprintOf(cpf)
		if fingerprint == nil {
			return ErrInvalidCPF
		}

		existing, err := findByFingerprint(tx, beneficiary.AccountID, *fingerprint)
		if err != nil {
			return err
		}
		if existing != nil && existing.ID != beneficiary.ID {
			// The document says these two rows were always one person. Fold this one into
			// the row that already owns the CPF and stop - the merge carries the uses.
			return Merge(tx, beneficiary.AccountID, existing.ID, beneficiary.ID)
		}
		updates["cpf_fingerprint"] = fingerprint
		updates["cpf_masked"] = masked
	}

	if len(updates) == 0 {
		return nil
	}
	if err := tx.Model(beneficiary).Updates(updates).Error; err != nil {
		return fmt.Errorf("account.Correct: updating the beneficiary: %w", err)
	}
	return nil
}

// Merge moves every use from source onto survivor and deletes source. Both must belong to
// the same account.
//
// Moving the uses rather than dropping them is what keeps the count honest: the issuances
// happened, the ledger was only wrong about who they were for.
func Merge(tx *gorm.DB, accountID, survivorID, sourceID string) error {
	var survivor, source data.AccountBeneficiary
	if err := tx.First(&survivor, "id = ? AND account_id = ?", survivorID, accountID).Error; err != nil {
		return ErrBeneficiaryNotFound
	}
	if err := tx.First(&source, "id = ? AND account_id = ?", sourceID, accountID).Error; err != nil {
		return ErrBeneficiaryNotFound
	}

	// Both rows may hold a use of the same issuance - the survivor came from the Traveos
	// passengers and the source from main_passenger_name, say. Moving those would violate
	// the (beneficiary_id, issuance_id) unique index, and they are the same slot anyway.
	err := tx.Exec(`
		DELETE FROM account_beneficiary_uses AS source_uses
		WHERE source_uses.beneficiary_id = ?
		  AND source_uses.issuance_id IS NOT NULL
		  AND EXISTS (
			SELECT 1 FROM account_beneficiary_uses AS survivor_uses
			WHERE survivor_uses.beneficiary_id = ?
			  AND survivor_uses.issuance_id = source_uses.issuance_id
		  )`, sourceID, survivorID).Error
	if err != nil {
		return fmt.Errorf("account.Merge: dropping the duplicated uses: %w", err)
	}

	if err := tx.Model(&data.AccountBeneficiaryUse{}).
		Where("beneficiary_id = ?", sourceID).
		Update("beneficiary_id", survivorID).Error; err != nil {
		return fmt.Errorf("account.Merge: moving the uses: %w", err)
	}

	// The survivor inherits a CPF it did not have: the two rows are one person, so a
	// document either of them carried describes both.
	if survivor.CPFFingerprint == nil && source.CPFFingerprint != nil {
		err := tx.Model(&survivor).Updates(map[string]any{
			"cpf_fingerprint": source.CPFFingerprint,
			"cpf_masked":      source.CPFMasked,
		}).Error
		if err != nil {
			return fmt.Errorf("account.Merge: carrying the CPF over: %w", err)
		}
	}

	if err := tx.Delete(&source).Error; err != nil {
		return fmt.Errorf("account.Merge: deleting the absorbed beneficiary: %w", err)
	}
	return nil
}

// Reconcile rebuilds an account's ledger from its issuances, discarding every manual
// correction. It is the escape hatch for a list that has drifted past fixing by hand.
//
// Admin-only at the route, and irreversible: a beneficiary someone typed in because the
// issuance was never recorded is gone afterwards, because nothing in the issuances says it
// should exist.
func Reconcile(tx *gorm.DB, accountID string) error {
	if err := tx.Where("account_id = ?", accountID).
		Delete(&data.AccountBeneficiary{}).Error; err != nil {
		return fmt.Errorf("account.Reconcile: clearing the ledger: %w", err)
	}

	// Unscoped: a soft-deleted issuance still consumed its slot, exactly as a cancelled one
	// does. This has to agree with the backfill in migration 37, which includes them too.
	//
	// Passengers are preloaded because they are the better source where they exist: an
	// issuance synced from Traveos knows the whole booking and carries documents, while
	// main_passenger_name is one name and on the bot path only a surname. Rebuilding from
	// the name alone would undercount a family of four as one person.
	var issuances []data.Issuance
	err := tx.Unscoped().
		Preload("Passengers").
		Where("account_id = ?", accountID).
		Order("created_at ASC").
		Find(&issuances).Error
	if err != nil {
		return fmt.Errorf("account.Reconcile: reading the issuances: %w", err)
	}

	for _, issuance := range issuances {
		for _, passenger := range passengersOf(issuance) {
			if err := Register(
				tx, accountID, issuance.ID, passenger.Name, passenger.CPF, issuance.CreatedAt,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// passengersOf reads whoever an issuance is known to have flown, preferring the synced
// passenger rows and falling back to the single name the grid or the bot recorded. Documents
// come back decrypted here - data.IssuancePassenger's AfterFind hook handles that.
func passengersOf(issuance data.Issuance) []Passenger {
	if len(issuance.Passengers) > 0 {
		passengers := make([]Passenger, 0, len(issuance.Passengers))
		for _, source := range issuance.Passengers {
			passengers = append(passengers, Passenger{
				Name: strings.TrimSpace(source.FirstName + " " + source.LastName),
				CPF:  source.Document,
			})
		}
		return passengers
	}

	if issuance.MainPassengerName != nil && strings.TrimSpace(*issuance.MainPassengerName) != "" {
		return []Passenger{{Name: *issuance.MainPassengerName}}
	}
	return nil
}

// rejectBadCPF turns an unusable document into an error, for the paths where a human typed
// it. Register does the opposite on purpose - there, degrading to the name beats refusing an
// issuance over a document the airline sent us.
func rejectBadCPF(cpf *string) error {
	if cpf == nil || *cpf == "" {
		return nil
	}
	if _, ok := utils.NormalizeCPF(*cpf); !ok {
		return ErrInvalidCPF
	}
	return nil
}
