package schemas

import (
	"time"
)

// CreateUserRequest defines the expected payload to create a new user
type CreateUserRequest struct {
	Name     string `json:"name" validate:"required"`
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required,min=6,max=20"`
	Role     string `json:"role" validate:"omitempty,oneof=user admin dev"`
	// AgencyIDs are the agencies the new user may see. Only a dev may send it; absent means
	// none, and it is ignored for a dev, whose role already covers every agency.
	AgencyIDs *[]string `json:"agency_ids" validate:"omitempty,dive,uuid"`
}

// UserResponse defines how a user should be returned in the API
type UserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Role  string `json:"role"`
	// AgencyIDs are the agencies granted to the user. Always empty for a dev: the role
	// itself means every agency.
	AgencyIDs []string  `json:"agency_ids"`
	CreatedAt time.Time `json:"created_at"`
}

// UpdateUserRequest defines the payload for updating an existing user
type UpdateUserRequest struct {
	Name     string `json:"name" validate:"omitempty"`
	Phone    string `json:"phone" validate:"omitempty"`
	Password string `json:"password" validate:"omitempty,min=6,max=20"`
	Role     string `json:"role" validate:"omitempty,oneof=user admin dev"`
	// AgencyIDs replaces the user's agencies when present - an empty list removes them all.
	// Absent (null) leaves them alone, which is what a panel tab opened before this field
	// existed keeps sending. Only a dev may send it.
	AgencyIDs *[]string `json:"agency_ids" validate:"omitempty,dive,uuid"`
}

// UpdateMeRequest is what a user may change about themselves. There is no role
// field on purpose: without one, a "role" key in the payload has nowhere to land.
// CurrentPassword is only required when the change touches the login - a new
// password or a new phone - which the handler enforces, not the tags.
type UpdateMeRequest struct {
	Name            string `json:"name" validate:"omitempty"`
	Phone           string `json:"phone" validate:"omitempty"`
	NewPassword     string `json:"new_password" validate:"omitempty,min=6,max=20"`
	CurrentPassword string `json:"current_password" validate:"omitempty"`
}
