package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// How a program's CPF cap is measured. See migration 37 for the published rules.
const (
	// CPFLimitWindowCalendarYear resets on 1 January (Smiles, Azul).
	CPFLimitWindowCalendarYear = "calendar_year"
	// CPFLimitWindowRolling12Months slides day by day (LATAM Pass).
	CPFLimitWindowRolling12Months = "rolling_12_months"

	// CPFLimitModeDistinctPeople counts each person once however many times they are
	// issued for - re-using a beneficiary costs no new slot (Smiles, Azul).
	CPFLimitModeDistinctPeople = "distinct_people"
	// CPFLimitModePerIssuance counts every issuance, even for someone already issued
	// on this account (LATAM Pass).
	CPFLimitModePerIssuance = "per_issuance"
)

type LoyaltyProgram struct {
	ID               string   `gorm:"primaryKey;type:uuid" json:"id"`
	Name             string   `gorm:"not null" json:"name"`
	Code             string   `gorm:"uniqueIndex;not null" json:"code"`
	Color            string   `gorm:"not null;default:'#64748B'" json:"color"`
	DefaultAirlineID string   `gorm:"column:default_airline_id;type:uuid;not null" json:"default_airline_id"`
	DefaultAirline   *Airline `gorm:"foreignKey:DefaultAirlineID;references:ID" json:"default_airline,omitempty"`
	// How many people (or issuances - see CPFLimitMode) an account on this program may
	// carry per window. Nil means nobody has researched this program's rule yet, which is
	// not the same as a limit of zero: the panel then shows no counter at all.
	CPFLimit *int `gorm:"column:cpf_limit" json:"cpf_limit"`
	// CPFLimitWindow is the period the limit is measured over: CPFLimitWindowCalendarYear
	// (Smiles, Azul - resets 1 January) or CPFLimitWindowRolling12Months (LATAM).
	CPFLimitWindow *string `gorm:"column:cpf_limit_window;type:cpf_limit_window" json:"cpf_limit_window"`
	// CPFLimitMode is what gets counted. Smiles counts distinct people, so re-issuing for
	// someone already used costs nothing; LATAM counts every issuance, so it does. The two
	// dimensions vary independently, which is why this is not folded into the window.
	CPFLimitMode *string   `gorm:"column:cpf_limit_mode;type:cpf_limit_mode" json:"cpf_limit_mode"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Same reasoning as Account: accounts and issuances reference a program with
	// ON DELETE RESTRICT, so a program that was ever used could never be removed.
	// Exposed for the same reason too - the grid gets archived programs embedded in
	// old issuances and has to label them without offering them in the picker.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (lp *LoyaltyProgram) BeforeCreate(tx *gorm.DB) error {
	if lp.ID == "" {
		lp.ID = uuid.New().String()
	}
	return nil
}
