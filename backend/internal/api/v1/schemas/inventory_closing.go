package schemas

// InventoryConsumerResponse is one agency's activity against a batch.
type InventoryConsumerResponse struct {
	AgencyID     string  `json:"agency_id"`
	AgencyName   *string `json:"agency_name"`
	Points       int     `json:"points"`
	CostCents    int     `json:"cost_cents"`
	RevenueCents int     `json:"revenue_cents"`
	ProfitCents  int     `json:"profit_cents"`
}

// InventorySettlementResponse is one agency owing another. A negative amount is the debt
// running the other way.
type InventorySettlementResponse struct {
	AgencyID     string  `json:"agency_id"`
	AgencyName   *string `json:"agency_name"`
	ToAgencyID   string  `json:"to_agency_id"`
	ToAgencyName *string `json:"to_agency_name"`
	AmountCents  int     `json:"amount_cents" example:"360640"`
}

// InventoryBatchResultResponse is one batch's line in the closing report.
type InventoryBatchResultResponse struct {
	BatchID            string  `json:"batch_id"`
	OriginName         *string `json:"origin_name" example:"Livelo"`
	AcquiredOn         string  `json:"acquired_on" example:"2026-08-05"`
	FundingAgencyID    string  `json:"funding_agency_id"`
	FundingAgencyName  *string `json:"funding_agency_name"`
	ProfitSharePercent int     `json:"profit_share_percent" example:"50"`
	SettledAt          *string `json:"settled_at"`

	Points    int `json:"points"`
	CostCents int `json:"cost_cents"`

	ConsumedPoints  int `json:"consumed_points"`
	ChargedCents    int `json:"charged_cents"`
	RevenueCents    int `json:"revenue_cents"`
	ProfitCents     int `json:"profit_cents"`
	RemainingPoints int `json:"remaining_points"`

	Consumers   []InventoryConsumerResponse   `json:"consumers"`
	Settlements []InventorySettlementResponse `json:"settlements"`
}

// InventoryClosingResponse is one account's closing report.
type InventoryClosingResponse struct {
	AccountID    string  `json:"account_id"`
	AccountLabel *string `json:"account_label"`
	StartsOn     string  `json:"starts_on" example:"2026-08-01"`
	EndsOn       string  `json:"ends_on" example:"2026-08-31"`
	Closed       bool    `json:"closed"`
	// ClosedPeriodID names the closing that froze this range, so the screen can offer to
	// reopen it. Null when the range is open.
	ClosedPeriodID *string `json:"closed_period_id"`
	// UnsupportedSplit warns that this account has more than two partner agencies, which
	// the single frozen percent cannot express - the settlement figures below are not
	// reliable, and the screen says so rather than showing them as fact.
	UnsupportedSplit bool `json:"unsupported_split"`

	Batches []InventoryBatchResultResponse `json:"batches"`
	// PendingPoints is stock that was issued before any batch covering it was entered.
	// It resolves itself once the missing purchase is recorded.
	PendingPoints int `json:"pending_points"`

	CostCents    int                           `json:"cost_cents"`
	RevenueCents int                           `json:"revenue_cents"`
	ProfitCents  int                           `json:"profit_cents"`
	Settlements  []InventorySettlementResponse `json:"settlements"`
}

// InventoryClosingListResponse is the GET /inventory-closing route response.
//
// It is a list because a holder can own several accounts - one per programme - and the
// settlement is paid on their sum. The report shows each account separately and the total
// beneath, which is what the spreadsheet could not do: its AZUL and SMILES tabs each
// carried their own total, and adding them up was done by hand.
type InventoryClosingListResponse struct {
	Success bool                         `json:"success" example:"true"`
	Message string                       `json:"message" example:"Fechamento calculado com sucesso"`
	Data    InventoryClosingDataResponse `json:"data"`
}

// InventoryClosingDataResponse carries the per-account reports and their consolidation.
type InventoryClosingDataResponse struct {
	Accounts []InventoryClosingResponse `json:"accounts"`
	// Totals is every account added together: the figure actually transferred.
	Totals InventoryClosingTotalsResponse `json:"totals"`
}

// InventoryClosingTotalsResponse is the consolidation across accounts.
type InventoryClosingTotalsResponse struct {
	CostCents     int                           `json:"cost_cents"`
	RevenueCents  int                           `json:"revenue_cents"`
	ProfitCents   int                           `json:"profit_cents"`
	PendingPoints int                           `json:"pending_points"`
	Settlements   []InventorySettlementResponse `json:"settlements"`
}
