package schemas

import (
	"time"

	"gorm.io/datatypes"
)

// CreateIssuanceRequest defines the expected payload to create a new issuance.
// complete is never accepted - it always starts as false.
type CreateIssuanceRequest struct {
	ProgramLocator   string `json:"program_locator" validate:"required"`
	LoyaltyProgramID string `json:"loyalty_program_id" validate:"required,uuid"`
	AccountID        string `json:"account_id" validate:"required,uuid"`
	// AccountNicknameID is optional - accounts without a registered alias are still
	// issuable. When present it must belong to AccountID.
	AccountNicknameID  *string `json:"account_nickname_id" validate:"omitempty,uuid"`
	Points             int     `json:"points" validate:"required"`
	ValueOfTheThousand int     `json:"value_of_the_thousand" validate:"required"`
	// AirlineID defaults to the loyalty program's default_airline_id when omitted.
	AirlineID *string `json:"airline_id" validate:"omitempty,uuid"`
	// AgencyID defaults to the agency where is_default=true when omitted.
	AgencyID *string `json:"agency_id" validate:"omitempty,uuid"`
	// IssuerID defaults to the authenticated (JWT) user when omitted.
	IssuerID            *string        `json:"issuer_id" validate:"omitempty,uuid"`
	AirlineLocator      *string        `json:"airline_locator" validate:"omitempty"`
	LatamPurchaseNumber *string        `json:"latam_purchase_number" validate:"omitempty"`
	MainPassengerName   *string        `json:"main_passenger_name" validate:"omitempty"`
	TripType            *string        `json:"trip_type" validate:"omitempty,oneof=one_way round_trip multi_leg"`
	OriginIATACode      *string        `json:"origin_iata_code" validate:"omitempty"`
	DestinationIATACode *string        `json:"destination_iata_code" validate:"omitempty"`
	Taxes               *int           `json:"taxes" validate:"omitempty"`
	Observation         *string        `json:"observation" validate:"omitempty"`
	AdditionalServices  datatypes.JSON `json:"additional_services" validate:"omitempty" swaggertype:"object"`
	// FlightDate (YYYY-MM-DD) creates the primary outbound flight leg.
	FlightDate *string `json:"flight_date" validate:"omitempty,datetime=2006-01-02"`
	// IssuedOn (YYYY-MM-DD) is the day the issuance happened, when that is not today -
	// a booking found missing on a statement and recorded afterwards. Absent means now,
	// which is the truth for everything recorded as it happens.
	IssuedOn *string `json:"issued_on" validate:"omitempty,datetime=2006-01-02" example:"2026-09-03"`
	// TraveosLookupID names a POST /issuances/traveos-lookup whose itinerary is written
	// onto the issuance as it is created, without asking the vendor again.
	TraveosLookupID *string `json:"traveos_lookup_id" validate:"omitempty"`
	// TraveosTicketCode is the same lookup's ticket, read directly when the lookup has
	// expired from the server's memory - one call, never a search or an emission.
	TraveosTicketCode *string `json:"traveos_ticket_code" validate:"omitempty"`
}

// TraveosLookupRequest is what the panel knows about a booking before the issuance
// exists: the fields Traveos is asked with, and nothing else.
type TraveosLookupRequest struct {
	AirlineID           string  `json:"airline_id" validate:"required,uuid"`
	ProgramLocator      string  `json:"program_locator" validate:"required"`
	AirlineLocator      *string `json:"airline_locator" validate:"omitempty"`
	OriginIATACode      *string `json:"origin_iata_code" validate:"omitempty"`
	MainPassengerName   *string `json:"main_passenger_name" validate:"omitempty"`
	LatamPurchaseNumber *string `json:"latam_purchase_number" validate:"omitempty"`
}

// TraveosLookupResponse previews the booking and names the lookup the create reuses.
type TraveosLookupResponse struct {
	LookupID            string  `json:"lookup_id"`
	TicketCode          string  `json:"ticket_code"`
	FlightDate          *string `json:"flight_date" example:"2026-10-22"`
	OriginIATACode      *string `json:"origin_iata_code"`
	DestinationIATACode *string `json:"destination_iata_code"`
	MainPassengerName   *string `json:"main_passenger_name"`
	LatamPurchaseNumber *string `json:"latam_purchase_number"`
}

