package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

type Person struct {
	ID   string `gorm:"primaryKey;type:uuid" json:"id"`
	Name string `gorm:"not null" json:"name"`
	// Encrypted at rest (AES-256-GCM) by the hooks below and decrypted on read, exactly like
	// Account.Password: the panel still shows the number, because staff need it to issue, but
	// a database dump no longer hands over the client registry.
	CPF *string `gorm:"column:cpf" json:"cpf"`
	// CPFFingerprint is the blind index that replaces the old unique index on cpf - see
	// migration 38. Never leaves the server: it is the lookup key for search and the
	// uniqueness key, and it is what matches a person against an account beneficiary.
	CPFFingerprint *string          `gorm:"column:cpf_fingerprint" json:"-"`
	BirthDate      *time.Time       `gorm:"type:date;column:birth_date" json:"birth_date"`
	Email          *string          `json:"email"`
	Phone          *string          `json:"phone"`
	ZipCode        *string          `gorm:"column:zip_code" json:"zip_code"`
	State          *string          `json:"state"`
	City           *string          `json:"city"`
	Address        *string          `json:"address"`
	AddressNumber  *string          `gorm:"column:address_number" json:"address_number"`
	Complement     *string          `json:"complement"`
	Neighborhood   *string          `json:"neighborhood"`
	Nicknames      []PersonNickname `gorm:"foreignKey:PersonID;references:ID" json:"nicknames,omitempty"`
	// ReportExportCounter is bumped every time the supplier payment PDF is exported for this
	// person as an administrator, and the number goes into the file name. Editable by hand from
	// the suppliers view, so a fresh clone or a corrected count can be set directly.
	ReportExportCounter int       `gorm:"column:report_export_counter;not null;default:0" json:"report_export_counter"`
	Complete            bool      `gorm:"not null;default:false" json:"complete"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// BeforeSave keeps the CPF and its fingerprint in step, and encrypts the number.
//
// Order matters: the fingerprint is derived from the plaintext, so it has to be computed
// before the value is replaced by ciphertext. A row saved twice is not re-encrypted - the
// "enc:" prefix is the marker, the same one Account.Password uses.
func (p *Person) BeforeSave(tx *gorm.DB) error {
	if p.CPF == nil || *p.CPF == "" {
		// Clearing the CPF has to clear the fingerprint too, or the person keeps holding a
		// document they no longer have - and keeps it reserved against everybody else.
		p.CPF = nil
		p.CPFFingerprint = nil
		return nil
	}

	if strings.HasPrefix(*p.CPF, "enc:") {
		// Already ciphertext: nothing to derive from, and nothing to encrypt again. Reached
		// when a caller saves a row it built by hand rather than read back.
		return nil
	}

	if normalized, ok := utils.NormalizeCPFForIndex(*p.CPF); ok {
		fingerprint, err := utils.Fingerprint(normalized)
		if err != nil {
			return err
		}
		p.CPFFingerprint = &fingerprint
	} else {
		p.CPFFingerprint = nil
	}

	encrypted, err := utils.Encrypt(*p.CPF)
	if err != nil {
		return err
	}
	p.CPF = &encrypted
	return nil
}

// AfterFind hands the CPF back in the clear. Values written before migration 38 have no
// prefix and pass through untouched, which is what makes the backfill something that can take
// its time instead of a flag day.
func (p *Person) AfterFind(tx *gorm.DB) error {
	if p.CPF == nil {
		return nil
	}
	decrypted, err := utils.Decrypt(*p.CPF)
	if err != nil {
		return err
	}
	p.CPF = &decrypted
	return nil
}

func (p *Person) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

// ComputeComplete reports whether all essential fields of a person are filled in.
// Complement is excluded since it's optional by nature (not every address has one).
// Handlers use this to (re)calculate the Complete flag unless the caller explicitly overrides it.
func (p *Person) ComputeComplete() bool {
	return p.Name != "" &&
		p.CPF != nil &&
		p.BirthDate != nil &&
		p.Email != nil &&
		p.Phone != nil &&
		p.ZipCode != nil &&
		p.State != nil &&
		p.City != nil &&
		p.Address != nil &&
		p.AddressNumber != nil &&
		p.Neighborhood != nil
}
