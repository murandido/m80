package utils

import (
	"net/http"

	"github.com/go-chi/render"
)

// StandardResponse defines the universal JSON structure for the API.
type StandardResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // 'omitempty' removes the field from JSON if it's nil
}

// SendJSON is a helper to send successful responses.
func SendJSON(w http.ResponseWriter, r *http.Request, statusCode int, message string, data interface{}) {
	render.Status(r, statusCode)
	render.JSON(w, r, StandardResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func SendError(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	render.Status(r, statusCode)
	render.JSON(w, r, StandardResponse{
		Success: false,
		Message: message,
		Data:    nil,
	})
}
