package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callListAccountHolders(t *testing.T, tx *gorm.DB, query url.Values) schemas.AccountHolderPaginatedData {
	t.Helper()
	handler := NewAccountHandler(tx)

	req := httptest.NewRequest(http.MethodGet, "/accounts/holders?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	handler.ListAccountHolders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Data schemas.AccountHolderPaginatedData `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Data
}

// holderWorld is a small registry isolated from whatever else the test database holds by
// a marker every name and login carries, so a search for it returns only these rows.
type holderWorld struct {
	marker   string
	smiles   data.LoyaltyProgram
	azul     data.LoyaltyProgram
	fulana   data.Person // holds a Smiles and an Azul account
	beltrano data.Person // holds nothing - an administrator
}

func seedHolderWorld(t *testing.T, tx *gorm.DB) holderWorld {
	t.Helper()
	marker := "hw" + uuid.New().String()[:8]

	world := holderWorld{
		marker:   marker,
		smiles:   seedProgram(t, tx),
		azul:     seedProgram(t, tx),
		fulana:   seedPerson(t, tx, "Ana "+marker),
		beltrano: seedPerson(t, tx, "Bruno "+marker),
	}

	smilesLogin, azulLogin := "a-smiles-"+marker, "a-azul-"+marker
	create(t, tx, &data.Account{LoyaltyProgramID: world.smiles.ID, HolderID: &world.fulana.ID, Login: &smilesLogin, AdministratorID: &world.beltrano.ID})
	create(t, tx, &data.Account{LoyaltyProgramID: world.azul.ID, HolderID: &world.fulana.ID, Login: &azulLogin, Category: data.AccountCategoryInjunction})

	return world
}

func TestListAccountHolders_GroupsAccountsUnderTheirHolder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		page := callListAccountHolders(t, tx, url.Values{"q": {world.marker}})

		if page.Total != 2 || len(page.Rows) != 2 {
			t.Fatalf("rows: got total=%d len=%d, want 2 (Ana, Bruno)", page.Total, len(page.Rows))
		}

		// name_asc: "Ana …" then "Bruno …".
		ana, bruno := page.Rows[0], page.Rows[1]
		if ana.Holder == nil || ana.Holder.ID != world.fulana.ID {
			t.Fatalf("first row: got %+v, want Ana", ana.Holder)
		}
		if len(ana.Accounts) != 2 {
			t.Errorf("Ana's accounts: got %d, want 2", len(ana.Accounts))
		}
		if bruno.Holder == nil || bruno.Holder.ID != world.beltrano.ID || len(bruno.Accounts) != 0 {
			t.Errorf("second row: want Bruno with no accounts, got %+v / %d accounts", bruno.Holder, len(bruno.Accounts))
		}
	})
}

func TestListAccountHolders_ProgramFilterKeepsTheWholeRow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		page := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "loyalty_program_id": {world.azul.ID}})

		// Only Ana holds an Azul account; Bruno holds nothing.
		if page.Total != 1 || len(page.Rows) != 1 {
			t.Fatalf("rows: got total=%d len=%d, want 1", page.Total, len(page.Rows))
		}
		// The filter picks the row, not the accounts in it: her Smiles comes along.
		if len(page.Rows[0].Accounts) != 2 {
			t.Errorf("accounts in the row: got %d, want 2", len(page.Rows[0].Accounts))
		}
	})
}

func TestListAccountHolders_SearchReachesAccountColumns(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		page := callListAccountHolders(t, tx, url.Values{"q": {"a-azul-" + world.marker}})
		if page.Total != 1 || page.Rows[0].Holder == nil || page.Rows[0].Holder.ID != world.fulana.ID {
			t.Fatalf("search by login: got total=%d, want Ana's row", page.Total)
		}
	})
}

func TestListAccountHolders_IncompleteFilterCoversPersonAndAccounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		incomplete := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "complete": {"false"}})
		if incomplete.Total != 2 {
			t.Errorf("complete=false: got %d, want 2 - nothing seeded here is complete", incomplete.Total)
		}

		complete := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "complete": {"true"}})
		if complete.Total != 0 {
			t.Errorf("complete=true: got %d, want 0", complete.Total)
		}
	})
}

func TestListAccountHolders_PaginatesRowsNotAccounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		first := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "limit": {"1"}, "page": {"1"}})
		if first.Total != 2 || len(first.Rows) != 1 || len(first.Rows[0].Accounts) != 2 {
			t.Fatalf("page 1: got total=%d rows=%d, want 2 total and Ana's full row", first.Total, len(first.Rows))
		}

		second := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "limit": {"1"}, "page": {"2"}})
		if len(second.Rows) != 1 || second.Rows[0].Holder == nil || second.Rows[0].Holder.ID != world.beltrano.ID {
			t.Fatalf("page 2: want Bruno's row")
		}
	})
}

func TestListAccountHolders_FlagsRepeatedProgramsAndAdminConflicts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		// Ana gets a second live Smiles, and her Azul a different administrator than her Smiles.
		secondLogin := "a-smiles-2-" + world.marker
		create(t, tx, &data.Account{LoyaltyProgramID: world.smiles.ID, HolderID: &world.fulana.ID, Login: &secondLogin})
		otherAdmin := seedPerson(t, tx, "Carla "+world.marker)
		if err := tx.Model(&data.Account{}).
			Where("holder_id = ? AND loyalty_program_id = ?", world.fulana.ID, world.azul.ID).
			UpdateColumn("administrator_id", otherAdmin.ID).Error; err != nil {
			t.Fatal(err)
		}

		dup := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "duplicate_programs_only": {"true"}})
		if dup.Total != 1 || dup.Rows[0].Holder == nil || dup.Rows[0].Holder.ID != world.fulana.ID {
			t.Fatalf("duplicate_programs_only: got %d rows, want only Ana", dup.Total)
		}
		if dup.DuplicateProgramsTotal < 1 {
			t.Errorf("duplicate_programs_total: got %d", dup.DuplicateProgramsTotal)
		}

		conflicts := callListAccountHolders(t, tx, url.Values{"q": {world.marker}, "admin_conflicts_only": {"true"}})
		if conflicts.Total != 1 || conflicts.Rows[0].Holder.ID != world.fulana.ID {
			t.Fatalf("admin_conflicts_only: got %d rows, want only Ana", conflicts.Total)
		}
		if conflicts.AdminConflictsTotal < 1 {
			t.Errorf("admin_conflicts_total: got %d", conflicts.AdminConflictsTotal)
		}
	})
}
