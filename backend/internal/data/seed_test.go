package data

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Same shape as the handlers' setupTestDB: probe the connection quietly first so a
// machine without a database skips instead of dying inside Connect's log.Fatal.
var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)

		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			testDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			testDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			testDBErr = err
			return
		}

		Connect()
		testDB = DB
	})

	if testDBErr != nil {
		t.Skipf("skipping seed tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runOnEmptyCatalog gives the body a transaction with no airlines and no loyalty
// programs in it, and rolls back afterwards. The seeders are about reconciling
// against whatever is already there, so every test needs to state its own starting
// point rather than inherit one.
func runOnEmptyCatalog(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin: %v", tx.Error)
	}
	defer tx.Rollback()

	for _, table := range []string{"loyalty_programs", "airlines"} {
		if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clearing %s: %v", table, err)
		}
	}

	fn(tx)
}

func countAirlines(t *testing.T, tx *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := tx.Model(&Airline{}).Count(&count).Error; err != nil {
		t.Fatalf("counting airlines: %v", err)
	}
	return count
}

func airlineByCode(t *testing.T, tx *gorm.DB, code string) Airline {
	t.Helper()
	var airline Airline
	if err := tx.Where("code = ?", code).First(&airline).Error; err != nil {
		t.Fatalf("airline %s: %v", code, err)
	}
	return airline
}

func TestSeedAirlinesPopulatesAnEmptyCatalog(t *testing.T) {
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		if got := countAirlines(t, tx); got != 14 {
			t.Errorf("seeded %d airlines, want 14", got)
		}

		// The five that have emitted for real, and the ids every emission is sent
		// with. A wrong id here is the one failure Traveos actually names (422
		// "Empresa não localizada"), so it is worth pinning down.
		for code, wantID := range map[string]int{
			"LATAM": 1, "AZUL": 2, "GOL": 3, "AA": 90, "AVIANCA": 101,
		} {
			airline := airlineByCode(t, tx, code)
			if airline.TraveosAirlineID == nil || *airline.TraveosAirlineID != wantID {
				t.Errorf("%s traveos_airline_id = %v, want %d", code, airline.TraveosAirlineID, wantID)
			}
			if !airline.TraveosSupported {
				t.Errorf("%s is not traveos_supported, but it has emitted for real", code)
			}
		}

		// The catalog entries nobody has a ticket for stay off: "ativa" on Traveos'
		// side is not a promise, and turning the gate on for a carrier we have never
		// read would turn a 🚀 into a ⚠️ for a group doing nothing wrong. TAP and
		// COPA are on because there is a real ticket for each on the account.
		for _, code := range []string{"AIREUROPA", "AZULVIAGENS", "UNITED", "QATAR"} {
			if airline := airlineByCode(t, tx, code); airline.TraveosSupported {
				t.Errorf("%s seeded as traveos_supported; nothing has been read from it", code)
			}
		}
		for _, code := range []string{"TAP", "COPA"} {
			if airline := airlineByCode(t, tx, code); !airline.TraveosSupported {
				t.Errorf("%s is off, but there is a ticket for it on the account", code)
			}
		}
		// Air France is "homologada" at 137 and issues through Traveos today; Iberia at 49
		// reads through it too.
		for code, wantID := range map[string]int{"AIRFRANCE": 137, "IBERIA": 49} {
			if airline := airlineByCode(t, tx, code); !airline.TraveosSupported ||
				airline.TraveosAirlineID == nil || *airline.TraveosAirlineID != wantID {
				t.Errorf("%s = supported %v, id %v, want supported at %d", code, airline.TraveosSupported, airline.TraveosAirlineID, wantID)
			}
		}

		// Traveos does not serve Aerolíneas at all: it is read from the airline alone, so
		// it has no catalog id and the vendor is never asked about it.
		if aerolineas := airlineByCode(t, tx, "AEROLINEAS"); aerolineas.TraveosAirlineID != nil ||
			aerolineas.TraveosSupported || !aerolineas.RequiresLastName ||
			aerolineas.BrandColor == nil || *aerolineas.BrandColor != "#0078BE" {
			t.Errorf("AEROLINEAS = id %v, supported %v, requires surname %v, colour %v; want no id, off, surname, #0078BE",
				aerolineas.TraveosAirlineID, aerolineas.TraveosSupported, aerolineas.RequiresLastName, aerolineas.BrandColor)
		}

		if latam := airlineByCode(t, tx, "LATAM"); !latam.RequiresPurchaseNumber {
			t.Error("LATAM seeded without requires_purchase_number")
		}

		// Eight of the nine carriers Traveos reaches need the surname to scrape the
		// airline's site; AZUL is the one that does not. Seeding this wrong is
		// expensive in both directions - too strict refuses an EMISSAO that was
		// fine, too loose spends a real emission the vendor cannot answer.
		for _, code := range []string{"GOL", "LATAM", "AA", "AVIANCA", "TAP", "COPA", "AIRFRANCE", "IBERIA"} {
			if airline := airlineByCode(t, tx, code); !airline.RequiresLastName {
				t.Errorf("%s seeded without requires_last_name", code)
			}
		}
		if azul := airlineByCode(t, tx, "AZUL"); azul.RequiresLastName {
			t.Error("AZUL seeded with requires_last_name; it is the one that does not need it")
		}

		// Only the two carriers read directly with the origin need it; demanding it of
		// the rest held up a lookup that never reads it.
		for _, code := range []string{"GOL", "AZUL"} {
			if airline := airlineByCode(t, tx, code); !airline.RequiresOrigin {
				t.Errorf("%s seeded without requires_origin", code)
			}
		}
		for _, code := range []string{"LATAM", "AA", "AVIANCA", "TAP", "COPA", "AIRFRANCE", "IBERIA"} {
			if airline := airlineByCode(t, tx, code); airline.RequiresOrigin {
				t.Errorf("%s seeded with requires_origin; its lookup does not read the origin", code)
			}
		}

		// The colours come from Traveos' own catalog, so a ticket prints in the
		// airline's colour before that carrier has ever been synced.
		if gol := airlineByCode(t, tx, "GOL"); gol.BrandColor == nil || *gol.BrandColor != "#FF5E00" {
			t.Errorf("GOL brand_color = %v, want #FF5E00 from the catalog", gol.BrandColor)
		}
	})
}

