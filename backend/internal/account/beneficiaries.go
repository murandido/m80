// Package account holds the rules about what an account may do, as opposed to what an
// issuance is. It lives under internal/ rather than in a handler for the same reason
// internal/issuance does: the WhatsApp bot issues too, and it has no *http.Request.
//
// What it answers today is the beneficiary cap - how many people (or how many issuances,
// depending on the program) an account has consumed in the current window.
package account

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Register records that an account issued for someone, creating the beneficiary if this is
// the first time, and charges one use against the issuance.
//
// Resolution order, which is the whole design in three lines:
//
//  1. by CPF fingerprint, when a document came with the issuance;
//  2. by normalized name, when it did not - the common case, since only Traveos-synced
//     issuances carry documents at all;
//  3. otherwise a new beneficiary.
//
// It is idempotent per (beneficiary, issuance): Traveos sync deletes and reinserts an
// issuance's passengers on every refresh, so registering the same person twice must not
// consume the slot twice.
//
// Never fails the caller over a bad document: an unparseable CPF degrades to the name match,
// because refusing the issuance over it would be a far worse trade.
func Register(
	tx *gorm.DB,
	accountID string,
	issuanceID string,
	name string,
	cpf *string,
	at time.Time,
) error {
	normalizedName := utils.NormalizeName(name)
	if normalizedName == "" {
		// Nothing to identify a person by. The issuance still exists; it just does not
		// name anyone yet, which is normal for a row the operator is still filling in.
		return nil
	}

	fingerprint, masked := fingerprintOf(cpf)

	beneficiary, err := resolve(tx, accountID, normalizedName, name, fingerprint, masked)
	if err != nil {
		return err
	}

	return recordUse(tx, beneficiary.ID, issuanceID, at)
}

// fingerprintOf turns a raw document into what actually gets stored. Returns nils when there
// is no usable CPF, which is not an error - it is most of the traffic.
func fingerprintOf(cpf *string) (fingerprint *string, masked *string) {
	if cpf == nil {
		return nil, nil
	}
	normalized, ok := utils.NormalizeCPF(*cpf)
	if !ok {
		return nil, nil
	}
	digest, err := utils.Fingerprint(normalized)
	if err != nil {
		// Only reachable with CPF_INDEX_KEY missing or malformed. Loud, because from here
		// on every beneficiary is matched by name alone and the counts quietly drift.
		slog.Error("account: cannot fingerprint a CPF, falling back to the name match", "err", err)
		return nil, nil
	}
	maskedValue := utils.MaskCPF(normalized)
	return &digest, &maskedValue
}

func resolve(
	tx *gorm.DB,
	accountID string,
	normalizedName string,
	name string,
	fingerprint *string,
	masked *string,
) (data.AccountBeneficiary, error) {
	if fingerprint != nil {
		byCPF, err := findByFingerprint(tx, accountID, *fingerprint)
		if err != nil {
			return data.AccountBeneficiary{}, err
		}
		if byCPF != nil {
			return *byCPF, nil
		}
	}

	byName, err := findByName(tx, accountID, normalizedName)
	if err != nil {
		return data.AccountBeneficiary{}, err
	}

	switch {
	case byName == nil:
		return create(tx, accountID, name, normalizedName, fingerprint, masked)

	// The row was known by name only and the document has just arrived - the passenger was
	// created from a WhatsApp message and Traveos has now synced it. Filling it in here is
	// what stops the same person from existing twice on the account.
	case byName.CPFFingerprint == nil && fingerprint != nil:
		if err := tx.Model(byName).
			Updates(map[string]any{"cpf_fingerprint": fingerprint, "cpf_masked": masked}).
			Error; err != nil {
			return data.AccountBeneficiary{}, fmt.Errorf(
				"account.Register: attaching a CPF to a beneficiary: %w", err)
		}
		byName.CPFFingerprint = fingerprint
		byName.CPFMasked = masked
		return *byName, nil

	default:
		return *byName, nil
	}
}

