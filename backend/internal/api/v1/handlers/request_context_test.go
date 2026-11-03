package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/events"
)

// The heavy read routes carry the request's context down to Postgres, so a request the
// panel gave up on stops there instead of holding a pooled connection. A request whose
// context is already cancelled is the observable proof: the query fails and the handler
// answers 500, where a handle without the context would have run it and answered 200.
//
// Run against the plain handle, not a transaction: a cancelled query can leave a
// transaction unusable, and nothing here writes anything to roll back.
func TestHeavyReadRoutes_StopWhenTheRequestIsCancelled(t *testing.T) {
	db := setupTestDB(t)

	issuances := NewIssuanceHandler(db, events.NewBroker(), nil)
	sales := NewSaleHandler(db, events.NewBroker())
	// An explicit agency the full scope allows, so resolveAgencyFilter answers without
	// reaching the database and the cancelled listing query is what fails.
	agencyQuery := "?agency_id=" + uuid.NewString()

	cases := []struct {
		name    string
		target  string
		handler http.HandlerFunc
	}{
		{"GetIssuances", "/issuances", issuances.GetIssuances},
		{"GetIssuancesExport", "/issuances/export", issuances.GetIssuancesExport},
		{"GetSalesPage", "/sales/page" + agencyQuery, sales.GetSalesPage},
		{"GetSalesExport", "/sales/export" + agencyQuery, sales.GetSalesExport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := withScope(httptest.NewRequest(http.MethodGet, tc.target, nil).WithContext(ctx), access.FullScope())
			rec := httptest.NewRecorder()

			tc.handler(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status: got %d, want 500 from the cancelled query (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}
