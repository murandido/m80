package schemas

import "time"

// UpdateSaleRequest is the payload for PATCH /sales/{id}. Both fields are optional
// pointers: nil means "don't change this field". An explicit null client_id unlinks the
// sale from its client, which is why the handler has to look at the raw body to tell
// "absent" from "null".
type UpdateSaleRequest struct {
	ClientID *string `json:"client_id" validate:"omitempty,uuid"`
	Paid     *bool   `json:"paid" validate:"omitempty"`
}

// SaleResponse is a flat, denormalized view of a sale for the "Vendas > Clientes" table -
// no nested relations, in the spirit of IssuanceSummaryResponse. Monetary fields are in
// cents, like everywhere else in the API.
type SaleResponse struct {
	ID         string  `json:"id"`
	IssuanceID string  `json:"issuance_id"`
	ClientID   *string `json:"client_id"`
	ClientName *string `json:"client_name"`
	Paid       bool    `json:"paid"`

	ProgramLocator    string    `json:"program_locator"`
	MainPassengerName *string   `json:"main_passenger_name"`
	IssuedAt          time.Time `json:"issued_at"`
	IssuerName        string    `json:"issuer_name"`

	Points             int  `json:"points"`
	ValueOfTheThousand int  `json:"value_of_the_thousand"`
	Taxes              *int `json:"taxes"`
	// AmountCents is what the client owes: points/1000 * value_of_the_thousand + taxes.
	AmountCents int `json:"amount_cents"`

	AccountID           string `json:"account_id"`
	AccountLabel        string `json:"account_label"`
	LoyaltyProgramName  string `json:"loyalty_program_name"`
	LoyaltyProgramColor string `json:"loyalty_program_color"`

	// CancelledAt set means the sale was reopened by the issuance being cancelled: money
	// already collected has to go back, as opposed to simply never having been charged.
	// There is no "deleted" counterpart - deleting an issuance deletes its sale.
	CancelledAt *time.Time `json:"cancelled_at"`
}

// SupplierBalanceResponse is one row of the "Vendas > Fornecedores" table: how many miles
// are owed to the administrator of a given account for a period. Deliberately miles only -
// the R$ conversion is done outside the system, per account agreement.
type SupplierBalanceResponse struct {
	AccountID           string  `json:"account_id"`
	AccountLabel        string  `json:"account_label"`
	LoyaltyProgramName  string  `json:"loyalty_program_name"`
	LoyaltyProgramColor string  `json:"loyalty_program_color"`
	AdministratorName   *string `json:"administrator_name"`

	IssuedPoints   int `json:"issued_points"`
	RefundedPoints int `json:"refunded_points"`
	NetPoints      int `json:"net_points"`
}
