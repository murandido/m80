package data

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// LoyaltyDocument is one CPF the agency enrols in promotions, scoped to a loyalty
// programme - the same person legitimately holds both an Azul and a Smiles account.
//
// The number is encrypted at rest and handed back in the clear on read, exactly like
// Person.CPF and Account.Password: the panel has to show it, but a database dump should
// not hand over the agency's document list.
type LoyaltyDocument struct {
	ID   string `gorm:"primaryKey;type:uuid" json:"id"`
	Name string `gorm:"not null" json:"name"`
	CPF  string `gorm:"column:cpf;not null" json:"cpf"`
	// CPFFingerprint is the blind index that carries uniqueness, for the reason spelled
	// out in migration 38: the GCM nonce is random, so ciphertext differs on every write
	// and a unique index over the encrypted column would never catch a duplicate. Never
	// leaves the server.
	CPFFingerprint   string          `gorm:"column:cpf_fingerprint;not null" json:"-"`
	LoyaltyProgramID string          `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram   *LoyaltyProgram `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	// Retired documents stay on the table: promo_enrollments references them, and the
	// history of what was enrolled has to survive the document leaving the rotation.
	// No `default` tag - see PromoRecipe.Active.
	Active    bool      `gorm:"not null" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ld *LoyaltyDocument) BeforeCreate(tx *gorm.DB) error {
	if ld.ID == "" {
		ld.ID = uuid.New().String()
	}
	return nil
}

// BeforeSave derives the fingerprint and then encrypts the number.
//
// Order matters: the fingerprint comes from the plaintext, so it has to be computed
// before the value is replaced by ciphertext. The "enc:" prefix marks a row that has
// already been through here, so saving twice does not double-encrypt.
func (ld *LoyaltyDocument) BeforeSave(tx *gorm.DB) error {
	if strings.HasPrefix(ld.CPF, "enc:") {
		return nil
	}

	// Lenient normalisation, matching Person.BeforeSave: the same document has to
	// produce the same fingerprint on both tables. Whether the number is a *real* CPF is
	// checked in the handler, where a bad one can be answered with a 400 instead of a
	// database error surfacing from a save hook.
	normalized, ok := utils.NormalizeCPFForIndex(ld.CPF)
	if !ok {
		return errors.New("loyalty document: cpf is empty")
	}
	fingerprint, err := utils.Fingerprint(normalized)
	if err != nil {
		return err
	}
	ld.CPFFingerprint = fingerprint

	encrypted, err := utils.Encrypt(ld.CPF)
	if err != nil {
		return err
	}
	ld.CPF = encrypted
	return nil
}

// AfterFind hands the number back in the clear. utils.Decrypt passes through anything
// without the "enc:" prefix, so a row written by hand still reads.
func (ld *LoyaltyDocument) AfterFind(tx *gorm.DB) error {
	decrypted, err := utils.Decrypt(ld.CPF)
	if err != nil {
		return err
	}
	ld.CPF = decrypted
	return nil
}

// DigitsOnlyCPF returns the number stripped of punctuation, which is what an enrolment
// form expects to be typed into it.
func (ld *LoyaltyDocument) DigitsOnlyCPF() string {
	if digits, ok := utils.NormalizeCPFForIndex(ld.CPF); ok {
		return digits
	}
	return ld.CPF
}
