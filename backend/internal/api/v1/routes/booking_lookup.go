package routes

import (
	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/handlers"
	"github.com/murandido/m-80-backend/internal/booking"
	"gorm.io/gorm"
)

// RegisterBookingLookupRoutes mounts the direct-lookups report. Dev only, like the airline
// screen it is read next to: it is where direct_booking_enabled gets decided.
//
// TEMPORARY - see migration 66. Delete this file, its line in v1/router.go and the handler
// once no carrier falls back to Traveos.
func RegisterBookingLookupRoutes(r chi.Router, db *gorm.DB, bookings *booking.Registry) {
	handler := handlers.NewBookingLookupReportHandler(db, bookings)

	r.With(middleware.RequireDev()).Get("/booking-lookups/report", handler.GetReport)
}
