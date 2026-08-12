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

// UpdateSupplierIssuancesRequest sets the payment terms on a set of issuances: what the
// agency pays per thousand miles, and whether the taxes are reimbursed on top.
//
// It takes an explicit list of ids rather than the filters that produced them: re-deriving
// the set on the server would let it write to rows the operator never saw, and the whole
// point of the confirmation dialog is that the two are the same set.
//
// Both terms are optional and an omitted one is left alone, which is what lets one endpoint
// serve four controls - the cell, the bulk rate, the row's tax tick and the bulk tax
// buttons - without four code paths that can drift apart. Note that omitted is not the same
// as null for the rate: see UpdateSupplierIssuances for how the two are told apart.
type UpdateSupplierIssuancesRequest struct {
	// IssuanceIDs is required and must not be empty - an UPDATE with no WHERE is not a
	// reachable state through this endpoint.
	IssuanceIDs []string `json:"issuance_ids" validate:"required,min=1,max=1000,dive,uuid"`
	// CostOfTheThousand is in centavos. Null clears the rate back to "not agreed yet",
	// which is what makes the payment summary refuse to generate again.
	CostOfTheThousand *int `json:"cost_of_the_thousand" validate:"omitempty,min=0" example:"1650"`
	// TaxesIncludedInCost says whether the taxes are reimbursed on these issuances.
	TaxesIncludedInCost *bool `json:"taxes_included_in_cost" example:"true"`
}

// UpdateSupplierIssuancesResponse reports how many issuances the write actually touched,
// which the panel compares against what it asked for.
type UpdateSupplierIssuancesResponse struct {
	Updated int `json:"updated" example:"47"`
}

// SupplierIssuanceResponse is one line of an account's statement - the un-aggregated
// version of the balance above.
//
// An issuance produces an "issued" line in the period it was created and, separately, a
// "refund" line in the period it was cancelled: miles that were paid for and came back are
// money owed in the other direction, in the month the cancellation happened rather than the
// month of the original booking. An issuance created and cancelled inside the same period
// therefore produces both lines, and they cancel out.
//
// Points and Taxes arrive already signed - negative on a refund - so a caller only ever has
// to sum. There is no amount in reais here on purpose: whether taxes count towards what is
// paid is a choice made on screen, so the money is computed by whoever is displaying it,
// from CostOfTheThousand.
type SupplierIssuanceResponse struct {
	IssuanceID string `json:"issuance_id"`
	// Kind is "issued" or "refund".
	Kind string `json:"kind" example:"issued"`
	// Date is the issuance's created_at, or its cancelled_at on a refund line.
	Date                time.Time `json:"date"`
	ProgramLocator      string    `json:"program_locator" example:"ABC123"`
	AirlineLocator      *string   `json:"airline_locator"`
	OriginIATACode      *string   `json:"origin_iata_code" example:"GRU"`
	DestinationIATACode *string   `json:"destination_iata_code" example:"MIA"`

	Points int  `json:"points" example:"50000"`
	Taxes  *int `json:"taxes" example:"8400"`
	// CostOfTheThousand is what the agency pays the administrator per thousand miles, in
	// centavos. Nil means it has not been agreed yet, which is what makes the payment
	// summary refuse to generate rather than under-report the debt.
	CostOfTheThousand *int `json:"cost_of_the_thousand" example:"1650"`
	// TaxesIncludedInCost says whether Taxes is part of what is paid for this line.
	TaxesIncludedInCost bool `json:"taxes_included_in_cost" example:"true"`
}

// SupplierBalanceResponse is one row of the "Vendas > Fornecedores" table: how many miles
// are owed to the administrator of a given account for a period, and the statement of the
// issuances behind that figure.
type SupplierBalanceResponse struct {
	AccountID           string  `json:"account_id"`
	AccountLabel        string  `json:"account_label"`
	LoyaltyProgramName  string  `json:"loyalty_program_name"`
	LoyaltyProgramColor string  `json:"loyalty_program_color"`
	AdministratorID     *string `json:"administrator_id"`
	AdministratorName   *string `json:"administrator_name"`

	IssuedPoints   int `json:"issued_points"`
	RefundedPoints int `json:"refunded_points"`
	NetPoints      int `json:"net_points"`

	// Issuances is every line that produced the totals above, in chronological order.
	Issuances []SupplierIssuanceResponse `json:"issuances"`
}
