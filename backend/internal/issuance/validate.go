package issuance

import (
	"errors"
	"fmt"
	"strings"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// ValidateAirportCodes checks that every non-empty IATA code given resolves to a
// row in airports. The columns are already FKs to airports.iata_code, so the
// database would reject a bad code anyway - but it does so as a raw 23503 that
// every caller turned into a generic 500. Resolving the codes up front lets the
// failure name the offending code instead.
func ValidateAirportCodes(db *gorm.DB, codes ...*string) error {
	for _, code := range codes {
		if code == nil || strings.TrimSpace(*code) == "" {
			continue
		}

		var airport data.Airport
		err := db.First(&airport, "iata_code = ?", *code).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: %s", ErrAirportNotFound, *code)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateAirlineRequirements enforces the airlines.requires_last_name and
// requires_purchase_number flags, which until now existed only in the airline
// CRUD and were never read when issuing.
//
// It is deliberately NOT called on every write. The panel's grid autosaves a
// draft row the moment its five mandatory fields are filled (see
// draftIsReadyToCreate in the frontend), long before the operator has typed a
// passenger name - enforcing these flags at create time would break that flow
// entirely. They are instead enforced at the *completion* boundary: creating an
// already-complete issuance, or transitioning an existing one to complete. The
// bot calls it directly because a WhatsApp message is a one-shot input with no
// later chance to fill the gap.
func ValidateAirlineRequirements(airline data.Airline, lastName, purchaseNumber *string) error {
	if airline.RequiresLastName && isBlank(lastName) {
		return ErrLastNameRequired
	}
	if airline.RequiresPurchaseNumber && isBlank(purchaseNumber) {
		return ErrPurchaseNumberRequired
	}
	return nil
}

func isBlank(value *string) bool {
	return value == nil || strings.TrimSpace(*value) == ""
}
