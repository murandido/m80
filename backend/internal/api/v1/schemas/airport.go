package schemas

// AirportResponse defines how an airport is returned, both nested in an issuance
// response and as a search result from GET /airports.
type AirportResponse struct {
	ID       string  `json:"id"`
	IATACode string  `json:"iata_code"`
	Name     string  `json:"name"`
	City     *string `json:"city"`
	State    *string `json:"state"`
	Country  string  `json:"country"`
}

// CreateAirportRequest defines the expected payload to create a new airport.
type CreateAirportRequest struct {
	IATACode string  `json:"iata_code" validate:"required"`
	Name     string  `json:"name" validate:"required"`
	City     *string `json:"city" validate:"omitempty"`
	State    *string `json:"state" validate:"omitempty"`
	Country  *string `json:"country" validate:"omitempty"`
}

// UpdateAirportRequest defines the payload for updating an existing airport.
// All fields are optional pointers: nil means "don't change this field".
type UpdateAirportRequest struct {
	IATACode *string `json:"iata_code" validate:"omitempty"`
	Name     *string `json:"name" validate:"omitempty"`
	City     *string `json:"city" validate:"omitempty"`
	State    *string `json:"state" validate:"omitempty"`
	Country  *string `json:"country" validate:"omitempty"`
}

// AirportPaginatedData is the data payload for the paginated airport listing.
type AirportPaginatedData struct {
	Airports []AirportResponse `json:"airports"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	Limit    int               `json:"limit"`
}
