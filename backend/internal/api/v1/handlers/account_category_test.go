package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// What the panel actually receives. The rules themselves are covered in
// account_beneficiaries_test.go; this is about the wire: the counter reaching the card, the
// category filter cutting the list, and the CPF staying out of every response.

type beneficiaryListEnvelope struct {
	Data []schemas.AccountBeneficiaryResponse `json:"data"`
}

func callBeneficiaries(tx *gorm.DB, accountID string) *httptest.ResponseRecorder {
	handler := NewAccountBeneficiaryHandler(tx)
	req := httptest.NewRequest(http.MethodGet, "/accounts/"+accountID+"/beneficiaries", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", accountID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	handler.GetAccountBeneficiaries(rec, req)
	return rec
}

func callCreateBeneficiary(tx *gorm.DB, accountID, body string) *httptest.ResponseRecorder {
	handler := NewAccountBeneficiaryHandler(tx)
	req := httptest.NewRequest(http.MethodPost, "/accounts/"+accountID+"/beneficiaries",
		strings.NewReader(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", accountID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	rec := httptest.NewRecorder()
	handler.CreateAccountBeneficiary(rec, req)
	return rec
}

func TestAccountListCarriesTheCategoryAndTheUsage(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()
		issuance := world.issue(t, tx, now)
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		accounts := callGetAccounts(t, tx, "loyalty_program_id="+world.program.ID)
		if len(accounts) != 1 {
			t.Fatalf("got %d accounts, want 1", len(accounts))
		}

		account := accounts[0]
		if account.Category != data.AccountCategoryStandard {
			t.Errorf("category = %q, want %q", account.Category, data.AccountCategoryStandard)
		}
		if account.CPFUsage == nil {
			t.Fatal("cpf_usage is null on a standard account with a capped program")
		}
		if account.CPFUsage.Used != 1 || account.CPFUsage.Limit != 25 || account.CPFUsage.Remaining != 24 {
			t.Errorf("cpf_usage = %+v, want 1/25 with 24 remaining", *account.CPFUsage)
		}
		if account.CPFUsage.ResetsAt == nil {
			t.Error("a calendar-year cap should say when it resets")
		}
	})
}

func TestAccountListFiltersByCategory(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)

		// A second account on the same program, under a court injunction.
		injunction := data.Account{
			HolderID:         testHolder(t, tx),
			LoyaltyProgramID: world.program.ID,
			Category:         data.AccountCategoryInjunction,
		}
		create(t, tx, &injunction)

		for _, tc := range []struct {
			category string
			want     int
		}{
			{data.AccountCategoryStandard, 1},
			{data.AccountCategoryInjunction, 1},
			{data.AccountCategoryUnidentified, 0},
			{"", 2},
		} {
			query := "loyalty_program_id=" + world.program.ID
			if tc.category != "" {
				query += "&category=" + tc.category
			}

			accounts := callGetAccounts(t, tx, query)
			if len(accounts) != tc.want {
				t.Errorf("category %q returned %d accounts, want %d", tc.category, len(accounts), tc.want)
			}
		}

		// The injunction account has no cap to report, so it carries no counter - which is
		// not the same as a counter reading zero.
		underInjunction := callGetAccounts(t, tx,
			"category="+data.AccountCategoryInjunction+"&loyalty_program_id="+world.program.ID)
		if underInjunction[0].CPFUsage != nil {
			t.Error("an account under an injunction should carry no cpf_usage")
		}
	})
}

func TestBeneficiaryResponseNeverCarriesTheCPF(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()
		issuance := world.issue(t, tx, now)
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		rec := callBeneficiaries(tx, world.account.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		// The number is not in the database, so it cannot be in the response - but the
		// fingerprint could leak by accident, and that is worth asserting on the raw body.
		body := rec.Body.String()
		for _, forbidden := range []string{"52998224725", "529.982.247-25", "cpf_fingerprint"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("the response leaked %q: %s", forbidden, body)
			}
		}

		var envelope beneficiaryListEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("failed to decode the response: %v", err)
		}
		if len(envelope.Data) != 1 {
			t.Fatalf("got %d beneficiaries, want 1", len(envelope.Data))
		}
		beneficiary := envelope.Data[0]
		if !beneficiary.HasCPF {
			t.Error("has_cpf = false on a beneficiary registered with a document")
		}
		if beneficiary.CPFMasked == nil || *beneficiary.CPFMasked != "***.982.247-**" {
			t.Errorf("cpf_masked = %v, want ***.982.247-**", beneficiary.CPFMasked)
		}
		if beneficiary.Uses != 1 || beneficiary.LastUsedAt == nil {
			t.Errorf("uses = %d, last_used_at = %v, want 1 and a date", beneficiary.Uses, beneficiary.LastUsedAt)
		}
	})
}

