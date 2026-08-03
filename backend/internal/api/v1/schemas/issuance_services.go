package schemas

// The seat and baggage sub-resources of an issuance. They are two resources
// rather than one, because they exist at different granularities: a seat belongs
// to a passenger on a leg, while a baggage allowance belongs to a passenger for
// a whole direction. A single endpoint would have to invent a shape that fits
// neither.
//
// Both are symmetric on purpose: what GET returns is exactly what PUT accepts,
// so the panel can read, edit and write back the same object without a mapping
// layer that can drift.

// IssuanceSeat is one passenger's seat on one leg.
type IssuanceSeat struct {
	PassengerID string `json:"passenger_id" validate:"required,uuid4"`
	FlightID    string `json:"flight_id" validate:"required,uuid4"`
	// Seat is null when the airline never assigned one, which is a real state -
	// several assign at check-in - and not the same as an empty string.
	Seat *string `json:"seat" validate:"omitempty,max=8"`
}

// IssuanceBaggage is one baggage entitlement for one direction.
type IssuanceBaggage struct {
	PassengerID string `json:"passenger_id" validate:"required,uuid4"`
	Direction   string `json:"direction" validate:"required,oneof=outbound inbound"`
	// Position is the entitlement's place in the airline's own list. It decides
	// which icon the ticket draws - the first is the cabin bag - so the panel
	// sends it back rather than letting the server renumber.
	Position int `json:"position" validate:"min=0"`
	// Weight is null when the airline reports a bare count, which the ticket then
	// labels by kind instead.
	Weight   *string `json:"weight" validate:"omitempty,max=16"`
	Quantity int     `json:"quantity" validate:"min=0,max=99"`
}

// PutIssuanceSeatsRequest replaces every seat of an issuance at once. It is the
// whole set, never a delta: the operator's browser may have been open for an
// hour while the bot re-synced the same issuance, and a delta applied on top of
// that would merge two versions nobody chose.
type PutIssuanceSeatsRequest struct {
	Seats []IssuanceSeat `json:"seats" validate:"required,dive"`
}

// PutIssuanceBaggageRequest replaces every baggage allowance of an issuance at
// once, for the same reason as PutIssuanceSeatsRequest.
type PutIssuanceBaggageRequest struct {
	Baggage []IssuanceBaggage `json:"baggage" validate:"required,dive"`
}

// IssuanceSeatsResponse is the envelope's data for the seats routes.
type IssuanceSeatsResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    []IssuanceSeat `json:"data"`
}

// IssuanceBaggageResponse is the envelope's data for the baggage routes.
type IssuanceBaggageResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    []IssuanceBaggage `json:"data"`
}
