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