// UpdateIssuanceRequest defines the payload for a partial (autosave) update of an
// existing issuance. All fields are optional pointers: nil means "don't change this
// field". complete is never accepted directly - use Status instead, which the handler
// translates into complete/cancelled_at.
type UpdateIssuanceRequest struct {
	LoyaltyProgramID *string `json:"loyalty_program_id" validate:"omitempty,uuid"`
	AirlineID        *string `json:"airline_id" validate:"omitempty,uuid"`
	AccountID        *string `json:"account_id" validate:"omitempty,uuid"`
	// AccountNicknameID is only read when AccountID is also present - the two are
	// updated as a unit. See UpdateIssuance for why.
	AccountNicknameID   *string        `json:"account_nickname_id" validate:"omitempty,uuid"`
	AgencyID            *string        `json:"agency_id" validate:"omitempty,uuid"`
	IssuerID            *string        `json:"issuer_id" validate:"omitempty,uuid"`
	ProgramLocator      *string        `json:"program_locator" validate:"omitempty"`
	AirlineLocator      *string        `json:"airline_locator" validate:"omitempty"`
	LatamPurchaseNumber *string        `json:"latam_purchase_number" validate:"omitempty"`
	MainPassengerName   *string        `json:"main_passenger_name" validate:"omitempty"`
	TripType            *string        `json:"trip_type" validate:"omitempty,oneof=one_way round_trip multi_leg"`
	OriginIATACode      *string        `json:"origin_iata_code" validate:"omitempty"`
	DestinationIATACode *string        `json:"destination_iata_code" validate:"omitempty"`
	Points              *int           `json:"points" validate:"omitempty"`
	ValueOfTheThousand  *int           `json:"value_of_the_thousand" validate:"omitempty"`
	Taxes               *int           `json:"taxes" validate:"omitempty"`
	Observation         *string        `json:"observation" validate:"omitempty"`
	AdditionalServices  datatypes.JSON `json:"additional_services" validate:"omitempty" swaggertype:"object"`
	// Status transitions the issuance between incomplete/complete/cancelled. "deleted"
	// is not a valid value here - it's only reachable via DELETE /issuances/{id}.
	Status *string `json:"status" validate:"omitempty,oneof=incomplete complete cancelled"`
	// FlightDate (YYYY-MM-DD) upserts the primary outbound flight leg's departure date.
	FlightDate *string `json:"flight_date" validate:"omitempty,datetime=2006-01-02"`
	// IssuedOn (YYYY-MM-DD) moves the issuance to another day. The time of day is kept,
	// so two rows recorded in the same session keep their order in the FIFO queue.
	IssuedOn *string `json:"issued_on" validate:"omitempty,datetime=2006-01-02" example:"2026-09-03"`
	// The redemption fee this issuance avoided, in centavos, corrected by hand. A value pins
	// the correction against the automatic recalculation; an explicit null releases the
	// issuance back to the rule and recomputes it on the spot.
	//
	// This field is the reason the handler reads the raw body as well as the struct: for
	// every other field here an absent key and a JSON null both decode to nil and mean
	// "leave alone", and that is exactly the distinction the override needs.
	RedemptionFeeSavedCents *int `json:"redemption_fee_saved_cents" validate:"omitempty,min=0" example:"6990"`
}

// CancelIssuanceRequest is the optional body for POST /issuances/{id}/cancel.
// All fields are optional: a bare cancel with no refund is valid. Amounts are in cents.
type CancelIssuanceRequest struct {
	RefundedPoints  *int `json:"refunded_points" validate:"omitempty"`
	RefundedTaxes   *int `json:"refunded_taxes" validate:"omitempty"`
	CancellationFee *int `json:"cancellation_fee" validate:"omitempty"`
	// CancellationOnly marks the issuance as a row that exists only to carry this
	// estorno: the booking was made outside the panel, so its points were never miles
	// issued here and the supplier balance must count the refund alone. Registering the
	// booking and then cancelling it with twice the points is what this replaces.
	CancellationOnly bool `json:"cancellation_only" validate:"omitempty"`
	// CancelledOn (YYYY-MM-DD) is the day the airline gave the points back, when that is
	// not today. It is what dates the refund in the costing and in the supplier balance,
	// so a cancellation recorded a week late still lands in the week it happened.
	CancelledOn *string `json:"cancelled_on" validate:"omitempty,datetime=2006-01-02" example:"2026-09-05"`
}