func findByFingerprint(tx *gorm.DB, accountID, fingerprint string) (*data.AccountBeneficiary, error) {
	var found data.AccountBeneficiary
	err := tx.Where("account_id = ? AND cpf_fingerprint = ?", accountID, fingerprint).
		First(&found).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("account.Register: looking a beneficiary up by CPF: %w", err)
	}
	return &found, nil
}

// findByName only ever looks at rows that have no CPF yet. A beneficiary already identified
// by a document is not re-matched by name: two people can share a name, and the document is
// the stronger claim.
func findByName(tx *gorm.DB, accountID, normalizedName string) (*data.AccountBeneficiary, error) {
	var found data.AccountBeneficiary
	err := tx.Where("account_id = ? AND normalized_name = ? AND cpf_fingerprint IS NULL",
		accountID, normalizedName).First(&found).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("account.Register: looking a beneficiary up by name: %w", err)
	}
	return &found, nil
}

func create(
	tx *gorm.DB,
	accountID string,
	name string,
	normalizedName string,
	fingerprint *string,
	masked *string,
) (data.AccountBeneficiary, error) {
	isHolder, err := matchesHolder(tx, accountID, normalizedName, fingerprint)
	if err != nil {
		return data.AccountBeneficiary{}, err
	}

	beneficiary := data.AccountBeneficiary{
		AccountID:      accountID,
		Name:           name,
		NormalizedName: normalizedName,
		CPFFingerprint: fingerprint,
		CPFMasked:      masked,
		IsHolder:       isHolder,
	}
	if err := tx.Create(&beneficiary).Error; err != nil {
		return data.AccountBeneficiary{}, fmt.Errorf("account.Register: creating a beneficiary: %w", err)
	}
	return beneficiary, nil
}

// matchesHolder reports whether this is the account's own holder. None of the three programs
// counts the holder against the cap, so a holder who flies on their own account every week
// must not eat the account's slots.
func matchesHolder(tx *gorm.DB, accountID, normalizedName string, fingerprint *string) (bool, error) {
	var holder struct {
		Name string
		CPF  *string
	}
	err := tx.Table("accounts").
		Select("people.name AS name, people.cpf AS cpf").
		Joins("JOIN people ON people.id = accounts.holder_id").
		Where("accounts.id = ?", accountID).
		Scan(&holder).Error
	if err != nil {
		return false, fmt.Errorf("account.Register: looking up the account holder: %w", err)
	}
	if holder.Name == "" {
		// No holder registered on the account - nothing to be, and nothing to exclude.
		return false, nil
	}

	// people.cpf is still plain text, so the strong match is available today. When that
	// column is encrypted this branch stops matching and the name below carries it.
	if fingerprint != nil && holder.CPF != nil {
		if normalized, ok := utils.NormalizeCPF(*holder.CPF); ok {
			if digest, err := utils.Fingerprint(normalized); err == nil && digest == *fingerprint {
				return true, nil
			}
		}
	}

	return utils.NormalizeName(holder.Name) == normalizedName, nil
}

// recordUse charges one slot. The unique index on (beneficiary_id, issuance_id) plus DO
// NOTHING is what makes a repeated Traveos sync free.
func recordUse(tx *gorm.DB, beneficiaryID, issuanceID string, at time.Time) error {
	use := data.AccountBeneficiaryUse{
		BeneficiaryID: beneficiaryID,
		UsedAt:        at,
		Source:        data.BeneficiaryUseSourceIssuance,
	}
	if issuanceID != "" {
		use.IssuanceID = &issuanceID
	}

	err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "beneficiary_id"}, {Name: "issuance_id"}},
		DoNothing: true,
	}).Create(&use).Error
	if err != nil {
		return fmt.Errorf("account.Register: recording a beneficiary use: %w", err)
	}
	return nil
}
