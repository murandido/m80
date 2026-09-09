package schemas

import "time"

// CreatePointBatchOriginRequest defines the expected payload to create a new origin.
type CreatePointBatchOriginRequest struct {
	Name string `json:"name" validate:"required" example:"Livelo"`
}

// UpdatePointBatchOriginRequest defines the payload for updating an existing origin.
// All fields are optional pointers: nil means "don't change this field".
type UpdatePointBatchOriginRequest struct {
	Name   *string `json:"name" validate:"omitempty"`
	Active *bool   `json:"active" validate:"omitempty"`
}

// PointBatchOriginResponse defines how an origin should be returned in the API.
type PointBatchOriginResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name" example:"Livelo"`
	Active    bool      `json:"active" example:"true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PointBatchOriginListResponse is the GET /point-batch-origins route response.
type PointBatchOriginListResponse struct {
	Success bool                       `json:"success" example:"true"`
	Message string                     `json:"message" example:"Origens listadas com sucesso"`
	Data    []PointBatchOriginResponse `json:"data"`
}

// PointBatchOriginSingleResponse is the response from single-origin routes.
type PointBatchOriginSingleResponse struct {
	Success bool                     `json:"success" example:"true"`
	Message string                   `json:"message" example:"Origem criada com sucesso"`
	Data    PointBatchOriginResponse `json:"data"`
}
