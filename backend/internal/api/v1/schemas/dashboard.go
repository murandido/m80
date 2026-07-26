package schemas

// DashboardRange echoes back the window and bucket size the aggregation actually used, so
// the frontend labels its axis from the server's answer instead of re-deriving it and
// risking a mismatch when a date was unparseable and silently dropped.
type DashboardRange struct {
	From string `json:"from" example:"2026-07-01"`
	To   string `json:"to" example:"2026-07-31"`
	// Granularity is one of hour, day, week, month.
	Granularity string `json:"granularity" example:"day"`
}

// DashboardTotals are the headline numbers for a window.
//
// AmountCents is a pointer on purpose: it is left nil - and so omitted from the JSON
// entirely - for anyone who is not an admin. Operators are allowed to see the agency's
// volume in miles, but not what it bills. Hiding the field only in the UI would leave the
// number one devtools tab away.
type DashboardTotals struct {
	IssuanceCount int  `json:"issuance_count" example:"128"`
	Points        int  `json:"points" example:"4820000"`
	AmountCents   *int `json:"amount_cents,omitempty" example:"19280000"`

	// Cancellations are counted by when they were cancelled, not when the underlying
	// issuance was created, which is why they are separate from IssuanceCount rather than
	// subtracted from it.
	CancelledCount int `json:"cancelled_count" example:"4"`
	RefundedPoints int `json:"refunded_points" example:"120000"`
}

// DashboardBucket is one point of the time series. Bucket is a pre-formatted local-time
// key whose shape follows the granularity: "2006-01-02T15:00" for hour, "2006-01-02" for
// day and week (the week's Monday), "2006-01" for month.
type DashboardBucket struct {
	Bucket        string `json:"bucket" example:"2026-07-01"`
	IssuanceCount int    `json:"issuance_count" example:"5"`
	Points        int    `json:"points" example:"180000"`
	AmountCents   *int   `json:"amount_cents,omitempty" example:"720000"`
}

// DashboardBreakdown is one slice of a categorical rollup - a loyalty program or an
// issuer. Color carries the loyalty program's own hex when the dimension has one; it stays
// empty for issuers, who have no colour of their own.
type DashboardBreakdown struct {
	ID            string `json:"id"`
	Name          string `json:"name" example:"Smiles"`
	Color         string `json:"color,omitempty" example:"#7C3AED"`
	IssuanceCount int    `json:"issuance_count" example:"42"`
	Points        int    `json:"points" example:"1600000"`
	AmountCents   *int   `json:"amount_cents,omitempty" example:"6400000"`
}

// DashboardResponse is the whole payload behind GET /dashboard: one request feeds every
// panel on the page, so the tiles and the charts can never disagree about the period.
type DashboardResponse struct {
	Range  DashboardRange  `json:"range"`
	Totals DashboardTotals `json:"totals"`
	// PreviousTotals is the same window shifted back by its own length, that feeds the
	// deltas on the stat tiles. Nil when the request was open-ended on either side, since
	// "the previous period" is undefined without both bounds.
	PreviousTotals *DashboardTotals `json:"previous_totals,omitempty"`

	Series    []DashboardBucket    `json:"series"`
	ByProgram []DashboardBreakdown `json:"by_program"`
	ByIssuer  []DashboardBreakdown `json:"by_issuer"`
}
