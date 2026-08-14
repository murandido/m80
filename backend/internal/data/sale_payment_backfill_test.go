package data

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The backfill in migration 36 is the one part of this feature that cannot be re-run: it
// turns every sale already marked paid into a payment, and if it gets the amount wrong the
// whole history is wrong in a way nobody notices until a client is charged twice.
//
// So these tests execute the statement from the migration file itself rather than a copy of
// it. A copy would be free to drift from what actually ships, which is exactly the failure
// mode worth protecting against.
const backfillMarker = "INSERT INTO sale_payments"

func backfillStatement(t *testing.T) string {
	t.Helper()

	content, err := migrationsFS.ReadFile("migrations/000036_create_sale_payments_table.up.sql")
	if err != nil {
		t.Fatalf("reading migration 36: %v", err)
	}

	index := strings.Index(string(content), backfillMarker)
	if index < 0 {
		t.Fatalf("migration 36 no longer contains %q - this test is reading the wrong file", backfillMarker)
	}
	return string(content)[index:]
}

// saleWorld is the chain a sale needs to exist: airline, program, agency, user, account and
// the issuance itself.
type saleWorld struct {
	issuance Issuance
	sale     Sale
}

func seedSale(t *testing.T, tx *gorm.DB, points, valueOfTheThousand int, taxes *int, paid bool) saleWorld {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreateRow(t, tx, &airline)

	program := LoyaltyProgram{
		Name: "Programa " + suffix, Code: "P" + suffix, DefaultAirlineID: airline.ID,
	}
	mustCreateRow(t, tx, &program)

	agency := Agency{Name: "Agência " + suffix}
	mustCreateRow(t, tx, &agency)

	user := User{Name: "Emissor " + suffix, Phone: nextBackfillPhone(), Password: "secret123"}
	mustCreateRow(t, tx, &user)

	holder := Person{Name: "Titular " + suffix}
	mustCreateRow(t, tx, &holder)

	account := Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
	mustCreateRow(t, tx, &account)

	issuance := Issuance{
		LoyaltyProgramID:   program.ID,
		AirlineID:          airline.ID,
		ProgramLocator:     strings.ToUpper(suffix),
		Points:             points,
		ValueOfTheThousand: valueOfTheThousand,
		Taxes:              taxes,
		AccountID:          account.ID,
		IssuerID:           user.ID,
		AgencyID:           agency.ID,
		Active:             true,
	}
	mustCreateRow(t, tx, &issuance)

	sale := Sale{IssuanceID: issuance.ID, Paid: paid}
	mustCreateRow(t, tx, &sale)

	return saleWorld{issuance: issuance, sale: sale}
}

func mustCreateRow(t *testing.T, tx *gorm.DB, value any) {
	t.Helper()
	if err := tx.Create(value).Error; err != nil {
		t.Fatalf("seeding %T: %v", value, err)
	}
}

var backfillPhoneCounter int

// users.phone is unique, so each seeded issuer needs its own valid Brazilian mobile.
func nextBackfillPhone() string {
	backfillPhoneCounter++
	return fmt.Sprintf("1198%07d", backfillPhoneCounter)
}

// runBackfill executes the migration's statement inside the test transaction and returns the
// payments it produced for the given sale.
func runBackfill(t *testing.T, tx *gorm.DB, saleID string) []SalePayment {
	t.Helper()

	if err := tx.Exec(backfillStatement(t)).Error; err != nil {
		t.Fatalf("running the backfill: %v", err)
	}

	var payments []SalePayment
	if err := tx.Where("sale_id = ?", saleID).Find(&payments).Error; err != nil {
		t.Fatalf("loading the payments: %v", err)
	}
	return payments
}

func TestBackfillTurnsAPaidSaleIntoOnePaymentForItsFullValue(t *testing.T) {
	runInBackfillTx(t, func(tx *gorm.DB) {
		taxes := 25306
		// 18.000 points at R$ 16,25 per thousand = R$ 292,50, plus R$ 253,06 of taxes.
		world := seedSale(t, tx, 18000, 1625, &taxes, true)

		payments := runBackfill(t, tx, world.sale.ID)

		if len(payments) != 1 {
			t.Fatalf("got %d payments, want exactly 1", len(payments))
		}
		if payments[0].AmountCents != 29250+25306 {
			t.Errorf("amount = %d, want %d - the figure the screen was showing",
				payments[0].AmountCents, 29250+25306)
		}
		if payments[0].CreatedByID != world.issuance.IssuerID {
			t.Errorf("created_by = %q, want the issuer %q - nobody recorded who ticked the box",
				payments[0].CreatedByID, world.issuance.IssuerID)
		}
	})
}

func TestBackfillRoundsTheThousandUpLikeTheScreenDoes(t *testing.T) {
	runInBackfillTx(t, func(tx *gorm.DB) {
		// 29.577 points at R$ 16,50 = 48.802,05 centavos. pointsValueCents rounds UP to the
		// centavo (and so does the export spreadsheet), so this has to be 48803, not 48802.
		world := seedSale(t, tx, 29577, 1650, nil, true)

		payments := runBackfill(t, tx, world.sale.ID)

		if len(payments) != 1 {
			t.Fatalf("got %d payments, want 1", len(payments))
		}
		if payments[0].AmountCents != 48803 {
			t.Errorf("amount = %d, want 48803 (rounded up); a cent of drift here is a cent of drift against the export",
				payments[0].AmountCents)
		}
	})
}

func TestBackfillLeavesAnUnpaidSaleAlone(t *testing.T) {
	runInBackfillTx(t, func(tx *gorm.DB) {
		world := seedSale(t, tx, 18000, 1625, nil, false)

		payments := runBackfill(t, tx, world.sale.ID)

		if len(payments) != 0 {
			t.Fatalf("got %d payments for a sale nobody had paid, want 0", len(payments))
		}
	})
}

func TestBackfillSkipsASaleWorthNothing(t *testing.T) {
	runInBackfillTx(t, func(tx *gorm.DB) {
		// A zero-value payment would trip the check constraint and take the whole migration
		// - and the API boot behind it - down with it.
		world := seedSale(t, tx, 0, 0, nil, true)

		payments := runBackfill(t, tx, world.sale.ID)

		if len(payments) != 0 {
			t.Fatalf("got %d payments for a sale worth nothing, want 0", len(payments))
		}
	})
}

// runInBackfillTx rolls everything back, so running the migration's statement over and over
// never leaves duplicates behind.
func runInBackfillTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("beginning the test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })

	fn(tx)
}
