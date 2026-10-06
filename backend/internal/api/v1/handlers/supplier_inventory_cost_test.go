package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// seedInventoryBooking is a 10.000-point booking on an account that owns its points, with an
// optional batch already sitting in the account to pay for it.
func seedInventoryBooking(t *testing.T, tx *gorm.DB, batchCostCents *int) data.Issuance {
	t.Helper()
	main := defaultAgency(t, tx)
	issuance, _ := seedSaleFixture(t, tx, main.ID)

	if err := tx.Model(&data.Account{}).Where("id = ?", issuance.AccountID).
		Update("owns_point_inventory", true).Error; err != nil {
		t.Fatalf("failed to turn the inventory on: %v", err)
	}
	if err := tx.Model(&issuance).Updates(map[string]any{
		"points":                10000,
		"value_of_the_thousand": 2000,
	}).Error; err != nil {
		t.Fatalf("failed to size the booking: %v", err)
	}

	if batchCostCents != nil {
		batch := data.PointBatch{
			AccountID: issuance.AccountID, AcquiredOn: time.Now().AddDate(0, 0, -1), Points: 10000,
			CostCents: *batchCostCents, FundingAgencyID: main.ID, ProfitSharePercent: 50, Opening: true,
		}
		create(t, tx, &batch)
	}
	return issuance
}

func supplierLineOf(t *testing.T, tx *gorm.DB, issuance data.Issuance) schemas.SupplierIssuanceResponse {
	t.Helper()
	today := spDay(t, 0)
	balance := findBalance(callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today), issuance.AccountID)
	if balance == nil || len(balance.Issuances) != 1 {
		t.Fatalf("want one statement line for the booking, got %+v", balance)
	}
	return balance.Issuances[0]
}

// A booking no batch covers yet has no cost to derive, so it is priced by hand like any
// other - before this, the field was locked and the line stayed blank for good.
func TestSupplierCost_InventoryBookingWithoutABatchIsTyped(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance := seedInventoryBooking(t, tx, nil)

		line := supplierLineOf(t, tx, issuance)
		if line.CostFromInventory || line.CostCents != nil {
			t.Fatalf("an uncovered booking claimed an inventory cost: %+v", line)
		}

		rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"cost_of_the_thousand":1650}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("typing the rate: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		line = supplierLineOf(t, tx, issuance)
		if line.CostOfTheThousand == nil || *line.CostOfTheThousand != 1650 {
			t.Errorf("rate on the statement: got %v, want the typed 1650", line.CostOfTheThousand)
		}
	})
}

// Once a batch pays for the booking the FIFO is the answer, shown without waiting for
// anyone to open the closing screen, and a typed rate is refused.
func TestSupplierCost_InventoryBookingCoveredByABatchIsDerived(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		cost := 15000
		issuance := seedInventoryBooking(t, tx, &cost)

		line := supplierLineOf(t, tx, issuance)
		if !line.CostFromInventory || line.CostCents == nil || *line.CostCents != 15000 {
			t.Fatalf("covered booking: got %+v, want 15000 cents from the inventory", line)
		}
		if line.CostOfTheThousand == nil || *line.CostOfTheThousand != 1500 {
			t.Errorf("derived rate: got %v, want 1500", line.CostOfTheThousand)
		}

		rec := callSetSupplierCost(t, tx, `{"issuance_ids":["`+issuance.ID+`"],"cost_of_the_thousand":1650}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("typing over a derived rate: got %d, want 422", rec.Code)
		}
	})
}

// A batch that cost nothing - a flight accrual - still covers the booking, and the line says
// zero rather than falling back to blank.
func TestSupplierCost_ZeroCostBatchIsAnAnswer(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		free := 0
		issuance := seedInventoryBooking(t, tx, &free)

		line := supplierLineOf(t, tx, issuance)
		if !line.CostFromInventory || line.CostCents == nil || *line.CostCents != 0 {
			t.Fatalf("zero-cost batch: got %+v, want 0 cents from the inventory", line)
		}
	})
}
