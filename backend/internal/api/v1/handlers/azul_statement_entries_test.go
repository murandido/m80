package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// seedAzulStatementTarget creates an account that owns its inventory plus the cast an issuance
// needs, for the statement-page routes.
func seedAzulStatementTarget(t *testing.T, tx *gorm.DB) (data.Account, data.User) {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air", Code: "T" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Test Program", Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	agency := data.Agency{Name: "Test Agency " + suffix}
	create(t, tx, &agency)
	user := createUser(t, tx, "Operador "+suffix)
	account := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID, OwnsPointInventory: true, TracksStatement: true}
	create(t, tx, &account)

	return account, user
}

func seedAzulStatementIssuance(t *testing.T, tx *gorm.DB, account data.Account, user data.User, locator string) data.Issuance {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Air " + suffix, Code: "A" + suffix}
	create(t, tx, &airline)
	agency := data.Agency{Name: "Ag " + suffix}
	create(t, tx, &agency)

	passenger := "MARIA SILVA"
	// Origin/destination IATA codes are left nil: they carry an FK to airports, which these
	// tests do not seed, and the resolution under test does not depend on them.
	issuance := data.Issuance{
		LoyaltyProgramID:   account.LoyaltyProgramID,
		AirlineID:          airline.ID,
		ProgramLocator:     locator,
		Points:             25000,
		ValueOfTheThousand: 1500,
		AccountID:          account.ID,
		IssuerID:           user.ID,
		AgencyID:           agency.ID,
		MainPassengerName:  &passenger,
	}
	create(t, tx, &issuance)
	return issuance
}

func seedAzulLine(t *testing.T, tx *gorm.DB, accountID, occurredOn, kind string, points int, locator *string) {
	t.Helper()
	line := data.AzulStatementLine{
		AccountID:  accountID,
		OccurredOn: day(t, occurredOn),
		Kind:       kind,
		Points:     points,
		Locator:    locator,
	}
	create(t, tx, &line)
}

