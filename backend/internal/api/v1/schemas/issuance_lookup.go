package schemas

// IssuanceLookupRequest is what the panel knows about a booking before the issuance
// exists: the fields a booking provider is asked with, and nothing else.
type IssuanceLookupRequest struct {
	AirlineID           string  `json:"airline_id" validate:"required,uuid"`
	ProgramLocator      string  `json:"program_locator" validate:"required"`
	AirlineLocator      *string `json:"airline_locator" validate:"omitempty"`
	OriginIATACode      *string `json:"origin_iata_code" validate:"omitempty"`
	MainPassengerName   *string `json:"main_passenger_name" validate:"omitempty"`
	PassengerLastName   *string `json:"passenger_last_name" validate:"omitempty"`
	LatamPurchaseNumber *string `json:"latam_purchase_number" validate:"omitempty"`
}

// IssuanceLookupPreview is the part of the fetched booking the panel's form shows.
// The itinerary itself is only written by the sync that follows the save.
type IssuanceLookupPreview struct {
	FlightDate          *string `json:"flight_date" example:"2026-10-22"`
	OriginIATACode      *string `json:"origin_iata_code" example:"GRU"`
	DestinationIATACode *string `json:"destination_iata_code" example:"MIA"`
	MainPassengerName   *string `json:"main_passenger_name" example:"FERNANDA ASSIS"`
	LatamPurchaseNumber *string `json:"latam_purchase_number"`
	// Source names the provider that answered - the airline's own integration or
	// the vendor behind it.
	Source string `json:"source" example:"traveos"`
}

// IssuanceLookupResponse is the POST /issuances/lookup response.
type IssuanceLookupResponse struct {
	Success bool                  `json:"success" example:"true"`
	Message string                `json:"message" example:"Reserva encontrada"`
	Data    IssuanceLookupPreview `json:"data"`
}