func TestSeedAirlinesFillsAnEmptyBrandColourButNeverReplacesOne(t *testing.T) {
	// A colour that is already there came from a previous seed or from a real
	// ticket the vendor answered with. Both beat rewriting it on every boot.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		own := "#123456"
		gol := Airline{Name: "GOL", Code: "GOL", BrandColor: &own}
		azul := Airline{Name: "AZUL", Code: "AZUL"}
		if err := tx.Create(&gol).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}
		if err := tx.Create(&azul).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		if got := airlineByCode(t, tx, "GOL"); got.BrandColor == nil || *got.BrandColor != own {
			t.Errorf("GOL brand_color = %v, want the existing %s left alone", got.BrandColor, own)
		}
		if got := airlineByCode(t, tx, "AZUL"); got.BrandColor == nil || *got.BrandColor != "#54B1E3" {
			t.Errorf("AZUL brand_color = %v, want it filled from the catalog", got.BrandColor)
		}
	})
}

func TestSeedAirlinesIsIdempotent(t *testing.T) {
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("first SeedAirlines: %v", err)
		}
		first := airlineByCode(t, tx, "GOL")

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("second SeedAirlines: %v", err)
		}

		if got := countAirlines(t, tx); got != 14 {
			t.Errorf("after seeding twice there are %d airlines, want 14", got)
		}
		if second := airlineByCode(t, tx, "GOL"); second.ID != first.ID {
			t.Error("re-seeding replaced the row instead of leaving it alone")
		}
	})
}