// IssuanceFlightResponse defines how a flight leg should be returned nested in an issuance response.
type IssuanceFlightResponse struct {
	ID                  string     `json:"id"`
	Direction           string     `json:"direction"`
	LegOrder            int        `json:"leg_order"`
	FlightNumber        *string    `json:"flight_number"`
	OriginIATACode      *string    `json:"origin_iata_code"`
	DestinationIATACode *string    `json:"destination_iata_code"`
	DepartureAt         time.Time  `json:"departure_at"`
	ArrivalAt           *time.Time `json:"arrival_at"`
	DurationMin         *int       `json:"duration_min"`
	Cabin               *string    `json:"cabin"`
	CreatedAt           time.Time  `json:"created_at"`
}

// IssuanceResponse defines how an issuance should be returned in the API.
type IssuanceResponse struct {
	ID                  string                   `json:"id"`
	LoyaltyProgramID    string                   `json:"loyalty_program_id"`
	LoyaltyProgram      *LoyaltyProgramResponse  `json:"loyalty_program,omitempty"`
	AirlineID           string                   `json:"airline_id"`
	Airline             *AirlineResponse         `json:"airline,omitempty"`
	ProgramLocator      string                   `json:"program_locator"`
	AirlineLocator      *string                  `json:"airline_locator"`
	LatamPurchaseNumber *string                  `json:"latam_purchase_number"`
	MainPassengerName   *string                  `json:"main_passenger_name"`
	TripType            *string                  `json:"trip_type"`
	OriginIATACode      *string                  `json:"origin_iata_code"`
	OriginAirport       *AirportResponse         `json:"origin_airport,omitempty"`
	DestinationIATACode *string                  `json:"destination_iata_code"`
	DestinationAirport  *AirportResponse         `json:"destination_airport,omitempty"`
	Points              int                      `json:"points"`
	ValueOfTheThousand  int                      `json:"value_of_the_thousand"`
	Taxes               *int                     `json:"taxes"`
	AccountID           string                   `json:"account_id"`
	Account             *AccountResponse         `json:"account,omitempty"`
	AccountNicknameID   *string                  `json:"account_nickname_id"`
	AccountNickname     *AccountNicknameResponse `json:"account_nickname,omitempty"`
	IssuerID            string                   `json:"issuer_id"`
	Issuer              *UserResponse            `json:"issuer,omitempty"`
	AgencyID            string                   `json:"agency_id"`
	Agency              *AgencyResponse          `json:"agency,omitempty"`
	Complete            bool                     `json:"complete"`
	// TraveosSyncedAt separates "complete because Traveos confirmed the itinerary"
	// from "complete because someone ticked it on the grid". The panel needs the
	// distinction to know whether a ticket can be rendered at all, and the
	// timestamp rather than a flag because the operator's real question is how
	// stale the data is.
	TraveosSyncedAt *time.Time `json:"traveos_synced_at"`
	// TraveosSyncingAt is set while a pull from Traveos is in flight, so every panel
	// shows the row as busy and refuses to edit it - not just the one that started
	// the pull. Already filtered for staleness by the server: a marker left behind
	// by a process that died mid-pull arrives as null.
	TraveosSyncingAt *time.Time `json:"traveos_syncing_at"`
	// TraveosAttemptedAt is when the vendor was last asked about this issuance,
	// whether or not it had the booking. The panel needs the failed lookups too:
	// it is what tells an imported row that has been looked up and not found from
	// one that has never been looked up at all.
	TraveosAttemptedAt *time.Time `json:"traveos_attempted_at"`
	// Imported says the issuance came from the pre-panel spreadsheet. The panel
	// uses it to stop the automatic Traveos lookup after a single attempt, and the
	// server uses it to refuse issuing the ticket at the vendor.
	Imported bool `json:"imported"`
	// CancellationOnly says the row exists only to carry an estorno the spreadsheet
	// recorded for a booking the panel never had. Its points are not miles this agency
	// issued, so the supplier balance counts its refund and not its booking - which is
	// what the panel needs in order to explain a cancelled issuance whose points never
	// show up as issued anywhere.
	CancellationOnly bool `json:"cancellation_only"`
	// RedemptionFeeSavedCents is the airline's redemption fee this issuance did not pay,
	// because the account it was made on is exempt. Null on every ordinary account and
	// whenever the rule could not be applied (no synced itinerary, or a program with no fee
	// configured) - which is not the same as zero, and zero is a real answer: the rule ran
	// and there was no fee to avoid.
	//
	// It does not take part in what the client owes: it is recorded for control and billed
	// nowhere. See data.Issuance.
	RedemptionFeeSavedCents *int `json:"redemption_fee_saved_cents"`
	// RedemptionFeeManual says an operator typed the amount by hand, which is what stops
	// the next Traveos sync from recomputing over the correction.
	RedemptionFeeManual bool `json:"redemption_fee_manual"`
	// Status is derived: "incomplete" | "complete" | "cancelled".
	Status             string                      `json:"status" example:"incomplete"`
	Flights            []IssuanceFlightResponse    `json:"flights,omitempty"`
	Passengers         []IssuancePassengerResponse `json:"passengers,omitempty"`
	AdditionalServices datatypes.JSON              `json:"additional_services" swaggertype:"object"`
	Observation        *string                     `json:"observation"`
	Active             bool                        `json:"active"`
	CancelledAt        *time.Time                  `json:"cancelled_at"`
	CancelledByID      *string                     `json:"cancelled_by_id"`
	RefundedPoints     *int                        `json:"refunded_points"`
	RefundedTaxes      *int                        `json:"refunded_taxes"`
	CancellationFee    *int                        `json:"cancellation_fee"`
	CreatedAt          time.Time                   `json:"created_at"`
	// IssuedOn is created_at as the operator's calendar day, which is what every screen
	// means by "the day of the issuance". Resolved here rather than in the browser so the
	// grid and the closing can never disagree about which day a row is on.
	IssuedOn  string    `json:"issued_on" example:"2026-09-03"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IssuanceSummaryResponse defines a lightweight issuance representation for listing
// screens - only the fields needed to render a table row, no nested relations.
type IssuanceSummaryResponse struct {
	ID                  string     `json:"id"`
	LoyaltyProgramName  string     `json:"loyalty_program_name"`
	AirlineName         string     `json:"airline_name"`
	ProgramLocator      string     `json:"program_locator"`
	AirlineLocator      *string    `json:"airline_locator"`
	IssuedAt            time.Time  `json:"issued_at"`
	FlightDate          *time.Time `json:"flight_date"`
	MainPassengerName   *string    `json:"main_passenger_name"`
	OriginIATACode      *string    `json:"origin_iata_code"`
	DestinationIATACode *string    `json:"destination_iata_code"`
	Points              int        `json:"points"`
	ValueOfTheThousand  int        `json:"value_of_the_thousand"`
	Taxes               *int       `json:"taxes"`
	AccountLogin        *string    `json:"account_login"`
	IssuerName          string     `json:"issuer_name"`
	AgencyName          string     `json:"agency_name"`
	TraveosSyncedAt     *time.Time `json:"traveos_synced_at"`
	TraveosSyncingAt    *time.Time `json:"traveos_syncing_at"`
	TraveosAttemptedAt  *time.Time `json:"traveos_attempted_at"`
	Imported            bool       `json:"imported"`
	CancellationOnly    bool       `json:"cancellation_only"`
	// Status is derived: "incomplete" | "complete" | "cancelled".
	Status string `json:"status" example:"incomplete"`
}
