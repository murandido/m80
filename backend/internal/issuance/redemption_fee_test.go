package issuance

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/murandido/m-80-backend/internal/data"
)

// feeFixture is a synced issuance with the Azul rule configured on its program: the shape
// every case below starts from, since anything less is one of the "do not compute" guards.
type feeFixture struct {
	fixture
	Issuance data.Issuance
}

func seedFeeFixture(t *testing.T, tx *gorm.DB) feeFixture {
	t.Helper()

	f := seedFixture(t, tx, data.Airline{})

	domestic, international, window := 6990, 17490, 120
	f.Program.RedemptionFeeDomesticCents = &domestic
	f.Program.RedemptionFeeInternationalCents = &international
	f.Program.RedemptionFeeAdvanceDays = &window
	if err := tx.Save(&f.Program).Error; err != nil {
		t.Fatalf("failed to configure the program fee: %v", err)
	}

	issuance, err := Create(tx, f.input(), f.User.ID)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	markSynced(t, tx, issuance.ID)

	return feeFixture{fixture: f, Issuance: issuance}
}

// markSynced is what separates an issuance the rule will touch from one it will not. Written
// with UpdateColumn so it does not disturb created_at, which the window is measured from.
func markSynced(t *testing.T, tx *gorm.DB, issuanceID string) {
	t.Helper()
	now := time.Now()
	err := tx.Model(&data.Issuance{}).Where("id = ?", issuanceID).
		UpdateColumn("traveos_synced_at", now).Error
	if err != nil {
		t.Fatalf("failed to mark the issuance as synced: %v", err)
	}
}

// addLeg attaches one flight leg. daysAhead is counted from now, which is close enough to the
// issuance's created_at for a threshold measured in days.
func addLeg(t *testing.T, tx *gorm.DB, issuanceID, direction string, legOrder, daysAhead int, origin, destination string) {
	t.Helper()
	flight := data.IssuanceFlight{
		IssuanceID:          issuanceID,
		Direction:           direction,
		LegOrder:            legOrder,
		DepartureAt:         time.Now().AddDate(0, 0, daysAhead),
		OriginIATACode:      &origin,
		DestinationIATACode: &destination,
	}
	create(t, tx, &flight)
}

func addPassenger(t *testing.T, tx *gorm.DB, issuanceID string, position int) {
	t.Helper()
	addTypedPassenger(t, tx, issuanceID, position, "ADT")
}

func addTypedPassenger(t *testing.T, tx *gorm.DB, issuanceID string, position int, passengerType string) {
	t.Helper()
	passenger := data.IssuancePassenger{
		IssuanceID: issuanceID,
		FirstName:  "Passenger",
		LastName:   uuid.New().String()[:8],
		Type:       passengerType,
		Position:   position,
	}
	create(t, tx, &passenger)
}

func readTaxes(t *testing.T, tx *gorm.DB, issuanceID string) *int {
	t.Helper()
	var issuance data.Issuance
	if err := tx.First(&issuance, "id = ?", issuanceID).Error; err != nil {
		t.Fatalf("failed to re-read the issuance: %v", err)
	}
	return issuance.Taxes
}

func assertTaxes(t *testing.T, tx *gorm.DB, issuanceID string, want int) {
	t.Helper()
	taxes := readTaxes(t, tx, issuanceID)
	if taxes == nil {
		t.Fatalf("taxes = nil, want %d", want)
	}
	if *taxes != want {
		t.Fatalf("taxes = %d, want %d", *taxes, want)
	}
}

func setTaxes(t *testing.T, tx *gorm.DB, issuanceID string, cents int) {
	t.Helper()
	err := tx.Model(&data.Issuance{}).Where("id = ?", issuanceID).
		UpdateColumn("taxes", cents).Error
	if err != nil {
		t.Fatalf("failed to set taxes: %v", err)
	}
}

// seedAirport registers an airport in a given country, so the domestic/international split
// has something to read. The IATA code is unique per call because the table is shared.
func seedAirport(t *testing.T, tx *gorm.DB, country string) string {
	t.Helper()
	code := uuid.New().String()[:3]
	airport := data.Airport{IATACode: code, Name: "Test Airport " + code, Country: country}
	create(t, tx, &airport)
	return code
}

func readSaved(t *testing.T, tx *gorm.DB, issuanceID string) *int {
	t.Helper()
	var issuance data.Issuance
	if err := tx.First(&issuance, "id = ?", issuanceID).Error; err != nil {
		t.Fatalf("failed to re-read the issuance: %v", err)
	}
	return issuance.RedemptionFeeSavedCents
}

func assertSaved(t *testing.T, tx *gorm.DB, issuanceID string, want int) {
	t.Helper()
	saved := readSaved(t, tx, issuanceID)
	if saved == nil {
		t.Fatalf("saved = nil, want %d", want)
	}
	if *saved != want {
		t.Fatalf("saved = %d, want %d", *saved, want)
	}
}

