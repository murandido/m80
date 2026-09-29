package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

func TestGetIssuances_OnlyTheCallersAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		visible, _ := seedIssuance(t, tx)
		hidden, _ := seedIssuance(t, tx)
		scope := access.AgencyScope{IDs: []string{visible.AgencyID}}

		ids := callGetIssuancesAs(t, tx, "", scope)
		if !ids[visible.ID] || ids[hidden.ID] {
			t.Errorf("list: want only the in-scope issuance, got %v", ids)
		}
		// The search returns early from the filter builder; it must still be scoped.
		if ids := callGetIssuancesAs(t, tx, "q="+hidden.ProgramLocator, scope); ids[hidden.ID] {
			t.Error("search reached an issuance outside the caller's agencies")
		}
		// Asking for the hidden agency by id must not widen anything either.
		if ids := callGetIssuancesAs(t, tx, "agency_id="+hidden.AgencyID, scope); len(ids) != 0 {
			t.Errorf("filtering by an out-of-scope agency returned %v", ids)
		}
		if ids := callGetIssuancesAs(t, tx, "", access.AgencyScope{}); len(ids) != 0 {
			t.Errorf("no grants: want nothing, got %v", ids)
		}
	})
}

// The by-id routes are guarded by a middleware on the route group, so the test goes through
// a router carrying it rather than calling the handler.
func TestIssuanceByID_OutsideScopeIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		visible, user := seedIssuance(t, tx)
		hidden, _ := seedIssuance(t, tx)
		scope := access.AgencyScope{IDs: []string{visible.AgencyID}}

		handler := NewIssuanceHandler(tx, events.NewBroker(), nil)
		router := chi.NewRouter()
		router.Route("/issuances/{id}", func(r chi.Router) {
			r.Use(middleware.RequireIssuanceInScope(tx))
			r.Get("/", handler.GetIssuance)
			r.Post("/cancel", handler.CancelIssuance)
		})

		call := func(method, path string) int {
			req := httptest.NewRequest(method, path, strings.NewReader("{}"))
			req = req.WithContext(asUser(req.Context(), data.User{ID: user.ID, Role: data.RoleAdmin}))
			req = withScope(req, scope)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			return rec.Code
		}

		if code := call(http.MethodGet, "/issuances/"+visible.ID); code != http.StatusOK {
			t.Errorf("in scope: got %d, want 200", code)
		}
		if code := call(http.MethodGet, "/issuances/"+hidden.ID); code != http.StatusNotFound {
			t.Errorf("out of scope: got %d, want 404", code)
		}
		if code := call(http.MethodPost, "/issuances/"+hidden.ID+"/cancel"); code != http.StatusNotFound {
			t.Errorf("cancel out of scope: got %d, want 404", code)
		}
		var still data.Issuance
		if err := tx.First(&still, "id = ?", hidden.ID).Error; err != nil || !still.Active {
			t.Errorf("the out-of-scope issuance was touched: active=%v err=%v", still.Active, err)
		}
		// A malformed id still reaches the handler, which owns that answer.
		if code := call(http.MethodGet, "/issuances/not-a-uuid"); code == http.StatusServiceUnavailable {
			t.Error("a malformed id was treated as an outage")
		}
	})
}

func TestCreateIssuance_StaysInsideTheCallersAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		seed, user := seedIssuance(t, tx)
		defaultAgency(t, tx)
		mine := seedNamedAgency(t, tx, "Mine")
		other := seedNamedAgency(t, tx, "Other")
		scope := access.AgencyScope{IDs: []string{mine.ID}}

		create := func(agencyField string) *httptest.ResponseRecorder {
			body := `{
				"program_locator": "LOC` + uuid.New().String()[:6] + `",
				"loyalty_program_id": "` + seed.LoyaltyProgramID + `",
				"account_id": "` + seed.AccountID + `",
				"points": 10000,
				"value_of_the_thousand": 2000` + agencyField + `
			}`
			req := httptest.NewRequest(http.MethodPost, "/issuances", strings.NewReader(body))
			req = withScope(req.WithContext(asUser(req.Context(), data.User{ID: user.ID, Role: data.RoleAdmin})), scope)
			rec := httptest.NewRecorder()
			NewIssuanceHandler(tx, events.NewBroker(), nil).CreateIssuance(rec, req)
			return rec
		}

		if rec := create(`, "agency_id": "` + other.ID + `"`); rec.Code != http.StatusNotFound {
			t.Errorf("foreign agency: got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}

		// No agency named and the main agency out of scope: it lands on the caller's own.
		rec := create("")
		if rec.Code != http.StatusCreated {
			t.Fatalf("no agency: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}
		var resp cancelResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Data.AgencyID != mine.ID {
			t.Errorf("no agency: landed on %s, want the caller's %s", resp.Data.AgencyID, mine.ID)
		}
	})
}

func TestUpdateIssuance_CannotMoveToAForeignAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		other := seedNamedAgency(t, tx, "Other")
		scope := access.AgencyScope{IDs: []string{issuance.AgencyID}}

		req := httptest.NewRequest(http.MethodPatch, "/issuances/"+issuance.ID,
			strings.NewReader(`{"agency_id": "`+other.ID+`"}`))
		req = withIDs(req, map[string]string{"id": issuance.ID})
		req = withScope(req.WithContext(asUser(req.Context(), data.User{ID: user.ID, Role: data.RoleAdmin})), scope)
		rec := httptest.NewRecorder()
		NewIssuanceHandler(tx, events.NewBroker(), nil).UpdateIssuance(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
		var after data.Issuance
		if err := tx.First(&after, "id = ?", issuance.ID).Error; err != nil || after.AgencyID != issuance.AgencyID {
			t.Errorf("the issuance moved: agency=%s err=%v", after.AgencyID, err)
		}
	})
}

func TestLegacyImport_RefusesAgenciesOutsideTheScope(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		user := createUser(t, tx, "Importer")
		mine := seedNamedAgency(t, tx, "Mine")
		other := seedNamedAgency(t, tx, "Other")
		scope := access.AgencyScope{IDs: []string{mine.ID}}
		preview := (*LegacyImportHandler).PreviewLegacyImport

		for name, mappings := range map[string]string{
			"day agency": `{"agency_id": "` + other.ID + `"}`,
			"row agency": `{"agency_id": "` + mine.ID + `", "row_agencies": [{"block_index": 0, "number": 1, "agency_id": "` + other.ID + `"}]}`,
		} {
			rec := postLegacyImportAs(t, tx, preview, user.ID, []byte("irrelevant"),
				map[string]string{"mappings": mappings}, scope)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: got %d, want 404 (%s)", name, rec.Code, rec.Body.String())
			}
		}
	})
}
