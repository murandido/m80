package schemas

import "time"

// DirectLookupReport is what GET /booking-lookups/report answers with: how each
// airline's own integration fared over a window, next to what Traveos had to catch.
//
// TEMPORARY, like the table it reads (migration 66): it exists to decide when each
// carrier can drop Traveos, and goes away with the vendor.
type DirectLookupReport struct {
	Days     int                   `json:"days" example:"7"`
	Since    time.Time             `json:"since"`
	Airlines []AirlineLookupReport `json:"airlines"`
}

// AirlineLookupReport is one airline's line. Counts are per sync unless named for
// attempts: a sync is one call that may have asked the airline and then Traveos.
type AirlineLookupReport struct {
	AirlineID            string `json:"airline_id"`
	Code                 string `json:"code" example:"COPA"`
	Name                 string `json:"name" example:"COPA AIRLINES"`
	DirectBookingEnabled bool   `json:"direct_booking_enabled"`
	TraveosSupported     bool   `json:"traveos_supported"`
	// DirectProvider is the name of the airline's registered integration ("copa"), empty
	// when none is registered on this server.
	DirectProvider string `json:"direct_provider" example:"copa"`

	Syncs int `json:"syncs"`
	// AnsweredByDirect, AnsweredByTraveos and Unanswered split the syncs by who found the
	// booking. FellBackToTraveos is the part of AnsweredByTraveos where the airline was
	// asked first and did not answer - the number that has to reach zero.
	AnsweredByDirect  int `json:"answered_by_direct"`
	AnsweredByTraveos int `json:"answered_by_traveos"`
	FellBackToTraveos int `json:"fell_back_to_traveos"`
	Unanswered        int `json:"unanswered"`

	// The direct integration's own attempts, by outcome.
	DirectFound       int `json:"direct_found"`
	DirectNotFound    int `json:"direct_not_found"`
	DirectUnavailable int `json:"direct_unavailable"`
	DirectRefused     int `json:"direct_refused"`
	// DirectAverageMS is the mean duration of the direct attempts, 0 when there were none.
	DirectAverageMS int `json:"direct_average_ms"`

	// RecentFailures are the latest direct attempts that did not find the booking, newest
	// first, capped.
	RecentFailures []LookupFailure `json:"recent_failures"`
}

// LookupFailure is one direct attempt that did not find the booking.
type LookupFailure struct {
	IssuanceID string `json:"issuance_id"`
	Locator    string `json:"locator" example:"ABC123"`
	Outcome    string `json:"outcome" example:"unavailable"`
	Error      string `json:"error"`
	// AnsweredBy is who found the booking in the same sync ("traveos"), empty when nobody did.
	AnsweredBy string    `json:"answered_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// DirectLookupReportResponse is the standard envelope around the report.
type DirectLookupReportResponse struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"Direct lookup report fetched successfully"`
	Data    DirectLookupReport `json:"data"`
}