func TestSeedAirlinesCorrectsAnExistingRowInPlace(t *testing.T) {
	// The state this whole seeder exists for: a carrier typed in by hand, with a
	// code that does not match its ticket logo and no Traveos id at all.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		existing := Airline{Name: "Gol", Code: "Gol"}
		if err := tx.Create(&existing).Error; err != nil {
			t.Fatalf("seeding the hand-typed row: %v", err)
		}

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		if got := countAirlines(t, tx); got != 14 {
			t.Errorf("got %d airlines, want 14 - the existing GOL was duplicated", got)
		}

		gol := airlineByCode(t, tx, "GOL")
		// The uuid is what every issuance points at. Replacing the row instead of
		// updating it would orphan them all.
		if gol.ID != existing.ID {
			t.Errorf("GOL has a new uuid (%s, was %s); issuances would lose their airline",
				gol.ID, existing.ID)
		}
		if gol.TraveosAirlineID == nil || *gol.TraveosAirlineID != 3 {
			t.Errorf("traveos_airline_id = %v, want 3", gol.TraveosAirlineID)
		}
	})
}

func TestSeedAirlinesMatchesAMisspelledName(t *testing.T) {
	// "American Arilines" is in the database. Listing the typo as an alias is what
	// keeps it from becoming a second American Airlines beside the real one.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		existing := Airline{Name: "American Arilines", Code: "AA"}
		if err := tx.Create(&existing).Error; err != nil {
			t.Fatalf("seeding the misspelled row: %v", err)
		}

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		american := airlineByCode(t, tx, "AA")
		if american.ID != existing.ID {
			t.Error("the misspelled row was not recognised and a duplicate was created")
		}
		if american.Name != "AMERICAN AIRLINES" {
			t.Errorf("name = %q, want the corrected AMERICAN AIRLINES", american.Name)
		}
	})
}

func TestSeedAirlinesCorrectsTheAirlineRequirementFlags(t *testing.T) {
	// Both directions, on a row that already exists. This is the case the seeder is
	// for: the carriers already in the database were typed by hand and their flags
	// were never revisited.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		// Needs the surname, does not say so.
		gol := Airline{Name: "GOL", Code: "GOL", RequiresLastName: false}
		// Says it needs the surname, and does not.
		azul := Airline{Name: "AZUL", Code: "AZUL", RequiresLastName: true}
		if err := tx.Create(&gol).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}
		if err := tx.Create(&azul).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		if got := airlineByCode(t, tx, "GOL"); !got.RequiresLastName {
			t.Error("GOL was not given requires_last_name")
		}
		if got := airlineByCode(t, tx, "AZUL"); got.RequiresLastName {
			t.Error("AZUL kept requires_last_name; the catalog is authoritative in both directions")
		}
	})
}

func TestSeedAirlinesLeavesAnUnknownAirlineAlone(t *testing.T) {
	// The seeder only speaks about the twelve carriers it knows. Anything else in
	// the table was somebody's own entry and is none of its business.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		stranger := Airline{Name: "EMIRATES", Code: "EMIRATES", RequiresLastName: true}
		if err := tx.Create(&stranger).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		var reloaded Airline
		if err := tx.Where("id = ?", stranger.ID).First(&reloaded).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		if reloaded.Code != "EMIRATES" || !reloaded.RequiresLastName {
			t.Errorf("an airline outside the seed file was rewritten: %+v", reloaded)
		}
	})
}

func TestSeedAirlinesPromotesTraveosSupportButNeverDemotesIt(t *testing.T) {
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		// One of the five, left off - the seeder knows it emits and turns it on.
		off := Airline{Name: "AVIANCA", Code: "AVIANCA", TraveosSupported: false}
		// One of the seven, turned on by hand - a deliberate choice in the CRUD, and
		// the seeder has no business undoing it.
		on := Airline{Name: "QATAR AIRWAYS", Code: "QATAR", TraveosSupported: true}
		if err := tx.Create(&off).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}
		if err := tx.Create(&on).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}

		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		if avianca := airlineByCode(t, tx, "AVIANCA"); !avianca.TraveosSupported {
			t.Error("AVIANCA was not promoted to traveos_supported")
		}
		if qatar := airlineByCode(t, tx, "QATAR"); !qatar.TraveosSupported {
			t.Error("QATAR was demoted; the seeder must only ever promote")
		}
	})
}

