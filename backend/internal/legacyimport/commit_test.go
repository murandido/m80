package legacyimport

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// allRows selects every row of a day, which is what the screen sends when nothing was
// unticked.
func allRows(day Day) []RowSelection {
	var selected []RowSelection
	for _, block := range day.Blocks {
		for _, row := range block.Rows {
			selected = append(selected, RowSelection{BlockIndex: block.Index, Number: row.Number})
		}
	}
	return selected
}

func fetchIssuance(t *testing.T, tx *gorm.DB, locator string) data.Issuance {
	t.Helper()
	var issuance data.Issuance
	if err := tx.Where("program_locator = ?", locator).First(&issuance).Error; err != nil {
		t.Fatalf("loading the issuance %q: %v", locator, err)
	}
	return issuance
}

func TestCommit_WritesTheRowWithTheSheetsOwnDate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		row := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		row.Taxes = intptr(25306)
		day := dayWith(w.user.Name, row)
		mappings := Mappings{AgencyID: w.agency.ID}

		result, err := Commit(tx, day, mappings, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if result.Imported != 1 {
			t.Fatalf("imported = %d (skipped %+v), want 1", result.Imported, result.Skipped)
		}

		issuance := fetchIssuance(t, tx, "EQF8VK")
		// The whole reason CreateInput learned about CreatedAt: the grid and every
		// date filter read it as the day of the issuance, so an August row stamped
		// with now() would vanish from its own day.
		//
		// Compared in São Paulo, which is where the listings resolve their date range.
		// Midnight UTC would pass a naive UTC comparison and still file the row under
		// the previous day on every screen - which is exactly what it did at first.
		loc, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			t.Fatalf("loading the listing timezone: %v", err)
		}
		if got := issuance.CreatedAt.In(loc).Format("2006-01-02"); got != "2026-08-01" {
			t.Errorf("created_at reads as %s in São Paulo, want the sheet's day 2026-08-01", got)
		}
		if !issuance.Imported {
			t.Error("imported = false, want true - it is what keeps Traveos from issuing this booking")
		}
		if issuance.Complete {
			t.Error("complete = true, want false - the spreadsheet has no itinerary")
		}
		if issuance.Points != 18000 || issuance.ValueOfTheThousand != 1625 {
			t.Errorf("points/value = %d/%d, want 18000/1625", issuance.Points, issuance.ValueOfTheThousand)
		}
		if issuance.Taxes == nil || *issuance.Taxes != 25306 {
			t.Errorf("taxes = %v, want 25306", issuance.Taxes)
		}
		if issuance.AgencyID != w.agency.ID {
			t.Errorf("agency = %q, want the chosen %q", issuance.AgencyID, w.agency.ID)
		}
		if issuance.IssuerID != w.user.ID {
			t.Errorf("issuer = %q, want %q", issuance.IssuerID, w.user.ID)
		}

		// The same open sale every other issuance is born with - the Vendas tab is
		// the only place that tracks who still has to be charged.
		var sales int64
		tx.Model(&data.Sale{}).Where("issuance_id = ?", issuance.ID).Count(&sales)
		if sales != 1 {
			t.Errorf("sales = %d, want 1", sales)
		}
	})
}

func TestCommit_WritesOnlyTheSelectedRows(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		first := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		second := sampleRow(w.program.Code, "LNS6MD", "MARISE")
		second.Number = 4
		day := dayWith(w.user.Name, first, second)

		// Standing in for the operator unticking a block: Clara and Wilmara already
		// registered their recent days through the panel.
		selected := []RowSelection{{BlockIndex: 0, Number: 3}}

		result, err := Commit(tx, day, Mappings{AgencyID: w.agency.ID}, selected, w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if result.Imported != 1 {
			t.Fatalf("imported = %d, want 1", result.Imported)
		}

		var count int64
		tx.Model(&data.Issuance{}).Where("program_locator = ?", "LNS6MD").Count(&count)
		if count != 0 {
			t.Errorf("the unticked row was written anyway (%d rows)", count)
		}
	})
}

func TestCommit_SendsEachRowToItsOwnAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		// One day, two agencies: which agency an issuance belongs to is a fact about
		// that issuance, and the spreadsheet has no column for it.
		other := data.Agency{Name: "Outra " + w.suffix}
		mustCreate(t, tx, &other)

		first := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		second := sampleRow(w.program.Code, "LNS6MD", "MARISE")
		second.Number = 4
		day := dayWith(w.user.Name, first, second)

		mappings := Mappings{
			AgencyID:    w.agency.ID,
			RowAgencies: []RowAgencyMapping{{BlockIndex: 0, Number: 4, AgencyID: other.ID}},
		}

		result, err := Commit(tx, day, mappings, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if result.Imported != 2 {
			t.Fatalf("imported = %d (skipped %+v), want 2", result.Imported, result.Skipped)
		}

		// The row with no choice of its own takes the day's agency.
		if got := fetchIssuance(t, tx, "EQF8VK").AgencyID; got != w.agency.ID {
			t.Errorf("agency = %q, want the day's %q", got, w.agency.ID)
		}
		if got := fetchIssuance(t, tx, "LNS6MD").AgencyID; got != other.ID {
			t.Errorf("agency = %q, want the row's own %q", got, other.ID)
		}
	})
}

