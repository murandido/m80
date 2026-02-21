package schemas

import (
	"time"
)

// CreateUserRequest defines the expected payload to create a new user
type CreateUserRequest struct {
	Name     string `json:"name" validate:"required"`
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required,min=6,max=20"`
	Role     string `json:"role" validate:"omitempty,oneof=user admin"`
}

// UserResponse defines how a user should be returned in the API
type UserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// UpdateUserRequest defines the payload for updating an existing user
type UpdateUserRequest struct {
	Name     string `json:"name" validate:"omitempty"`
	Phone    string `json:"phone" validate:"omitempty"`
	Password string `json:"password" validate:"omitempty,min=6,max=20"`
	Role     string `json:"role" validate:"omitempty,oneof=user admin"`
}
