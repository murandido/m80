package schemas

// StatementBookingLine compares one booking between the statement and the panel.
type StatementBookingLine struct {
	Locator string `json:"locator" example:"MJF6HC"`
	// Verdict is matched, missing_in_panel, missing_in_statement or amount_differs.
	Verdict string `json:"verdict" example:"amount_differs"`
	// StatementPoints is what left the account according to the airline, positive, with
	// the legs of one booking already summed.
	StatementPoints int    `json:"statement_points" example:"360000"`
	PanelPoints     int    `json:"panel_points" example:"350000"`
	IssuanceID      string `json:"issuance_id"`
	OccurredOn      string `json:"occurred_on" example:"2026-08-05"`
}

// StatementRefundLine compares one booking's refund.
type StatementRefundLine struct {
	Locator         string `json:"locator" example:"YW51QG"`
	Verdict         string `json:"verdict" example:"amount_differs"`
	StatementPoints int    `json:"statement_points" example:"40000"`
	PanelPoints     int    `json:"panel_points" example:"0"`
	IssuanceID      string `json:"issuance_id"`
}

// StatementCreditLine is one credit on the statement, paired with a batch or not.
//
// A credit with verdict missing_in_panel is stock that arrived and was never priced: the
// costing is spending it at whatever the neighbouring batch cost. It is the line this
// screen exists to surface, and the only one the commit route acts on.
type StatementCreditLine struct {
	// Index identifies the credit within this statement, and is what the commit route
	// takes. The client never sends the credit's contents back - the file is read and
	// reconciled again from scratch, so the preview is advice, not contract.
	Index       int    `json:"index" example:"0"`
	OccurredOn  string `json:"occurred_on" example:"2026-08-27"`
	Points      int    `json:"points" example:"158175"`
	Description string `json:"description" example:"Resgate pontos Inter"`
	Verdict     string `json:"verdict" example:"missing_in_panel"`
	BatchID     string `json:"batch_id"`
}

// StatementReconciliationResponse is the whole comparison.
type StatementReconciliationResponse struct {
	// AccountNumber is the programme's own number for the account, when the file states
	// it. Advisory: it lets the screen warn that the file may be from another account.
	AccountNumber string `json:"account_number" example:"03303567123"`
	// From and To are the statement's own span, not a window anybody chose.
	From string `json:"from" example:"2026-07-22"`
	To   string `json:"to" example:"2026-09-07"`

	Bookings []StatementBookingLine `json:"bookings"`
	Refunds  []StatementRefundLine  `json:"refunds"`
	Credits  []StatementCreditLine  `json:"credits"`

	// StatementNetPoints and PanelNetPoints are the change in balance each side claims.
	// Neither export states a balance, so agreeing on the change is the only check there
	// is - and disagreeing means something above was missed.
	StatementNetPoints int `json:"statement_net_points" example:"-158800"`
	PanelNetPoints     int `json:"panel_net_points" example:"-158800"`
}

// StatementReconciliationSingleResponse is the preview route's envelope.
type StatementReconciliationSingleResponse struct {
	Success bool                            `json:"success" example:"true"`
	Message string                          `json:"message" example:"Extrato conciliado"`
	Data    StatementReconciliationResponse `json:"data"`
}

// StatementBatchSelection is one credit the operator chose to record, with the two things
// the statement cannot know: what it cost and where it came from.
type StatementBatchSelection struct {
	Index    int    `json:"index" example:"0"`
	OriginID string `json:"origin_id" validate:"required,uuid4"`
	// CostCents is what this credit cost. Zero is a real answer - a club subscription and
	// flight accrual both arrive for nothing.
	CostCents int `json:"cost_cents" validate:"min=0" example:"224775"`
	// FundingAgencyID is the agency that paid for it.
	FundingAgencyID string `json:"funding_agency_id" validate:"required,uuid4"`
	// Note travels onto the batch, for the operator's own labelling.
	Note string `json:"note"`
}

// StatementCommitResponse says what the commit wrote.
type StatementCommitResponse struct {
	// Created counts batches, not credits: credits of the same origin and payer are summed
	// into one batch, so four instalments of the same purchase come back as one.
	Created int `json:"created" example:"1"`
	// Credits counts how many statement lines went into them, which is what makes a commit
	// of twenty accruals into one batch readable instead of alarming.
	Credits int `json:"credits" example:"4"`
	// Skipped counts selections the reconciliation no longer reports as missing - a batch
	// entered between the preview and the commit, or the same file submitted twice.
	Skipped int `json:"skipped" example:"0"`
}

// StatementCommitSingleResponse is the commit route's envelope.
type StatementCommitSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Lotes lançados a partir do extrato"`
	Data    StatementCommitResponse `json:"data"`
}