func TestCommit_FallsBackToTheDaysAgencyWhenTheRowNamesNothingReal(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "EQF8VK", "MARISE"))
		mappings := Mappings{
			AgencyID: w.agency.ID,
			RowAgencies: []RowAgencyMapping{{
				BlockIndex: 0,
				Number:     3,
				AgencyID:   "00000000-0000-0000-0000-000000000000",
			}},
		}

		result, err := Commit(tx, day, mappings, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		// A stale id costs the choice, not the row: agency_id is a foreign key, so
		// writing it through would have failed the whole day.
		if result.Imported != 1 {
			t.Fatalf("imported = %d, want 1", result.Imported)
		}
		if got := fetchIssuance(t, tx, "EQF8VK").AgencyID; got != w.agency.ID {
			t.Errorf("agency = %q, want the day's %q", got, w.agency.ID)
		}
	})
}

func TestCommit_IsSafeToRunTwiceOverTheSameDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "EQF8VK", "MARISE"))
		mappings := Mappings{AgencyID: w.agency.ID}

		if _, err := Commit(tx, day, mappings, allRows(day), w.user.ID); err != nil {
			t.Fatalf("first Commit: %v", err)
		}

		result, err := Commit(tx, day, mappings, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("second Commit: %v", err)
		}
		if result.Imported != 0 {
			t.Errorf("imported = %d on the second run, want 0", result.Imported)
		}
		if len(result.Skipped) != 1 {
			t.Fatalf("skipped = %+v, want one entry explaining itself", result.Skipped)
		}

		var count int64
		tx.Model(&data.Issuance{}).Where("program_locator = ?", "EQF8VK").Count(&count)
		if count != 1 {
			t.Errorf("issuances with that locator = %d, want 1", count)
		}
	})
}

func TestCommit_SkipsAnUnresolvableRowWithoutLosingTheRest(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		good := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		orphan := sampleRow(w.program.Code, "LNS6MD", "APELIDO QUE NINGUÉM CADASTROU")
		orphan.Number = 4
		day := dayWith(w.user.Name, good, orphan)

		result, err := Commit(tx, day, Mappings{AgencyID: w.agency.ID}, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}

		// One bad line out of thirty-five should not cost the day.
		if result.Imported != 1 {
			t.Errorf("imported = %d, want 1", result.Imported)
		}
		if len(result.Skipped) != 1 || result.Skipped[0].Locator != "LNS6MD" {
			t.Errorf("skipped = %+v, want just LNS6MD with a reason", result.Skipped)
		}
	})
}

func TestCommit_RegistersTheAliasTheOperatorMapped(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Wesley Lima", "")

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "EQF8VK", "WESLEY (ELI)"))
		mappings := Mappings{
			AgencyID: w.agency.ID,
			Accounts: []AccountMapping{{
				ProgramCode:  w.program.Code,
				AccountLabel: "WESLEY (ELI)",
				AccountID:    account.ID,
			}},
		}

		result, err := Commit(tx, day, mappings, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if result.Imported != 1 {
			t.Fatalf("imported = %d (skipped %+v), want 1", result.Imported, result.Skipped)
		}
		if result.RegisteredAliases != 1 {
			t.Errorf("registered aliases = %d, want 1", result.RegisteredAliases)
		}

		// The point of registering it: the next day of the same spreadsheet resolves
		// on its own, with no mapping at all.
		preview, err := Resolve(tx, dayWith(w.user.Name,
			sampleRow(w.program.Code, "AJSD2Z", "wesley (eli)"),
		), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve after the alias was registered: %v", err)
		}
		if row := onlyRow(t, preview); row.AccountID != account.ID {
			t.Errorf("account = %q, want %q resolved from the new alias", row.AccountID, account.ID)
		}
	})
}

func TestCommit_DoesNotRegisterAnAliasTheAccountAlreadyAnswersTo(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Wesley Lima", "Wesley")

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "EQF8VK", "WESLEY"))
		mappings := Mappings{
			AgencyID: w.agency.ID,
			Accounts: []AccountMapping{{
				ProgramCode:  w.program.Code,
				AccountLabel: "WESLEY",
				AccountID:    account.ID,
			}},
		}

		result, err := Commit(tx, day, mappings, allRows(day), w.user.ID)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		// "WESLEY" would pass the unique index, which is over the exact string, and
		// leave the account answering to the same word twice.
		if result.RegisteredAliases != 0 {
			t.Errorf("registered aliases = %d, want 0", result.RegisteredAliases)
		}

		var aliases int64
		tx.Model(&data.AccountNickname{}).Where("account_id = ?", account.ID).Count(&aliases)
		if aliases != 1 {
			t.Errorf("aliases on the account = %d, want 1", aliases)
		}
	})
}

func TestCommit_WritesTheFlightDate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		departure := time.Date(2026, time.November, 13, 0, 0, 0, 0, time.UTC)
		row := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		row.FlightDate = &departure
		day := dayWith(w.user.Name, row)

		if _, err := Commit(tx, day, Mappings{AgencyID: w.agency.ID}, allRows(day), w.user.ID); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		issuance := fetchIssuance(t, tx, "EQF8VK")
		var flight data.IssuanceFlight
		if err := tx.Where("issuance_id = ?", issuance.ID).First(&flight).Error; err != nil {
			t.Fatalf("loading the flight: %v", err)
		}
		if !flight.DepartureAt.UTC().Equal(departure) {
			t.Errorf("departure = %v, want %v", flight.DepartureAt, departure)
		}
	})
}

func intptr(value int) *int { return &value }
