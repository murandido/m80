package schemas

import "time"

// CreatePointBatchRequest defines the expected payload to record one acquisition of points.
//
// ProfitSharePercent is optional: left out, the batch freezes the account's current
// default. Passed, it overrides for this batch only - which is how an arrangement that
// changed mid-month gets recorded without touching anything already settled.
type CreatePointBatchRequest struct {
	AccountID string `json:"account_id" validate:"required,uuid4"`
	// OriginID is required unless Opening is true - an opening balance was carried
	// forward, not bought from anybody.
	OriginID           *string `json:"origin_id" validate:"omitempty,uuid4"`
	AcquiredOn         string  `json:"acquired_on" validate:"required" example:"2026-08-21"`
	Points             int     `json:"points" validate:"required,gt=0" example:"632700"`
	CostCents          int     `json:"cost_cents" validate:"min=0" example:"894172"`
	FundingAgencyID    string  `json:"funding_agency_id" validate:"required,uuid4"`
	ProfitSharePercent *int    `json:"profit_share_percent" validate:"omitempty,min=0,max=100" example:"50"`
	Opening            bool    `json:"opening" example:"false"`
	Note               *string `json:"note"`
}

// UpdatePointBatchRequest defines the payload for updating an existing batch.
// All fields are optional pointers: nil means "don't change this field".
//
// AccountID and Opening are absent on purpose. Moving a batch to another account would
// re-price two accounts' histories at once, and turning an ordinary batch into an opening
// balance would invent a carry-forward that no closing produced; both are a delete plus a
// new entry, where the intent is visible.
type UpdatePointBatchRequest struct {
	OriginID           *string `json:"origin_id" validate:"omitempty,uuid4"`
	AcquiredOn         *string `json:"acquired_on" validate:"omitempty" example:"2026-08-21"`
	Points             *int    `json:"points" validate:"omitempty,gt=0"`
	CostCents          *int    `json:"cost_cents" validate:"omitempty,min=0"`
	FundingAgencyID    *string `json:"funding_agency_id" validate:"omitempty,uuid4"`
	ProfitSharePercent *int    `json:"profit_share_percent" validate:"omitempty,min=0,max=100"`
	Note               *string `json:"note"`
}

// SettlePointBatchRequest marks a batch as settled between the agencies, or clears it.
type SettlePointBatchRequest struct {
	Settled bool `json:"settled" example:"true"`
}

// PointBatchResponse defines how a batch should be returned in the API.
type PointBatchResponse struct {
	ID                 string                    `json:"id"`
	AccountID          string                    `json:"account_id"`
	OriginID           *string                   `json:"origin_id"`
	Origin             *PointBatchOriginResponse `json:"origin,omitempty"`
	AcquiredOn         string                    `json:"acquired_on" example:"2026-08-21"`
	Points             int                       `json:"points" example:"632700"`
	CostCents          int                       `json:"cost_cents" example:"894172"`
	FundingAgencyID    string                    `json:"funding_agency_id"`
	FundingAgency      *AgencyResponse           `json:"funding_agency,omitempty"`
	ProfitSharePercent int                       `json:"profit_share_percent" example:"50"`
	Opening            bool                      `json:"opening"`
	SettledAt          *time.Time                `json:"settled_at"`
	Note               *string                   `json:"note"`
	// CostOfTheThousand is what a thousand of these points cost, in centavos, kept
	// fractional because the rate genuinely is: 894172 over 632700 points is 1413,2638.
	// Display only - the costing never prices a slice by multiplying it.
	CostOfTheThousand float64   `json:"cost_of_the_thousand" example:"1413.2638"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PointBatchListResponse is the GET /point-batches route response.
type PointBatchListResponse struct {
	Success bool                 `json:"success" example:"true"`
	Message string               `json:"message" example:"Lotes listados com sucesso"`
	Data    []PointBatchResponse `json:"data"`
}

// PointBatchSingleResponse is the response from single-batch routes.
type PointBatchSingleResponse struct {
	Success bool               `json:"success" example:"true"`
	Message string             `json:"message" example:"Lote criado com sucesso"`
	Data    PointBatchResponse `json:"data"`
}
