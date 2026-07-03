package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Person struct {
	ID            string     `gorm:"primaryKey;type:uuid" json:"id"`
	Name          string     `gorm:"not null" json:"name"`
	CPF           *string    `gorm:"column:cpf;uniqueIndex:idx_people_cpf,where:cpf IS NOT NULL" json:"cpf"`
	BirthDate     *time.Time `gorm:"type:date;column:birth_date" json:"birth_date"`
	Email         *string    `json:"email"`
	Phone         *string    `json:"phone"`
	ZipCode       *string    `gorm:"column:zip_code" json:"zip_code"`
	State         *string    `json:"state"`
	City          *string    `json:"city"`
	Address       *string    `json:"address"`
	AddressNumber *string    `gorm:"column:address_number" json:"address_number"`
	Complement    *string    `json:"complement"`
	Neighborhood  *string    `json:"neighborhood"`
	Complete      bool       `gorm:"not null;default:false" json:"complete"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
