package utils

import (
	"log/slog"
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

// SendError sends a failure response. Every 5xx is also logged here, because most handlers
// return a 500 without logging the cause, and chi's request logger records only the status:
// without this line the reason is lost. Only the path goes in, never the query string, which
// carries search terms such as client names and documents.
func SendError(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	if statusCode >= http.StatusInternalServerError {
		slog.Error("http 5xx", "method", r.Method, "path", r.URL.Path, "status", statusCode, "message", message)
	}
	render.Status(r, statusCode)
	render.JSON(w, r, StandardResponse{
		Success: false,
		Message: message,
		Data:    nil,
	})
}