func TestSeedLoyaltyProgramsCreatesTheMissingOnes(t *testing.T) {
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}
		if err := SeedLoyaltyPrograms(tx); err != nil {
			t.Fatalf("SeedLoyaltyPrograms: %v", err)
		}

		var count int64
		if err := tx.Model(&LoyaltyProgram{}).Count(&count).Error; err != nil {
			t.Fatalf("counting: %v", err)
		}
		if count != 4 {
			t.Errorf("seeded %d loyalty programs, want 4", count)
		}

		var smiles LoyaltyProgram
		if err := tx.Where("code = ?", "SMILES").First(&smiles).Error; err != nil {
			t.Fatalf("SMILES: %v", err)
		}
		gol := airlineByCode(t, tx, "GOL")
		if smiles.DefaultAirlineID != gol.ID {
			t.Error("SMILES' default airline is not GOL; the code was not resolved")
		}
	})
}

func TestSeedLoyaltyProgramsLeavesAnExistingProgramUntouched(t *testing.T) {
	// Accounts point at loyalty programs, so a duplicate scatters one program's
	// accounts across two rows. Nothing about an existing program is known to be
	// wrong, so matching one means leaving it entirely alone.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}

		azul := airlineByCode(t, tx, "AZUL")
		existing := LoyaltyProgram{
			Name:             "TudoAzul",
			Code:             "TUDOAZUL",
			Color:            "#123456",
			DefaultAirlineID: azul.ID,
		}
		if err := tx.Create(&existing).Error; err != nil {
			t.Fatalf("seeding: %v", err)
		}

		if err := SeedLoyaltyPrograms(tx); err != nil {
			t.Fatalf("SeedLoyaltyPrograms: %v", err)
		}

		var count int64
		if err := tx.Model(&LoyaltyProgram{}).Count(&count).Error; err != nil {
			t.Fatalf("counting: %v", err)
		}
		if count != 4 {
			t.Errorf("got %d programs, want 4 - TudoAzul was duplicated as AZUL", count)
		}

		var reloaded LoyaltyProgram
		if err := tx.Where("id = ?", existing.ID).First(&reloaded).Error; err != nil {
			t.Fatalf("reloading: %v", err)
		}
		if reloaded.Name != "TudoAzul" || reloaded.Code != "TUDOAZUL" || reloaded.Color != "#123456" {
			t.Errorf("the existing program was rewritten: %+v", reloaded)
		}
	})
}

func TestSeedLoyaltyProgramsDoesNotResurrectAnArchivedProgram(t *testing.T) {
	// The seeder runs on every boot. Without the Unscoped lookup it would re-create
	// each archived program with a fresh uuid, and the accounts left pointing at the
	// archived row would be stranded on a program the panel no longer lists.
	runOnEmptyCatalog(t, func(tx *gorm.DB) {
		if err := SeedAirlines(tx); err != nil {
			t.Fatalf("SeedAirlines: %v", err)
		}
		if err := SeedLoyaltyPrograms(tx); err != nil {
			t.Fatalf("SeedLoyaltyPrograms: %v", err)
		}

		var smiles LoyaltyProgram
		if err := tx.Where("code = ?", "SMILES").First(&smiles).Error; err != nil {
			t.Fatalf("SMILES: %v", err)
		}
		if err := tx.Delete(&smiles).Error; err != nil {
			t.Fatalf("archiving SMILES: %v", err)
		}

		if err := SeedLoyaltyPrograms(tx); err != nil {
			t.Fatalf("re-running SeedLoyaltyPrograms: %v", err)
		}

		var live int64
		if err := tx.Model(&LoyaltyProgram{}).Where("code = ?", "SMILES").Count(&live).Error; err != nil {
			t.Fatalf("counting live SMILES: %v", err)
		}
		if live != 0 {
			t.Errorf("SMILES came back after being archived: %d live rows", live)
		}

		var total int64
		if err := tx.Unscoped().Model(&LoyaltyProgram{}).Where("code = ?", "SMILES").Count(&total).Error; err != nil {
			t.Fatalf("counting every SMILES: %v", err)
		}
		if total != 1 {
			t.Errorf("got %d SMILES rows in total, want the archived one only", total)
		}
	})
}