func azulStatementRequest(method, accountID, userID, body string) *http.Request {
	req := httptest.NewRequest(method, "/accounts/"+accountID+"/azul-statement", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", accountID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: data.RoleAdmin})
	return req.WithContext(ctx)
}

// The statement page's read: the synced lines come back in day order, and a redemption whose
// locator exists on the account is resolved to that issuance.
func TestAzulStatementEntriesResolvesTheLinkedIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedAzulStatementTarget(t, tx)
		issuance := seedAzulStatementIssuance(t, tx, account, user, "MJF6HC")

		locator := "MJF6HC"
		seedAzulLine(t, tx, account.ID, "2026-08-20", data.AzulLineAccrual, 5000, nil)
		seedAzulLine(t, tx, account.ID, "2026-09-10", data.AzulLineRedemption, -25000, &locator)

		handler := NewStatementHandler(tx)
		rec := httptest.NewRecorder()
		handler.AzulStatementEntries(rec, azulStatementRequest(http.MethodGet, account.ID, user.ID, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var response struct {
			Data struct {
				From  string `json:"from"`
				To    string `json:"to"`
				Lines []struct {
					Kind           string `json:"kind"`
					Locator        string `json:"locator"`
					LinkedIssuance *struct {
						IssuanceID string `json:"issuance_id"`
						Manual     bool   `json:"manual"`
					} `json:"linked_issuance"`
				} `json:"lines"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decoding response: %v", err)
		}
		if len(response.Data.Lines) != 2 {
			t.Fatalf("got %d lines, want 2", len(response.Data.Lines))
		}
		if response.Data.From != "2026-08-20" || response.Data.To != "2026-09-10" {
			t.Errorf("window = %s..%s, want 2026-08-20..2026-09-10", response.Data.From, response.Data.To)
		}
		if response.Data.Lines[0].LinkedIssuance != nil {
			t.Error("the accrual line (no locator) should not be linked")
		}
		redemption := response.Data.Lines[1]
		if redemption.LinkedIssuance == nil {
			t.Fatal("the redemption line was not linked to its issuance")
		}
		if redemption.LinkedIssuance.IssuanceID != issuance.ID {
			t.Errorf("linked issuance = %q, want %q", redemption.LinkedIssuance.IssuanceID, issuance.ID)
		}
		if redemption.LinkedIssuance.Manual {
			t.Error("an auto match reported manual = true")
		}
	})
}

// The edit-mode round trip: pin a locator to an issuance, then remove the pin.
func TestAzulBookingLinkAndUnlink(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedAzulStatementTarget(t, tx)
		// Two issuances: the auto match for "LOC" and the one the operator pins instead.
		auto := seedAzulStatementIssuance(t, tx, account, user, "LOC")
		pinned := seedAzulStatementIssuance(t, tx, account, user, "OTHER")

		handler := NewStatementHandler(tx)

		// Pin LOC to the "OTHER" issuance.
		rec := httptest.NewRecorder()
		body := `{"locator":"LOC","issuance_id":"` + pinned.ID + `"}`
		handler.LinkStatementBooking(rec, azulStatementRequest(http.MethodPost, account.ID, user.ID, body))
		if rec.Code != http.StatusOK {
			t.Fatalf("link status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var linkResp struct {
			Data struct {
				LinkedIssuance *struct {
					IssuanceID string `json:"issuance_id"`
					Manual     bool   `json:"manual"`
				} `json:"linked_issuance"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &linkResp); err != nil {
			t.Fatalf("decoding link response: %v", err)
		}
		if linkResp.Data.LinkedIssuance == nil || linkResp.Data.LinkedIssuance.IssuanceID != pinned.ID || !linkResp.Data.LinkedIssuance.Manual {
			t.Fatalf("after link = %+v, want the pinned %q manual=true", linkResp.Data.LinkedIssuance, pinned.ID)
		}

		// Unlink: LOC falls back to its automatic match.
		rec = httptest.NewRecorder()
		handler.UnlinkStatementBooking(rec, azulStatementRequest(http.MethodDelete, account.ID, user.ID, `{"locator":"LOC"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("unlink status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var unlinkResp struct {
			Data struct {
				LinkedIssuance *struct {
					IssuanceID string `json:"issuance_id"`
					Manual     bool   `json:"manual"`
				} `json:"linked_issuance"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &unlinkResp); err != nil {
			t.Fatalf("decoding unlink response: %v", err)
		}
		if unlinkResp.Data.LinkedIssuance == nil || unlinkResp.Data.LinkedIssuance.IssuanceID != auto.ID || unlinkResp.Data.LinkedIssuance.Manual {
			t.Fatalf("after unlink = %+v, want the auto %q manual=false", unlinkResp.Data.LinkedIssuance, auto.ID)
		}
	})
}

// Pinning to an issuance on another account is refused - a locator cannot cross accounts.
func TestAzulBookingLinkRejectsForeignIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedAzulStatementTarget(t, tx)
		other, otherUser := seedAzulStatementTarget(t, tx)
		foreign := seedAzulStatementIssuance(t, tx, other, otherUser, "FOREIGN")

		handler := NewStatementHandler(tx)
		rec := httptest.NewRecorder()
		body := `{"locator":"LOC","issuance_id":"` + foreign.ID + `"}`
		handler.LinkStatementBooking(rec, azulStatementRequest(http.MethodPost, account.ID, user.ID, body))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

// The CSV follows the page's date filter: only the lines of the chosen days, both bounds
// inclusive, and the period in the file name. Without a filter it is everything, as before.
func TestExportAzulStatementCoversOnlyThePeriod(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, user := seedAzulStatementTarget(t, tx)
		seedAzulLine(t, tx, account.ID, "2026-09-01", data.AzulLineAccrual, 1000, nil)
		seedAzulLine(t, tx, account.ID, "2026-09-10", data.AzulLineAccrual, 2000, nil)
		seedAzulLine(t, tx, account.ID, "2026-09-20", data.AzulLineAccrual, 3000, nil)
		seedAzulLine(t, tx, account.ID, "2026-09-30", data.AzulLineAccrual, 4000, nil)

		handler := NewStatementHandler(tx)
		export := func(query string) *httptest.ResponseRecorder {
			req := azulStatementRequest(http.MethodGet, account.ID, user.ID, "")
			req.URL.RawQuery = query
			rec := httptest.NewRecorder()
			handler.ExportAzulStatement(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}
			return rec
		}
		dataRows := func(rec *httptest.ResponseRecorder) int {
			// One header row; the BOM sits on it.
			return strings.Count(strings.TrimSpace(rec.Body.String()), "\n")
		}

		rec := export("date_from=2026-09-10&date_to=2026-09-20")
		if got := dataRows(rec); got != 2 {
			t.Errorf("filtered export has %d rows, want 2 (the 10th and the 20th, inclusive)", got)
		}
		if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "-2026-09-10_2026-09-20.csv") {
			t.Errorf("file name = %q, want the period in it", disposition)
		}

		if got := dataRows(export("date_from=2026-09-15")); got != 2 {
			t.Errorf("open-ended export has %d rows, want 2 (the 20th and the 30th)", got)
		}
		if got := dataRows(export("")); got != 4 {
			t.Errorf("unfiltered export has %d rows, want all 4", got)
		}
		if got := dataRows(export("date_from=not-a-day")); got != 4 {
			t.Errorf("an unparseable bound gave %d rows, want it ignored (all 4)", got)
		}
	})
}