func assertNotComputed(t *testing.T, tx *gorm.DB, issuanceID string) {
	t.Helper()
	if saved := readSaved(t, tx, issuanceID); saved != nil {
		t.Fatalf("saved = %v, want nil - nothing saved is not the same as a computed zero", *saved)
	}
}

// waive marks the fixture's account exempt, which is the only state the rule computes in.
func waive(t *testing.T, tx *gorm.DB, f *feeFixture) {
	t.Helper()
	f.Account.WaivesRedemptionFee = true
	if err := tx.Save(&f.Account).Error; err != nil {
		t.Fatalf("failed to mark the account exempt: %v", err)
	}
}

func TestRedemptionFee_SavesPerTrechoAndPerPassenger(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		domestic := seedAirport(t, tx, "BR")
		other := seedAirport(t, tx, "BR")

		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, domestic, other)
		addLeg(t, tx, f.Issuance.ID, "inbound", 1, 40, other, domestic)
		addPassenger(t, tx, f.Issuance.ID, 0)
		addPassenger(t, tx, f.Issuance.ID, 1)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		// 2 trechos * 2 passengers * R$ 69,90, all of it avoided.
		assertSaved(t, tx, f.Issuance.ID, 2*2*6990)
	})
}

// The same itinerary on an ordinary account saves nothing, and what it paid is deliberately
// not recorded - the column stays empty rather than reporting a zero saving.
func TestRedemptionFee_OrdinaryAccountRecordsNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		domestic := seedAirport(t, tx, "BR")
		other := seedAirport(t, tx, "BR")

		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, domestic, other)
		addLeg(t, tx, f.Issuance.ID, "inbound", 1, 40, other, domestic)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertNotComputed(t, tx, f.Issuance.ID)
	})
}

// Moving an issuance off an exempt account has to take the saving with it: leaving the old
// number behind would keep crediting an economy the agency no longer earns.
func TestRedemptionFee_LeavingAnExemptAccountClearsTheSaving(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}
		assertSaved(t, tx, f.Issuance.ID, 6990)

		f.Account.WaivesRedemptionFee = false
		if err := tx.Save(&f.Account).Error; err != nil {
			t.Fatalf("failed to drop the exemption: %v", err)
		}
		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertNotComputed(t, tx, f.Issuance.ID)
	})
}

func TestRedemptionFee_OnlyChargesTheDirectionsInsideTheWindow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")

		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		// The return leaves well past the window, so it costs nothing even though the
		// outbound on the same booking does.
		addLeg(t, tx, f.Issuance.ID, "inbound", 1, 200, destination, origin)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 6990)
	})
}

// "Serão isentas ... com data igual ou superior a 120 dias" - the boundary day is already
// free, which an off-by-one in the comparison would charge for.
func TestRedemptionFee_ExactlyTheWindowIsExempt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")

		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 120, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		// Zero, not nil: the rule ran and there was simply no fee to avoid. Those two states
		// are what the column distinguishes.
		assertSaved(t, tx, f.Issuance.ID, 0)
	})
}

func TestRedemptionFee_ForeignAirportUsesTheInternationalRate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "US")

		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 17490)
	})
}

// A connection is part of one trecho, not a second one. Counting legs instead of directions
// would double this booking's fee.
func TestRedemptionFee_ConnectionDoesNotCountAsAnotherTrecho(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		stopover := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")

		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, stopover)
		addLeg(t, tx, f.Issuance.ID, "outbound", 2, 30, stopover, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 6990)
	})
}

// The direction's own first leg is what the window is measured against, whatever order the
// legs happen to come back in.
func TestRedemptionFee_MeasuresTheWindowFromTheFirstLeg(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		stopover := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")

		// Leg 2 is inside the window; leg 1, the one the trip starts on, is not.
		addLeg(t, tx, f.Issuance.ID, "outbound", 2, 30, stopover, destination)
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 200, origin, stopover)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 0)
	})
}

func TestRedemptionFee_UnsyncedIssuanceIsLeftAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)

		err := tx.Model(&data.Issuance{}).Where("id = ?", f.Issuance.ID).
			UpdateColumn("traveos_synced_at", nil).Error
		if err != nil {
			t.Fatalf("failed to clear traveos_synced_at: %v", err)
		}

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertNotComputed(t, tx, f.Issuance.ID)
	})
}

func TestRedemptionFee_ManualCorrectionIsNotOverwritten(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		err := tx.Model(&data.Issuance{}).Where("id = ?", f.Issuance.ID).
			UpdateColumns(map[string]any{
				"redemption_fee_manual":      true,
				"redemption_fee_saved_cents": 12345,
			}).Error
		if err != nil {
			t.Fatalf("failed to set the manual override: %v", err)
		}

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 12345)
	})
}

