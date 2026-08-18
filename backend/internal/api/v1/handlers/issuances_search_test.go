package handlers

import (
	"net/url"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// backdate moves an issuance out of the current day so a test can prove the search
// reaches past the "today + me" window the panel opens with.
func backdate(t *testing.T, tx *gorm.DB, issuance data.Issuance, days int) {
	t.Helper()
	if err := tx.Model(&issuance).Update("created_at", time.Now().AddDate(0, 0, -days)).Error; err != nil {
		t.Fatalf("failed to backdate created_at: %v", err)
	}
}

// ensureAirport makes the IATA code resolvable for the FK. The airports table is
// seeded from a real dataset, so the row usually exists already - FirstOrCreate
// keeps the test working either way.
func ensureAirport(t *testing.T, tx *gorm.DB, code string) {
	t.Helper()
	airport := data.Airport{IATACode: code, Name: "Seeded " + code, Country: "BR"}
	if err := tx.Where("iata_code = ?", code).FirstOrCreate(&airport).Error; err != nil {
		t.Fatalf("failed to ensure airport %s: %v", code, err)
	}
}

func TestGetIssuances_SearchMatchesEveryLocator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		airlineLocator := "XY7788"
		latam := "9911223344"
		if err := tx.Model(&issuance).Updates(map[string]any{
			"airline_locator":       airlineLocator,
			"latam_purchase_number": latam,
		}).Error; err != nil {
			t.Fatalf("failed to set the extra locators: %v", err)
		}

		for _, term := range []string{issuance.ProgramLocator, airlineLocator, latam} {
			if ids := callGetIssuances(t, tx, "q="+url.QueryEscape(term)); !ids[issuance.ID] {
				t.Errorf("searching %q did not find the issuance", term)
			}
		}

		if ids := callGetIssuances(t, tx, "q=NOTHINGMATCHESTHIS"); ids[issuance.ID] {
			t.Error("a term matching no field still returned the issuance")
		}
	})
}

// The whole point of the feature: the operator has a locator and no idea which day
// it belongs to, while the screen is filtered on today and on them.
func TestGetIssuances_SearchIgnoresDateAndIssuer(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, issuer := seedIssuance(t, tx)
		backdate(t, tx, issuance, 30)

		other := createUser(t, tx, "Somebody Else")
		today := spTodayString(t)
		query := "q=" + url.QueryEscape(issuance.ProgramLocator) +
			"&date_from=" + today + "&date_to=" + today + "&issuer_id=" + other.ID

		if ids := callGetIssuances(t, tx, query); !ids[issuance.ID] {
			t.Error("the search should reach past the date and issuer filters, but did not")
		}

		// Without the search those same filters must still hide it - otherwise the
		// suspension leaked into the normal listing.
		filtersOnly := "date_from=" + today + "&date_to=" + today + "&issuer_id=" + issuer.ID
		if ids := callGetIssuances(t, tx, filtersOnly); ids[issuance.ID] {
			t.Error("a 30-day-old issuance appeared in today's flow")
		}
	})
}

// Accent folding is not cosmetic here: "joao" missing "João" is how the same
// booking gets registered a second time.
func TestGetIssuances_SearchMatchesPassengersAccentInsensitively(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		main := "JOÃO DA SILVA"
		if err := tx.Model(&issuance).Update("main_passenger_name", main).Error; err != nil {
			t.Fatalf("failed to set the main passenger: %v", err)
		}
		// A second traveller, who only exists in the child table.
		create(t, tx, &data.IssuancePassenger{
			IssuanceID: issuance.ID,
			FirstName:  "Conceição",
			LastName:   "Andrade",
			Type:       "ADT",
			Position:   2,
		})

		for _, term := range []string{"joao", "JOÃO", "silva", "conceicao", "Conceição Andrade"} {
			if ids := callGetIssuances(t, tx, "q="+url.QueryEscape(term)); !ids[issuance.ID] {
				t.Errorf("searching passenger %q did not find the issuance", term)
			}
		}
	})
}

func TestGetIssuances_SearchMatchesRouteIncludingConnections(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		origin, destination, connection := "CGH", "SDU", "BSB"
		for _, code := range []string{origin, destination, connection} {
			ensureAirport(t, tx, code)
		}

		if err := tx.Model(&issuance).Updates(map[string]any{
			"origin_iata_code":      origin,
			"destination_iata_code": destination,
		}).Error; err != nil {
			t.Fatalf("failed to set the route: %v", err)
		}
		// The connection exists only on the leg, never on the issuance itself.
		create(t, tx, &data.IssuanceFlight{
			IssuanceID:          issuance.ID,
			Direction:           "outbound",
			LegOrder:            1,
			OriginIATACode:      &origin,
			DestinationIATACode: &connection,
			DepartureAt:         time.Now(),
		})

		for _, term := range []string{origin, destination, connection, "cgh"} {
			if ids := callGetIssuances(t, tx, "q="+url.QueryEscape(term)); !ids[issuance.ID] {
				t.Errorf("searching route %q did not find the issuance", term)
			}
		}
	})
}

func TestGetIssuances_FiltersByAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		other := data.Agency{Name: "Other Agency " + issuance.ID[:8]}
		create(t, tx, &other)

		if ids := callGetIssuances(t, tx, "agency_id="+issuance.AgencyID); !ids[issuance.ID] {
			t.Error("filtering by the issuance's own agency hid it")
		}
		if ids := callGetIssuances(t, tx, "agency_id="+other.ID); ids[issuance.ID] {
			t.Error("filtering by a different agency still returned the issuance")
		}
	})
}

// The other dimensions keep applying while a search is active - only date and
// issuer step aside.
func TestGetIssuances_SearchStillRespectsAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, _ := seedIssuance(t, tx)
		other := data.Agency{Name: "Other Agency " + issuance.ID[:8]}
		create(t, tx, &other)

		query := "q=" + url.QueryEscape(issuance.ProgramLocator) + "&agency_id=" + other.ID
		if ids := callGetIssuances(t, tx, query); ids[issuance.ID] {
			t.Error("the agency filter was dropped while searching")
		}
	})
}
