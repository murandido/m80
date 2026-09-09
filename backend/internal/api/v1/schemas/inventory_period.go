package schemas

import "time"

// ClosePeriodRequest settles a stretch of an account's costing.
type ClosePeriodRequest struct {
	AccountID string `json:"account_id" validate:"required,uuid4"`
	StartsOn  string `json:"starts_on" validate:"required" example:"2026-08-01"`
	EndsOn    string `json:"ends_on" validate:"required" example:"2026-08-31"`
}

// InventoryPeriodResponse defines how a closed period is returned in the API.
type InventoryPeriodResponse struct {
	ID         string     `json:"id"`
	AccountID  string     `json:"account_id"`
	StartsOn   string     `json:"starts_on" example:"2026-08-01"`
	EndsOn     string     `json:"ends_on" example:"2026-08-31"`
	ClosedAt   *time.Time `json:"closed_at"`
	ClosedByID *string    `json:"closed_by_id"`
	CreatedAt  time.Time  `json:"created_at"`
}

// InventoryPeriodListResponse is the GET /inventory-periods route response.
type InventoryPeriodListResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Períodos listados com sucesso"`
	Data    []InventoryPeriodResponse `json:"data"`
}

// InventoryPeriodSingleResponse is the response from single-period routes.
type InventoryPeriodSingleResponse struct {
	Success bool                    `json:"success" example:"true"`
	Message string                  `json:"message" example:"Período fechado com sucesso"`
	Data    InventoryPeriodResponse `json:"data"`
}
