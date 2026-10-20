package issuance

import (
	"time"

	"gorm.io/gorm"

	"github.com/murandido/m-80-backend/internal/businessday"
	"github.com/murandido/m-80-backend/internal/data"
)

// feeTimezone is the zone the 120-day window is measured in. The threshold is a count of
// calendar days between two dates an operator reads off a screen, not a duration, so it has
// to be resolved against the zone they live in - the same reasoning as handlers.listTimezone
// and export.exportTimezone.
const feeTimezone = businessday.Timezone

// domesticCountry is the country code a leg has to stay inside for the domestic fee to
// apply. Airports seed with 'BR' as their default, so an airport nobody classified counts as
// domestic - which is the safer error here: it understates the fee rather than inventing an
// international one for a trip between two Brazilian cities.
const domesticCountry = "BR"

// infantPassengerType is the passenger_type enum value for a lap infant. Infants fly on an
// adult's lap and the airline does not charge them the redemption fee, so they are left out
// of the per-passenger multiplier - see the count in RecalculateRedemptionFee.
const infantPassengerType = "INF"

// RecalculateRedemptionFee brings an issuance's saved redemption fee back in line with its
// itinerary, its passengers and its account.
//
// The fee is the airline's "taxa de resgate": charged per passenger and per trecho when the
// ticket is issued fewer than the program's advance-days window before the flight. What is
// stored is only what an exempt account - Azul's Diamante Unique - did NOT have to pay, which
// is the number that prices issuing there rather than on an ordinary account. An ordinary
// account records nothing: what it paid the airline is settled outside this system and has no
// reader here. See data.Issuance.RedemptionFeeSavedCents.
//
// On an exempt account the computed fee is also folded into the issuance's Taxes, so it becomes
// part of what the client owes without the operator retyping it. It is added exactly once: the
// amount already inside Taxes is the previously saved fee, so a recalculation moves Taxes by the
// delta and leaves any taxes the operator typed on top untouched.
//
// Callers pass the transaction they are already in, like sale.Recalculate: the amount and the
// itinerary it is derived from must never be observed disagreeing. It has to run *after* the
// flights and passengers are written, which is why it is an explicit call and not a GORM
// hook - BeforeSave on the issuance fires before UpsertPrimaryOutboundFlight and would read
// the previous departure date.
//
// Every reason not to compute returns without writing anything. Leaving the column alone is
// not the same as writing zero: zero claims the rule ran and found nothing to save.
func RecalculateRedemptionFee(tx *gorm.DB, issuanceID string) error {
	var issuance data.Issuance
	err := tx.
		Preload("Account").
		Preload("LoyaltyProgram").
		Preload("Flights").
		Preload("Passengers").
		First(&issuance, "id = ?", issuanceID).Error
	if err != nil {
		return err
	}

	if issuance.Account == nil || issuance.LoyaltyProgram == nil {
		return nil
	}

	// An account that pays the fee has nothing to save, and the amount it paid is not
	// recorded here. Cleared rather than left alone, so an issuance moved off an exempt
	// account stops reporting a saving it no longer earns.
	//
	// Checked before the manual flag on purpose: on an ordinary account there is no amount
	// for a correction to be *about*, so a value that reached this column - typed against
	// the API, or left behind by an account that used to be exempt - is a mistake rather
	// than an override, and the flag must not protect it.
	if !issuance.Account.WaivesRedemptionFee {
		return clearRedemptionFeeSaved(tx, issuance)
	}

	// An operator already corrected this by hand. The rule got it wrong once here - it does
	// not get to try again on the next sync.
	if issuance.RedemptionFeeManual {
		return nil
	}
	// Without a synced itinerary there is no trip type and no passenger list: an issuance
	// typed on the grid or written by the bot carries one departure date and one passenger
	// name, which cannot tell a one-way from a round trip. Guessing would quietly halve or
	// double the fee, so the column stays empty and the operator fills it in.
	if issuance.TraveosSyncedAt == nil {
		return nil
	}

	program := issuance.LoyaltyProgram
	if program.RedemptionFeeAdvanceDays == nil ||
		program.RedemptionFeeDomesticCents == nil ||
		program.RedemptionFeeInternationalCents == nil {
		// A program nobody has configured charges no fee we know of.
		return nil
	}

	loc, err := time.LoadLocation(feeTimezone)
	if err != nil {
		loc = time.UTC
	}

	chargeable := chargeableDirections(issuance, *program.RedemptionFeeAdvanceDays, loc)

	international, err := touchesForeignAirport(tx, issuance)
	if err != nil {
		return err
	}

	perTrecho := *program.RedemptionFeeDomesticCents
	if international {
		perTrecho = *program.RedemptionFeeInternationalCents
	}

	// Lap infants (INF) fly on an adult's lap and are not charged the redemption fee, so they
	// are left out of the per-passenger multiplier. Floor of one: a synced itinerary with no
	// chargeable passenger rows is not a booking for nobody.
	passengers := 0
	for _, passenger := range issuance.Passengers {
		if passenger.Type == infantPassengerType {
			continue
		}
		passengers++
	}
	if passengers < 1 {
		passengers = 1
	}

	saved := perTrecho * chargeable * passengers

	// Fold the fee into Taxes so it is part of what the client owes, added exactly once. The
	// amount already baked into Taxes is the previously saved fee (reliable here: this path
	// only runs when the value is not a manual override), so moving Taxes by the delta never
	// re-adds the whole fee and preserves any taxes the operator typed on top.
	updates := map[string]any{"redemption_fee_saved_cents": saved}
	if delta := saved - intOrZero(issuance.RedemptionFeeSavedCents); delta != 0 {
		updates["taxes"] = intOrZero(issuance.Taxes) + delta
	}

	// UpdateColumns, not Save: this is bookkeeping derived from the itinerary, and bumping
	// updated_at would make it look like somebody edited the issuance. Same reasoning as
	// sale.Recalculate.
	return tx.Model(&data.Issuance{}).Where("id = ?", issuanceID).
		UpdateColumns(updates).Error
}