func TestRedemptionFee_ProgramWithoutParametersComputesNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		f.Program.RedemptionFeeDomesticCents = nil
		f.Program.RedemptionFeeInternationalCents = nil
		f.Program.RedemptionFeeAdvanceDays = nil
		if err := tx.Save(&f.Program).Error; err != nil {
			t.Fatalf("failed to clear the program fee: %v", err)
		}

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertNotComputed(t, tx, f.Issuance.ID)
	})
}

// A synced itinerary with no passenger rows is still a booking for somebody.
func TestRedemptionFee_NoPassengerRowsCountsAsOne(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 6990)
	})
}

// An ordinary account has no exemption for a correction to be about, so a value that
// reached the column - through the API, or left behind by an account that used to be
// exempt - is dropped even though it is flagged as a manual override.
func TestRedemptionFee_ManualValueOnAnOrdinaryAccountIsDropped(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		markSynced(t, tx, f.Issuance.ID)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)

		err := tx.Model(&data.Issuance{}).Where("id = ?", f.Issuance.ID).
			UpdateColumns(map[string]any{
				"redemption_fee_manual":      true,
				"redemption_fee_saved_cents": 12345,
			}).Error
		if err != nil {
			t.Fatalf("failed to set the manual override: %v", err)
		}

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertNotComputed(t, tx, f.Issuance.ID)
	})
}

// Clearing drops the manual flag with the amount. Without that, a row pinned while its
// account was exempt would stay pinned at NULL and never recompute if it went back.
func TestRedemptionFee_ClearingReleasesTheManualFlag(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		markSynced(t, tx, f.Issuance.ID)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		err := tx.Model(&data.Issuance{}).Where("id = ?", f.Issuance.ID).
			UpdateColumns(map[string]any{
				"redemption_fee_manual":      true,
				"redemption_fee_saved_cents": 12345,
			}).Error
		if err != nil {
			t.Fatalf("failed to set the manual override: %v", err)
		}

		// Ordinary account: this clears both columns.
		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		// Back onto an exempt account, the rule must be free to compute again.
		waive(t, tx, &f)
		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertSaved(t, tx, f.Issuance.ID, 6990)
	})
}

// Lap infants fly free of the redemption fee, so they must not multiply it. One adult and one
// infant on a single trecho owe one fee, not two.
func TestRedemptionFee_InfantsAreNotCharged(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addTypedPassenger(t, tx, f.Issuance.ID, 0, "ADT")
		addTypedPassenger(t, tx, f.Issuance.ID, 1, "INF")

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		// One chargeable passenger, one trecho: the infant is skipped.
		assertSaved(t, tx, f.Issuance.ID, 6990)
	})
}

// The computed fee is folded into Taxes so the client is charged it, and a second recalculation
// with the same itinerary must not add it again.
func TestRedemptionFee_FoldsFeeIntoTaxesOnce(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}
		assertSaved(t, tx, f.Issuance.ID, 6990)
		assertTaxes(t, tx, f.Issuance.ID, 6990)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("second RecalculateRedemptionFee failed: %v", err)
		}
		// Still 6990, not 13980: the delta on an unchanged itinerary is zero.
		assertTaxes(t, tx, f.Issuance.ID, 6990)
	})
}

// Taxes the operator typed on top of the folded fee survive a recalculation: only the fee's own
// delta moves, never the whole column.
func TestRedemptionFee_PreservesOperatorTaxesOnRecalc(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}
		assertTaxes(t, tx, f.Issuance.ID, 6990)

		// Operator adds R$ 50,00 of other taxes on top of the folded fee.
		setTaxes(t, tx, f.Issuance.ID, 6990+5000)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("second RecalculateRedemptionFee failed: %v", err)
		}
		assertTaxes(t, tx, f.Issuance.ID, 6990+5000)
	})
}

// Moving an issuance off an exempt account takes the folded fee back out of Taxes: the client no
// longer owes a fee the account no longer earns.
func TestRedemptionFee_LeavingExemptAccountUnfoldsTaxes(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFeeFixture(t, tx)
		waive(t, tx, &f)
		origin := seedAirport(t, tx, "BR")
		destination := seedAirport(t, tx, "BR")
		addLeg(t, tx, f.Issuance.ID, "outbound", 1, 30, origin, destination)
		addPassenger(t, tx, f.Issuance.ID, 0)

		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}
		assertTaxes(t, tx, f.Issuance.ID, 6990)

		f.Account.WaivesRedemptionFee = false
		if err := tx.Save(&f.Account).Error; err != nil {
			t.Fatalf("failed to drop the exemption: %v", err)
		}
		if err := RecalculateRedemptionFee(tx, f.Issuance.ID); err != nil {
			t.Fatalf("RecalculateRedemptionFee failed: %v", err)
		}

		assertNotComputed(t, tx, f.Issuance.ID)
		assertTaxes(t, tx, f.Issuance.ID, 0)
	})
}
