package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

// callPatchIssuance drives UpdateIssuance directly, the same way callCancel does.
func callPatchIssuance(tx *gorm.DB, id, userID, body string) *httptest.ResponseRecorder {
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)

	req := httptest.NewRequest(http.MethodPatch, "/issuances/"+id, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.UpdateIssuance(rec, req)
	return rec
}

// seedComputableIssuance is an issuance the automatic rule will actually act on: synced, on
// an exempt account, with one domestic leg inside the window and its program's fee configured.
func seedComputableIssuance(t *testing.T, tx *gorm.DB) (data.Issuance, data.User) {
	t.Helper()

	issuance, user := seedIssuance(t, tx)

	if err := tx.Model(&data.Account{}).Where("id = ?", issuance.AccountID).
		UpdateColumn("waives_redemption_fee", true).Error; err != nil {
		t.Fatalf("failed to mark the account exempt: %v", err)
	}

	domestic, international, window := 6990, 17490, 120
	err := tx.Model(&data.LoyaltyProgram{}).Where("id = ?", issuance.LoyaltyProgramID).
		UpdateColumns(map[string]any{
			"redemption_fee_domestic_cents":      domestic,
			"redemption_fee_international_cents": international,
			"redemption_fee_advance_days":        window,
		}).Error
	if err != nil {
		t.Fatalf("failed to configure the program fee: %v", err)
	}

	flight := data.IssuanceFlight{
		IssuanceID:  issuance.ID,
		Direction:   "outbound",
		LegOrder:    1,
		DepartureAt: time.Now().AddDate(0, 0, 30),
	}
	create(t, tx, &flight)

	err = tx.Model(&data.Issuance{}).Where("id = ?", issuance.ID).
		UpdateColumn("traveos_synced_at", time.Now()).Error
	if err != nil {
		t.Fatalf("failed to mark the issuance as synced: %v", err)
	}

	return issuance, user
}

func readPersistedFee(t *testing.T, tx *gorm.DB, id string) data.Issuance {
	t.Helper()
	var persisted data.Issuance
	if err := tx.First(&persisted, "id = ?", id).Error; err != nil {
		t.Fatalf("failed to re-read the issuance: %v", err)
	}
	return persisted
}

func TestPatchIssuance_RedemptionFeeValuePinsTheCorrection(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedComputableIssuance(t, tx)

		rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"redemption_fee_saved_cents":12345}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		persisted := readPersistedFee(t, tx, issuance.ID)
		if !persisted.RedemptionFeeManual {
			t.Error("expected the manual flag to be set by typing an amount")
		}
		// 12345, not the 6990 the rule would have computed: the correction wins.
		if persisted.RedemptionFeeSavedCents == nil || *persisted.RedemptionFeeSavedCents != 12345 {
			t.Errorf("redemption_fee_saved_cents = %v, want 12345", persisted.RedemptionFeeSavedCents)
		}
	})
}

func TestPatchIssuance_RedemptionFeeNullReturnsItToTheRule(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedComputableIssuance(t, tx)

		if rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"redemption_fee_saved_cents":12345}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"redemption_fee_saved_cents":null}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		persisted := readPersistedFee(t, tx, issuance.ID)
		if persisted.RedemptionFeeManual {
			t.Error("expected an explicit null to clear the manual flag")
		}
		if persisted.RedemptionFeeSavedCents == nil || *persisted.RedemptionFeeSavedCents != 6990 {
			t.Errorf("redemption_fee_saved_cents = %v, want the recomputed 6990", persisted.RedemptionFeeSavedCents)
		}
	})
}

// The distinction the raw-body decode exists for: an absent key must not look like a null.
func TestPatchIssuance_AbsentRedemptionFeeLeavesTheCorrectionAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedComputableIssuance(t, tx)

		if rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"redemption_fee_saved_cents":12345}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"observation":"anything else"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		persisted := readPersistedFee(t, tx, issuance.ID)
		if !persisted.RedemptionFeeManual {
			t.Error("expected the manual flag to survive an unrelated patch")
		}
		if persisted.RedemptionFeeSavedCents == nil || *persisted.RedemptionFeeSavedCents != 12345 {
			t.Errorf("redemption_fee_saved_cents = %v, want the untouched 12345", persisted.RedemptionFeeSavedCents)
		}
	})
}

// A patch that does not mention the fee still has to recompute it - moving the issuance off
// an exempt account has to take the saving with it.
func TestPatchIssuance_MovingToAnOrdinaryAccountClearsTheSaving(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedComputableIssuance(t, tx)

		if rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"redemption_fee_saved_cents":null}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if saved := readPersistedFee(t, tx, issuance.ID).RedemptionFeeSavedCents; saved == nil || *saved != 6990 {
			t.Fatalf("redemption_fee_saved_cents = %v, want 6990 before the move", saved)
		}

		ordinary := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: issuance.LoyaltyProgramID}
		create(t, tx, &ordinary)

		rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"account_id":"`+ordinary.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}

		if saved := readPersistedFee(t, tx, issuance.ID).RedemptionFeeSavedCents; saved != nil {
			t.Errorf("redemption_fee_saved_cents = %v, want nil - an ordinary account saves nothing", *saved)
		}
	})
}
