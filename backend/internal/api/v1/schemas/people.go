package schemas

import "time"

// CreatePersonRequest defines the expected payload to create a new person
type CreatePersonRequest struct {
	Name          string  `json:"name" validate:"required"`
	CPF           *string `json:"cpf" validate:"omitempty"`
	BirthDate     *string `json:"birth_date" validate:"omitempty"`
	Email         *string `json:"email" validate:"omitempty,email"`
	Phone         *string `json:"phone" validate:"omitempty"`
	ZipCode       *string `json:"zip_code" validate:"omitempty"`
	State         *string `json:"state" validate:"omitempty"`
	City          *string `json:"city" validate:"omitempty"`
	Address       *string `json:"address" validate:"omitempty"`
	AddressNumber *string `json:"address_number" validate:"omitempty"`
	Complement    *string `json:"complement" validate:"omitempty"`
	Neighborhood  *string `json:"neighborhood" validate:"omitempty"`
	// Complete overrides the automatically computed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// UpdatePersonRequest defines the payload for updating an existing person.
// All fields are optional pointers: nil means "don't change this field".
type UpdatePersonRequest struct {
	Name          *string `json:"name" validate:"omitempty"`
	CPF           *string `json:"cpf" validate:"omitempty"`
	BirthDate     *string `json:"birth_date" validate:"omitempty"`
	Email         *string `json:"email" validate:"omitempty,email"`
	Phone         *string `json:"phone" validate:"omitempty"`
	ZipCode       *string `json:"zip_code" validate:"omitempty"`
	State         *string `json:"state" validate:"omitempty"`
	City          *string `json:"city" validate:"omitempty"`
	Address       *string `json:"address" validate:"omitempty"`
	AddressNumber *string `json:"address_number" validate:"omitempty"`
	Complement    *string `json:"complement" validate:"omitempty"`
	Neighborhood  *string `json:"neighborhood" validate:"omitempty"`
	// Complete overrides the automatically recomputed value when provided
	Complete *bool `json:"complete" validate:"omitempty"`
}

// PersonResponse defines how a person should be returned in the API
type PersonResponse struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	CPF           *string                  `json:"cpf"`
	BirthDate     *time.Time               `json:"birth_date"`
	Email         *string                  `json:"email"`
	Phone         *string                  `json:"phone"`
	ZipCode       *string                  `json:"zip_code"`
	State         *string                  `json:"state"`
	City          *string                  `json:"city"`
	Address       *string                  `json:"address"`
	AddressNumber *string                  `json:"address_number"`
	Complement    *string                  `json:"complement"`
	Neighborhood  *string                  `json:"neighborhood"`
	Nicknames     []PersonNicknameResponse `json:"nicknames"`
	Complete      bool                     `json:"complete"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}