func TestAddingABeneficiaryByHandConsumesASlot(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)

		rec := callCreateBeneficiary(tx, world.account.ID,
			fmt.Sprintf(`{"name":"Alice Ferreira","cpf":%q}`, cpfAlice))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
		}
		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("used = %d, want 1", used)
		}

		// A document that fails the check digits is refused rather than quietly filed under
		// the name: someone typing a CPF is asserting something.
		rec = callCreateBeneficiary(tx, world.account.ID, `{"name":"Bruno","cpf":"111.111.111-11"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("a placeholder CPF got status %d, want 400", rec.Code)
		}
	})
}

func TestMergingTwoBeneficiariesKeepsTheIssuances(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 24, data.CPFLimitWindowRolling12Months, data.CPFLimitModePerIssuance)
		now := time.Now()

		// The same person entered twice under two spellings - what the name match does when
		// the bot recorded a surname and the panel a full name.
		first := world.issue(t, tx, now)
		register(t, tx, world.account.ID, first.ID, "MACHADO", nil, now)
		second := world.issue(t, tx, now)
		register(t, tx, world.account.ID, second.ID, "Jose Machado", nil, now)

		if used := world.usage(t, tx).Used; used != 2 {
			t.Fatalf("before the merge, used = %d, want 2", used)
		}

		var beneficiaries []data.AccountBeneficiary
		if err := tx.Where("account_id = ?", world.account.ID).
			Order("name").Find(&beneficiaries).Error; err != nil {
			t.Fatalf("failed to read the beneficiaries: %v", err)
		}
		if len(beneficiaries) != 2 {
			t.Fatalf("got %d beneficiaries, want 2", len(beneficiaries))
		}

		if err := acct.Merge(tx, world.account.ID, beneficiaries[0].ID, beneficiaries[1].ID); err != nil {
			t.Fatalf("Merge returned an error: %v", err)
		}

		// One person now - but both issuances still happened, and this program charges per
		// issuance, so the count must not drop.
		var remaining int64
		if err := tx.Model(&data.AccountBeneficiary{}).
			Where("account_id = ?", world.account.ID).Count(&remaining).Error; err != nil {
			t.Fatalf("failed to count the beneficiaries: %v", err)
		}
		if remaining != 1 {
			t.Errorf("got %d beneficiaries after the merge, want 1", remaining)
		}
		if used := world.usage(t, tx).Used; used != 2 {
			t.Errorf("after the merge, used = %d, want 2: the issuances were lost", used)
		}
	})
}

func TestReconcileRebuildsFromTheIssuancesAndIsIdempotent(t *testing.T) {

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		issuance := world.issue(t, tx, now)
		if err := tx.Model(&issuance).Update("main_passenger_name", "Alice Ferreira").Error; err != nil {
			t.Fatalf("failed to name the passenger: %v", err)
		}
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", nil, now)

		// Something typed in by hand that the issuances do not know about. Reconcile is the
		// escape hatch, and it is explicitly allowed to throw this away.
		if err := acct.RegisterManual(tx, world.account.ID, "Invented Person", nil, now); err != nil {
			t.Fatalf("RegisterManual returned an error: %v", err)
		}
		if used := world.usage(t, tx).Used; used != 2 {
			t.Fatalf("before the reconcile, used = %d, want 2", used)
		}

		for attempt := range 2 {
			if err := acct.Reconcile(tx, world.account.ID); err != nil {
				t.Fatalf("Reconcile returned an error: %v", err)
			}
			if used := world.usage(t, tx).Used; used != 1 {
				t.Fatalf("after reconcile %d, used = %d, want 1", attempt+1, used)
			}
		}
	})
}
