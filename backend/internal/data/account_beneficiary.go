package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AccountBeneficiary is one person an account has issued for. It belongs to the account,
// not to Person: the loyalty programs cap per account, and most of what we ever learn about
// a passenger - a surname typed into a WhatsApp message, a name imported from a spreadsheet -
// never becomes a registered person.
//
// The CPF is never stored. CPFFingerprint is an HMAC of it (see utils.Fingerprint), which is
// all the matching needs and is useless to whoever gets hold of a database dump.
type AccountBeneficiary struct {
	ID        string   `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID string   `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`
	// Nil until an issuance for this person arrives with a document - Traveos sync, or an
	// operator typing it in. Until then NormalizedName is the only match key there is.
	CPFFingerprint *string `gorm:"column:cpf_fingerprint" json:"-"`
	// What the panel shows, e.g. "***.456.789-**". Enough to tell two beneficiaries apart,
	// not enough to reconstruct the number.
	CPFMasked *string `gorm:"column:cpf_masked" json:"cpf_masked"`
	Name      string  `gorm:"not null" json:"name"`
	// Accent-free, uppercased, single-spaced - see utils.NormalizeName.
	NormalizedName string `gorm:"column:normalized_name;not null" json:"normalized_name"`
	// None of the programs counts the account holder against the cap.
	IsHolder  bool                    `gorm:"column:is_holder;not null;default:false" json:"is_holder"`
	Uses      []AccountBeneficiaryUse `gorm:"foreignKey:BeneficiaryID;references:ID" json:"uses,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

func (b *AccountBeneficiary) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

// Where a use came from.
const (
	BeneficiaryUseSourceIssuance = "issuance"
	BeneficiaryUseSourceManual   = "manual"
)

// AccountBeneficiaryUse is one issuance that consumed a beneficiary. The consumption a
// program cares about is a window over these rows, never a stored counter: that is what
// makes the Smiles number fall to zero on its own on 1 January, the LATAM one slide day by
// day, and "how many in 2026" still answerable in 2027.
//
// It also answers both counting modes from the same rows - COUNT(DISTINCT beneficiary_id)
// for a program that counts people, COUNT(*) for one that counts issuances.
type AccountBeneficiaryUse struct {
	ID            string              `gorm:"primaryKey;type:uuid" json:"id"`
	BeneficiaryID string              `gorm:"column:beneficiary_id;type:uuid;not null" json:"beneficiary_id"`
	Beneficiary   *AccountBeneficiary `gorm:"foreignKey:BeneficiaryID;references:ID" json:"-"`
	// Nil once the issuance is gone: the FK is ON DELETE SET NULL, because deleting an
	// issuance does not give the slot back any more than cancelling it does.
	IssuanceID *string   `gorm:"column:issuance_id;type:uuid" json:"issuance_id"`
	UsedAt     time.Time `gorm:"column:used_at;not null" json:"used_at"`
	Source     string    `gorm:"not null;default:'issuance'" json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

func (u *AccountBeneficiaryUse) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}
