package schemas

import "time"

// CreateObservationPresetRequest defines the expected payload to create a new observation preset
type CreateObservationPresetRequest struct {
	Code string `json:"code" validate:"required"`
	Text string `json:"text" validate:"required"`
}

// UpdateObservationPresetRequest defines the payload for updating an existing observation preset.
// All fields are optional pointers: nil means "don't change this field".
type UpdateObservationPresetRequest struct {
	Code *string `json:"code" validate:"omitempty"`
	Text *string `json:"text" validate:"omitempty"`
}

// ObservationPresetResponse defines how an observation preset should be returned in the API.
type ObservationPresetResponse struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}
