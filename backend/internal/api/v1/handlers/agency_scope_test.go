package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// withScope stands in for the half of RequireAuth that loads the caller's agency grants.
func withScope(req *http.Request, scope access.AgencyScope) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.AgencyScopeContextKey, scope))
}

// asUser stands in for RequireAuth as a whole: the user plus the full agency scope. Tests that
// are about something other than scoping use it, since without a scope in the context the
// handlers see "no agency" and every agency-bound query comes back empty - by design.
func asUser(ctx context.Context, user data.User) context.Context {
	ctx = context.WithValue(ctx, middleware.UserContextKey, user)
	return context.WithValue(ctx, middleware.AgencyScopeContextKey, access.FullScope())
}

func seedNamedAgency(t *testing.T, tx *gorm.DB, name string) data.Agency {
	t.Helper()
	agency := data.Agency{Name: name + " " + uuid.New().String()[:8]}
	create(t, tx, &agency)
	return agency
}

func TestGetAgencies_OnlyTheCallersScope(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		allowed := seedNamedAgency(t, tx, "Allowed")
		hidden := seedNamedAgency(t, tx, "Hidden")

		list := func(scope access.AgencyScope) map[string]bool {
			req := withScope(httptest.NewRequest(http.MethodGet, "/agencies", nil), scope)
			rec := httptest.NewRecorder()
			NewAgencyHandler(tx).GetAgencies(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Data []schemas.AgencyResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, agency := range body.Data {
				seen[agency.ID] = true
			}
			return seen
		}

		seen := list(access.AgencyScope{IDs: []string{allowed.ID}})
		if !seen[allowed.ID] || seen[hidden.ID] {
			t.Errorf("restricted: want only the allowed agency, saw %v", seen)
		}
		if seen := list(access.AgencyScope{}); len(seen) != 0 {
			t.Errorf("no grants: want nothing, saw %v", seen)
		}
		if seen := list(access.FullScope()); !seen[allowed.ID] || !seen[hidden.ID] {
			t.Errorf("dev: want both agencies, saw %v", seen)
		}
	})
}

func TestGetAgency_OutsideScopeIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		allowed := seedNamedAgency(t, tx, "Allowed")
		hidden := seedNamedAgency(t, tx, "Hidden")
		scope := access.AgencyScope{IDs: []string{allowed.ID}}

		for id, want := range map[string]int{allowed.ID: http.StatusOK, hidden.ID: http.StatusNotFound} {
			req := withScope(withIDs(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"id": id}), scope)
			rec := httptest.NewRecorder()
			NewAgencyHandler(tx).GetAgency(rec, req)
			if rec.Code != want {
				t.Errorf("agency %s: got %d, want %d", id, rec.Code, want)
			}

			req = withScope(withIDs(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"id": id}), scope)
			rec = httptest.NewRecorder()
			NewAgencyNicknameHandler(tx).GetAgencyNicknames(rec, req)
			if rec.Code != want {
				t.Errorf("nicknames of %s: got %d, want %d", id, rec.Code, want)
			}
		}
	})
}
