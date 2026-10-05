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
		if ana.Holder.ID == "" || ana.Holder.ID != world.fulana.ID {
			t.Fatalf("first row: got %+v, want Ana", ana.Holder)
		}
		if len(ana.Accounts) != 2 {
			t.Errorf("Ana's accounts: got %d, want 2", len(ana.Accounts))
		}
		if bruno.Holder.ID == "" || bruno.Holder.ID != world.beltrano.ID || len(bruno.Accounts) != 0 {
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
		if page.Total != 1 || page.Rows[0].Holder.ID == "" || page.Rows[0].Holder.ID != world.fulana.ID {
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
		if len(second.Rows) != 1 || second.Rows[0].Holder.ID == "" || second.Rows[0].Holder.ID != world.beltrano.ID {
			t.Fatalf("page 2: want Bruno's row")
		}
	})
}

// Searching an administrator's name returns every holder whose account they run. The
// administrator is the one being looked for, so her own row comes first, even though
// alphabetically the holders she administers ("Ana …") come before her ("Bruno …").
func TestListAccountHolders_SearchPutsTheNamedPersonFirst(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)

		page := callListAccountHolders(t, tx, url.Values{"q": {world.beltrano.Name}})
		if len(page.Rows) != 2 {
			t.Fatalf("rows: got %d, want Bruno and the Ana he administers", len(page.Rows))
		}
		if page.Rows[0].Holder.ID == "" || page.Rows[0].Holder.ID != world.beltrano.ID {
			t.Errorf("first row: got %+v, want Bruno himself", page.Rows[0].Holder)
		}
		if page.Rows[1].Holder.ID == "" || page.Rows[1].Holder.ID != world.fulana.ID {
			t.Errorf("second row: got %+v, want Ana, matched through her account's administrator", page.Rows[1].Holder)
		}
	})
}

// The same name, typed as it is, beats one that starts with it, which beats one with a word
// starting with it, which beats one that merely contains it - whatever the chosen order,
// which only settles rows that matched the same way.
func TestListAccountHolders_SearchRanksExactThenPrefixThenContains(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		term := "rk" + uuid.New().String()[:8]
		contains := seedPerson(t, tx, "Zz"+term)
		wordPrefix := seedPerson(t, tx, "Maria "+term+"son")
		prefix := seedPerson(t, tx, term+" Souza")
		exact := seedPerson(t, tx, term)

		for _, order := range []string{"name_asc", "name_desc", "created_desc"} {
			page := callListAccountHolders(t, tx, url.Values{"q": {term}, "order": {order}})
			want := []string{exact.ID, prefix.ID, wordPrefix.ID, contains.ID}
			if len(page.Rows) != len(want) {
				t.Fatalf("%s: rows: got %d, want %d", order, len(page.Rows), len(want))
			}
			for i, id := range want {
				if page.Rows[i].Holder.ID == "" || page.Rows[i].Holder.ID != id {
					t.Errorf("%s: row %d: got %+v, want %s", order, i, page.Rows[i].Holder, id)
				}
			}
		}
	})
}
