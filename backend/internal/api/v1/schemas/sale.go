package schemas

import "time"

// UpdateSaleRequest is the payload for PATCH /sales/{id}. Both fields are optional
// pointers: nil means "don't change this field". An explicit null client_id unlinks the
// sale from its client, which is why the handler has to look at the raw body to tell
// "absent" from "null".
type UpdateSaleRequest struct {
	ClientID *string `json:"client_id" validate:"omitempty,uuid"`
	// Paid is the old switch, kept working for a panel that has not been published yet -
	// the two sides deploy by independent labels. It is translated into the statement:
	// true records a payment for whatever is missing, false clears the statement. The
	// current panel settles a sale through /sales/{id}/payments instead.
	Paid *bool `json:"paid" validate:"omitempty"`
}

// CreateSalePaymentRequest is the payload for POST /sales/{id}/payments.
type CreateSalePaymentRequest struct {
	// AmountCents is in centavos and may be negative - that is how money handed back to
	// the client is recorded. Zero is refused: a movement of nothing is only ever a typo.
	AmountCents int `json:"amount_cents" validate:"required" example:"40000"`
	// PaidOn (YYYY-MM-DD) is the day the money moved, which is what the operator knows.
	PaidOn string  `json:"paid_on" validate:"required,datetime=2006-01-02" example:"2026-08-14"`
	Note   *string `json:"note" validate:"omitempty"`
}

// SalePaymentResponse is one line of a sale's statement.
type SalePaymentResponse struct {
	ID          string `json:"id"`
	SaleID      string `json:"sale_id"`
	AmountCents int    `json:"amount_cents" example:"40000"`
	// PaidOn is a calendar day (YYYY-MM-DD), not an instant: the operator knows the day
	// the money moved, not the hour. CreatedAt is when the line was typed.
	PaidOn        string    `json:"paid_on" example:"2026-08-14"`
	Note          *string   `json:"note"`
	CreatedByID   string    `json:"created_by_id"`
	CreatedByName string    `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`
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
	// PaidCents is the sum of the sale's payments, which can exceed AmountCents (the
	// client overpaid) or go negative (more was handed back than came in).
	PaidCents int `json:"paid_cents"`
	// BalanceCents is what is still missing: AmountCents - PaidCents. Zero or less means
	// settled, which is exactly what Paid reports.
	BalanceCents int `json:"balance_cents"`

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
	// Reviewed marks these issuances as already checked while reconciling. Unlike the two
	// terms above it changes no money - it records who has already looked.
	Reviewed *bool `json:"reviewed" example:"true"`
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
	// AccountID and AccountLabel say which account the line came out of. Redundant when the
	// view is grouped by account and the point of the line when it is grouped by
	// administrator, where one row mixes several accounts.
	AccountID    string `json:"account_id"`
	AccountLabel string `json:"account_label"`
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
	// CostCents is the exact amount the points inventory charged this line, present only
	// on an account that owns its points. It is the sum of the batch slices that paid for
	// the booking, and the figure the total is built from - CostOfTheThousand beside it is
	// that same amount rounded to a whole centavo per thousand, for display.
	CostCents *int `json:"cost_cents" example:"504245"`
	// OwnsPointInventory tells the screen this line's cost is derived and must not be
	// offered for editing.
	OwnsPointInventory bool `json:"owns_point_inventory" example:"false"`
	// TaxesIncludedInCost says whether Taxes is part of what is paid for this line.
	TaxesIncludedInCost bool `json:"taxes_included_in_cost" example:"true"`
	// Reviewed says somebody already checked this line while reconciling. Both lines of an
	// issuance that was created and cancelled in the same period carry the same value: the
	// flag is on the issuance, and the two lines are two views of it.
	Reviewed bool `json:"reviewed" example:"false"`
}

// SupplierBalanceResponse is one row of the "Vendas > Fornecedores" table: how many miles
// are owed to the administrator of a given account for a period, and the statement of the
// issuances behind that figure.
type SupplierBalanceResponse struct {
	// GroupKey and GroupLabel identify the row itself, whichever way the caller asked the
	// view to be grouped. The account fields below stay populated so a client that predates
	// group_by keeps working; on an administrator-grouped response they describe one of the
	// administrator's accounts, not the row, and GroupKey is the thing to key on.
	GroupKey   string `json:"group_key"`
	GroupLabel string `json:"group_label"`
	// AccountCount is how many accounts fed this row - always 1 when grouped by account.
	AccountCount int `json:"account_count" example:"3"`

	AccountID           string  `json:"account_id"`
	AccountLabel        string  `json:"account_label"`
	LoyaltyProgramName  string  `json:"loyalty_program_name"`
	LoyaltyProgramColor string  `json:"loyalty_program_color"`
	AdministratorID     *string `json:"administrator_id"`
	AdministratorName   *string `json:"administrator_name"`

	// ExportCounter is the administrator's supplier-PDF export count, present only on an
	// administrator-grouped row - the number lives on the person, not the account.
	ExportCounter *int `json:"export_counter,omitempty"`

	IssuedPoints   int `json:"issued_points"`
	RefundedPoints int `json:"refunded_points"`
	NetPoints      int `json:"net_points"`

	// Issuances is every line that produced the totals above, in chronological order.
	Issuances []SupplierIssuanceResponse `json:"issuances"`
}

// IncrementSupplierExportCounterRequest bumps an administrator's supplier-PDF export counter by
// one, called just before the download so the number in the file name advances per export.
type IncrementSupplierExportCounterRequest struct {
	AdministratorID string `json:"administrator_id" validate:"required,uuid"`
}

// SetSupplierExportCounterRequest overwrites an administrator's counter with an exact value, for
// correcting it or starting a fresh sequence by hand.
type SetSupplierExportCounterRequest struct {
	AdministratorID string `json:"administrator_id" validate:"required,uuid"`
	Counter         int    `json:"counter" validate:"min=0"`
}

// SupplierExportCounterResponse carries the administrator's counter after the change.
type SupplierExportCounterResponse struct {
	Counter int `json:"counter"`
}

// BulkIncrementSupplierExportCounterRequest bumps several administrators' counters at once, for
// the "all suppliers" PDF that covers many administrators in one document.
type BulkIncrementSupplierExportCounterRequest struct {
	AdministratorIDs []string `json:"administrator_ids" validate:"required,min=1,dive,uuid"`
}

// BulkSupplierExportCounterResponse reports how many administrators were incremented.
type BulkSupplierExportCounterResponse struct {
	Updated int `json:"updated"`
}
