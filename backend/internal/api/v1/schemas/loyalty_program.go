package schemas

import "time"

// CreateLoyaltyProgramRequest defines the expected payload to create a new loyalty program
type CreateLoyaltyProgramRequest struct {
	Name             string  `json:"name" validate:"required"`
	Code             string  `json:"code" validate:"required"`
	Color            *string `json:"color" validate:"omitempty"`
	DefaultAirlineID string  `json:"default_airline_id" validate:"required,uuid"`
	// The beneficiary cap the panel enforces. Nil means nobody has researched this
	// program's rule, which is not a cap of zero - the accounts screen then shows no
	// counter at all. See data.CPFLimit* for the values.
	CPFLimit       *int    `json:"cpf_limit" validate:"omitempty,min=0"`
	CPFLimitWindow *string `json:"cpf_limit_window" validate:"omitempty,oneof=calendar_year rolling_12_months"`
	CPFLimitMode   *string `json:"cpf_limit_mode" validate:"omitempty,oneof=distinct_people per_issuance"`
	// The redemption fee charged per passenger and per trecho when the ticket is issued
	// fewer than RedemptionFeeAdvanceDays days before the flight. All three nil means the
	// program charges no such fee, which is not the same as a fee of zero: nothing is
	// computed at all. See data.LoyaltyProgram and migration 40.
	RedemptionFeeDomesticCents      *int `json:"redemption_fee_domestic_cents" validate:"omitempty,min=0" example:"6990"`
	RedemptionFeeInternationalCents *int `json:"redemption_fee_international_cents" validate:"omitempty,min=0" example:"17490"`
	RedemptionFeeAdvanceDays        *int `json:"redemption_fee_advance_days" validate:"omitempty,min=0" example:"120"`
}

// UpdateLoyaltyProgramRequest defines the payload for updating an existing loyalty program.
// All fields are optional pointers: nil means "don't change this field".
type UpdateLoyaltyProgramRequest struct {
	Name             *string `json:"name" validate:"omitempty"`
	Code             *string `json:"code" validate:"omitempty"`
	Color            *string `json:"color" validate:"omitempty"`
	DefaultAirlineID *string `json:"default_airline_id" validate:"omitempty,uuid"`
	// The beneficiary cap the panel enforces. Nil means nobody has researched this
	// program's rule, which is not a cap of zero - the accounts screen then shows no
	// counter at all. See data.CPFLimit* for the values.
	CPFLimit       *int    `json:"cpf_limit" validate:"omitempty,min=0"`
	CPFLimitWindow *string `json:"cpf_limit_window" validate:"omitempty,oneof=calendar_year rolling_12_months"`
	CPFLimitMode   *string `json:"cpf_limit_mode" validate:"omitempty,oneof=distinct_people per_issuance"`
	// The redemption fee charged per passenger and per trecho when the ticket is issued
	// fewer than RedemptionFeeAdvanceDays days before the flight. All three nil means the
	// program charges no such fee, which is not the same as a fee of zero: nothing is
	// computed at all. See data.LoyaltyProgram and migration 40.
	RedemptionFeeDomesticCents      *int `json:"redemption_fee_domestic_cents" validate:"omitempty,min=0" example:"6990"`
	RedemptionFeeInternationalCents *int `json:"redemption_fee_international_cents" validate:"omitempty,min=0" example:"17490"`
	RedemptionFeeAdvanceDays        *int `json:"redemption_fee_advance_days" validate:"omitempty,min=0" example:"120"`
}

// LoyaltyProgramResponse defines how a loyalty program should be returned in the API.
type LoyaltyProgramResponse struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Code             string           `json:"code"`
	Color            string           `json:"color"`
	DefaultAirlineID string           `json:"default_airline_id"`
	DefaultAirline   *AirlineResponse `json:"default_airline,omitempty"`
	// Null when the program's rule has not been researched. The airlines change these
	// without notice, which is why they are editable rather than only seeded.
	CPFLimit       *int    `json:"cpf_limit"`
	CPFLimitWindow *string `json:"cpf_limit_window"`
	CPFLimitMode   *string `json:"cpf_limit_mode"`
	// Null when the program charges no redemption fee. Same reason as the cap above: the
	// airlines move these - Azul's launched at 90 days and a price per tier - so they are
	// editable rather than only seeded.
	RedemptionFeeDomesticCents      *int      `json:"redemption_fee_domestic_cents"`
	RedemptionFeeInternationalCents *int      `json:"redemption_fee_international_cents"`
	RedemptionFeeAdvanceDays        *int      `json:"redemption_fee_advance_days"`
	CreatedAt                       time.Time `json:"created_at"`
	UpdatedAt                       time.Time `json:"updated_at"`
	// Same as AccountResponse.DeletedAt: non-null only on a program reached through
	// a past issuance, so the grid can label the row without offering the program.
	DeletedAt *time.Time `json:"deleted_at"`
}