// intOrZero reads an optional cents column, treating "not set" as zero for arithmetic.
func intOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// clearRedemptionFeeSaved puts the column back to "nothing saved here", which is nil rather
// than zero for the same reason everywhere else: zero would read as a computed result.
func clearRedemptionFeeSaved(tx *gorm.DB, issuance data.Issuance) error {
	// The manual flag goes with the amount: it means "an operator corrected this number",
	// and there is no number left to have been corrected. Leaving it set would pin the row
	// at NULL, so moving it back onto an exempt account would never recompute.
	updates := map[string]any{
		"redemption_fee_saved_cents": nil,
		"redemption_fee_manual":      false,
	}
	// If an auto-computed fee had been folded into Taxes, take it back out: the account is no
	// longer exempt, so it no longer carries that saving, and leaving the amount in would keep
	// charging the client a fee the issuance no longer earns. A manual value is left alone - it
	// was typed against the API or left behind and was never folded into Taxes here, so
	// subtracting it would corrupt the taxes actually owed.
	if !issuance.RedemptionFeeManual && issuance.RedemptionFeeSavedCents != nil {
		updates["taxes"] = intOrZero(issuance.Taxes) - *issuance.RedemptionFeeSavedCents
	}
	return tx.Model(&data.Issuance{}).Where("id = ?", issuance.ID).
		UpdateColumns(updates).Error
}

// chargeableDirections counts the trechos this issuance owes the fee on: one per direction
// of the itinerary that departs inside the window.
//
// Per direction, not per leg. Azul's own examples call a round trip "dois trechos" and only
// ever describe direct flights; whether a connection counts as one trecho or two is not
// published anywhere we could find, and counting legs would double the fee on every itinerary
// with a stopover. Each direction is judged by the departure of its first leg, which is the
// one the passenger's trip actually starts on.
func chargeableDirections(issuance data.Issuance, advanceDays int, loc *time.Location) int {
	firstDeparture := map[string]data.IssuanceFlight{}
	for _, flight := range issuance.Flights {
		current, seen := firstDeparture[flight.Direction]
		if !seen || flight.LegOrder < current.LegOrder {
			firstDeparture[flight.Direction] = flight
		}
	}

	issuedOn := calendarDate(issuance.CreatedAt, loc)

	chargeable := 0
	for _, flight := range firstDeparture {
		// "igual ou superior" in the airline's wording: exactly the window is already exempt.
		if calendarDaysBetween(issuedOn, calendarDate(flight.DepartureAt, loc)) >= advanceDays {
			continue
		}
		chargeable++
	}
	return chargeable
}

// touchesForeignAirport reports whether any airport on the itinerary sits outside Brazil,
// which is what moves the whole issuance onto the international fee. The issuance's own
// origin/destination are included as a fallback for a leg the vendor sent without codes.
func touchesForeignAirport(tx *gorm.DB, issuance data.Issuance) (bool, error) {
	codes := map[string]struct{}{}
	add := func(code *string) {
		if code != nil && *code != "" {
			codes[*code] = struct{}{}
		}
	}
	for _, flight := range issuance.Flights {
		add(flight.OriginIATACode)
		add(flight.DestinationIATACode)
	}
	add(issuance.OriginIATACode)
	add(issuance.DestinationIATACode)

	if len(codes) == 0 {
		return false, nil
	}

	list := make([]string, 0, len(codes))
	for code := range codes {
		list = append(list, code)
	}

	var foreign int64
	err := tx.Model(&data.Airport{}).
		Where("iata_code IN ? AND country <> ?", list, domesticCountry).
		Count(&foreign).Error
	if err != nil {
		return false, err
	}
	return foreign > 0, nil
}

// calendarDate collapses a timestamp to midnight of its local calendar day, expressed in UTC
// so two of them can be subtracted without a DST offset creeping into the difference.
// departure_at is a timestamptz whose time component means a real departure on a synced
// itinerary and nothing at all on a date typed into the grid, so only the day is usable.
func calendarDate(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// calendarDaysBetween counts whole days between two values already collapsed by calendarDate.
func calendarDaysBetween(from, to time.Time) int {
	return int(to.Sub(from) / (24 * time.Hour))
}
