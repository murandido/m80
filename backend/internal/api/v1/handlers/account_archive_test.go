package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// These need a live Postgres for the reason the whole feature exists: what is worth
// asserting is that a foreign key which used to refuse the delete no longer does, and
// there is nothing left of a foreign key once the database is mocked away.

func callAccountDelete(tx *gorm.DB, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/accounts/"+id, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	NewAccountHandler(tx).DeleteAccount(rec, req)
	return rec
}

func callProgramDelete(tx *gorm.DB, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/loyalty-programs/"+id, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	NewLoyaltyProgramHandler(tx).DeleteLoyaltyProgram(rec, req)
	return rec
}

type accountListEnvelope struct {
	Data []schemas.AccountResponse `json:"data"`
}

type programListEnvelope struct {
	Data []schemas.LoyaltyProgramResponse `json:"data"`
}

type issuanceListEnvelope struct {
	Data []schemas.IssuanceResponse `json:"data"`
}

func listIssuances(t *testing.T, tx *gorm.DB) []schemas.IssuanceResponse {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/issuances", nil)
	NewIssuanceHandler(tx, events.NewBroker(), nil).GetIssuances(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /issuances: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var envelope issuanceListEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding issuance list: %v", err)
	}
	return envelope.Data
}

func TestDeleteAccount_ArchivesAnAccountThatAlreadyIssued(t *testing.T) {
	// The bug as reported: the operator deletes every issuance of an account and the
	// account still cannot be deleted, because a soft-deleted issuance keeps holding
	// the RESTRICT foreign key.
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		if err := tx.Delete(&issuance).Error; err != nil {
			t.Fatalf("soft-deleting the issuance: %v", err)
		}

		rec := callAccountDelete(tx, issuance.AccountID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var archived data.Account
		if err := tx.Unscoped().First(&archived, "id = ?", issuance.AccountID).Error; err != nil {
			t.Fatalf("the account row is gone, it should have been archived: %v", err)
		}
		if !archived.DeletedAt.Valid {
			t.Error("the account was not archived")
		}

		// The issuance has to survive intact: archiving an account is not a licence to
		// rewrite what a past issuance was made on.
		var kept data.Issuance
		if err := tx.Unscoped().First(&kept, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("the issuance disappeared with the account: %v", err)
		}
		if kept.AccountID != issuance.AccountID {
			t.Errorf("the issuance's account_id changed to %q", kept.AccountID)
		}
	})
}

func TestDeleteAccount_KeepsTheAccountOnPastIssuances(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)

		if rec := callAccountDelete(tx, issuance.AccountID); rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Gone from the list the pickers are built from...
		listRec := httptest.NewRecorder()
		NewAccountHandler(tx).GetAccounts(listRec, httptest.NewRequest(http.MethodGet, "/accounts", nil))
		var accounts accountListEnvelope
		if err := json.Unmarshal(listRec.Body.Bytes(), &accounts); err != nil {
			t.Fatalf("decoding account list: %v", err)
		}
		for _, account := range accounts.Data {
			if account.ID == issuance.AccountID {
				t.Fatal("the archived account is still listed by GET /accounts")
			}
		}

		// ...but still attached to the issuance it made, and marked as archived so the
		// panel can label the row without offering the account.
		for _, listed := range listIssuances(t, tx) {
			if listed.ID != issuance.ID {
				continue
			}
			if listed.Account == nil {
				t.Fatal("the issuance lost its account: the preload is not Unscoped")
			}
			if listed.Account.DeletedAt == nil {
				t.Error("the embedded account does not say it is archived")
			}
			return
		}
		t.Fatal("the issuance vanished from GET /issuances")
	})
}

func TestDeleteAccount_ArchivesTheAliasesAndFreesTheName(t *testing.T) {
	// The alias is the bot's only route to an account, so one that outlives its
	// account keeps sending new issuances to an account nobody can see - and keeps
	// the name reserved, which is the opposite of what archiving is for.
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)

		alias := data.AccountNickname{
			AccountID:        issuance.AccountID,
			LoyaltyProgramID: issuance.LoyaltyProgramID,
			Nickname:         "heraldo",
		}
		create(t, tx, &alias)

		if rec := callAccountDelete(tx, issuance.AccountID); rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var live int64
		if err := tx.Model(&data.AccountNickname{}).Where("id = ?", alias.ID).Count(&live).Error; err != nil {
			t.Fatalf("counting the alias: %v", err)
		}
		if live != 0 {
			t.Error("the alias outlived the account it belongs to")
		}

		// The freed name goes to a different account of the same program, which is what
		// the partial unique index exists to allow.
		successor := data.Account{LoyaltyProgramID: issuance.LoyaltyProgramID}
		create(t, tx, &successor)

		reused := data.AccountNickname{
			AccountID:        successor.ID,
			LoyaltyProgramID: issuance.LoyaltyProgramID,
			Nickname:         "heraldo",
		}
		if err := tx.Create(&reused).Error; err != nil {
			t.Fatalf("the archived alias still reserves the name: %v", err)
		}
	})
}

func TestDeleteLoyaltyProgram_ArchivesAndFreesTheCode(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)

		var program data.LoyaltyProgram
		if err := tx.First(&program, "id = ?", issuance.LoyaltyProgramID).Error; err != nil {
			t.Fatalf("loading the program: %v", err)
		}

		if rec := callProgramDelete(tx, program.ID); rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		listRec := httptest.NewRecorder()
		NewLoyaltyProgramHandler(tx).GetLoyaltyPrograms(
			listRec, httptest.NewRequest(http.MethodGet, "/loyalty-programs", nil),
		)
		var programs programListEnvelope
		if err := json.Unmarshal(listRec.Body.Bytes(), &programs); err != nil {
			t.Fatalf("decoding program list: %v", err)
		}
		for _, listed := range programs.Data {
			if listed.ID == program.ID {
				t.Fatal("the archived program is still listed by GET /loyalty-programs")
			}
		}

		successor := data.LoyaltyProgram{
			Name:             "Successor",
			Code:             program.Code,
			DefaultAirlineID: program.DefaultAirlineID,
		}
		if err := tx.Create(&successor).Error; err != nil {
			t.Fatalf("the archived program still reserves its code: %v", err)
		}

		// And the issuance still names the program it was made on.
		for _, listed := range listIssuances(t, tx) {
			if listed.ID != issuance.ID {
				continue
			}
			if listed.LoyaltyProgram == nil {
				t.Fatal("the issuance lost its loyalty program: the preload is not Unscoped")
			}
			if listed.LoyaltyProgram.DeletedAt == nil {
				t.Error("the embedded program does not say it is archived")
			}
			return
		}
		t.Fatal("the issuance vanished from GET /issuances")
	})
}
