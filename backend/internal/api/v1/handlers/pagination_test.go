package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The three listings that gained a paginated /manage sibling. Each one keeps its
// unpaginated route because a picker somewhere reads it whole, so every test here
// checks both halves: the page is a window, and the plain route still is not.

type clientPaginatedResponse struct {
	Data schemas.ClientPaginatedData `json:"data"`
}

type clientListResponse struct {
	Data []schemas.ClientResponse `json:"data"`
}

type loyaltyDocumentPaginatedResponse struct {
	Data schemas.LoyaltyDocumentPaginatedData `json:"data"`
}

func decodeOK(t *testing.T, rec *httptest.ResponseRecorder, name string, into any) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status: got %d, want 200 (body: %s)", name, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("failed to decode the %s response: %v", name, err)
	}
}

func callManageClients(t *testing.T, tx *gorm.DB, query string) schemas.ClientPaginatedData {
	t.Helper()
	rec := httptest.NewRecorder()
	NewClientHandler(tx).ManageClients(rec, withScope(httptest.NewRequest(http.MethodGet, "/clients/manage?"+query, nil), access.FullScope()))

	var resp clientPaginatedResponse
	decodeOK(t, rec, "ManageClients", &resp)
	return resp.Data
}

func callListClients(t *testing.T, tx *gorm.DB, query string) []schemas.ClientResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	NewClientHandler(tx).ListClients(rec, withScope(httptest.NewRequest(http.MethodGet, "/clients?"+query, nil), access.FullScope()))

	var resp clientListResponse
	decodeOK(t, rec, "ListClients", &resp)
	return resp.Data
}

func callListLoyaltyDocuments(t *testing.T, tx *gorm.DB, query string) schemas.LoyaltyDocumentPaginatedData {
	t.Helper()
	rec := httptest.NewRecorder()
	NewLoyaltyDocumentHandler(tx).ListLoyaltyDocuments(
		rec, httptest.NewRequest(http.MethodGet, "/loyalty-documents/manage?"+query, nil))

	var resp loyaltyDocumentPaginatedResponse
	decodeOK(t, rec, "ListLoyaltyDocuments", &resp)
	return resp.Data
}

// seedAccountsForPaging creates three accounts whose holders sort ana < bruno < carla,
// inserted out of order so the listing has to sort rather than echo insertion order.
// The prefix makes the search term unique, which is what keeps these assertions from
// depending on whatever else the database already holds.
func seedAccountsForPaging(t *testing.T, tx *gorm.DB) (prefix string, programID string) {
	t.Helper()
	suffix := uuid.New().String()[:8]
	prefix = "zzz-" + suffix + "-"

	airline := data.Airline{Name: "Paging Air", Code: "P" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Paging " + suffix, Code: "PG" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)

	for _, name := range []string{"carla", "ana", "bruno"} {
		holder := data.Person{Name: prefix + name}
		create(t, tx, &holder)
		account := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
		create(t, tx, &account)
	}
	return prefix, program.ID
}

// /accounts stays whole on purpose: it feeds the pickers on the issuances grid, both
// sales screens and the importer, and a page there would silently truncate a combobox.
// parseListQuery reads page and limit for every listing, so "ignores them" is a claim
// worth a test rather than an assumption.
func TestGetAccounts_StaysUnpaginatedForThePickers(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix, _ := seedAccountsForPaging(t, tx)

		if got := callListAccounts(tx, "q="+prefix+"&limit=2&page=1"); len(got) != 3 {
			t.Fatalf("got %d accounts, want all 3 - limit and page must not narrow this route", len(got))
		}
	})
}

func seedClientsForPaging(t *testing.T, tx *gorm.DB) string {
	t.Helper()
	prefix := "zzz-" + uuid.New().String()[:8] + "-"

	for _, name := range []string{"carla", "ana", "bruno"} {
		client := data.Client{Name: prefix + name}
		create(t, tx, &client)
	}
	return prefix
}

func TestManageClients_PaginatesInAStableOrder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix := seedClientsForPaging(t, tx)

		first := callManageClients(t, tx, "q="+prefix+"&limit=2&page=1")
		if first.Total != 3 {
			t.Errorf("total: got %d, want 3", first.Total)
		}
		if len(first.Clients) != 2 {
			t.Fatalf("page 1: got %d clients, want 2", len(first.Clients))
		}
		// This listing had no ORDER BY at all before, so the rows came back in heap
		// order - which is exactly the state in which a page window repeats one row
		// and hides another.
		if first.Clients[0].Name != prefix+"ana" || first.Clients[1].Name != prefix+"bruno" {
			t.Fatalf("page 1: got %q and %q, want them sorted by name",
				first.Clients[0].Name, first.Clients[1].Name)
		}

		second := callManageClients(t, tx, "q="+prefix+"&limit=2&page=2")
		if len(second.Clients) != 1 || second.Clients[0].Name != prefix+"carla" {
			t.Fatalf("page 2: got %+v, want only %q", second.Clients, prefix+"carla")
		}
	})
}

// The phone is half of what a client row is, so "who is this number" has to work.
func TestClients_SearchByNameOrPhone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		suffix := uuid.New().String()[:8]
		phone := "5511900" + suffix[:5]
		client := data.Client{Name: "Cliente-" + suffix, Phone: &phone}
		create(t, tx, &client)

		byName := callListClients(t, tx, "q=cliente-"+suffix)
		if len(byName) != 1 || byName[0].ID != client.ID {
			t.Fatalf("searching by name returned %d rows, want the seeded client", len(byName))
		}

		byPhone := callManageClients(t, tx, "q="+phone)
		if byPhone.Total != 1 || byPhone.Clients[0].ID != client.ID {
			t.Fatalf("searching by phone returned %d rows, want the seeded client", byPhone.Total)
		}
	})
}

// /clients stays whole for the sales screen's client picker, the same bargain the
// accounts and people listings make.
func TestListClients_StaysUnpaginatedForThePicker(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix := seedClientsForPaging(t, tx)

		if got := callListClients(t, tx, "q="+prefix+"&limit=2"); len(got) != 3 {
			t.Fatalf("got %d clients, want all 3 - limit must not narrow this route", len(got))
		}
	})
}

func TestListLoyaltyDocuments_PaginatesAndKeepsTheActiveFilter(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		suffix := uuid.New().String()[:8]
		prefix := "zzz-" + suffix + "-"

		airline := data.Airline{Name: "Doc Air", Code: "D" + suffix}
		create(t, tx, &airline)
		program := data.LoyaltyProgram{Name: "Doc " + suffix, Code: "DC" + suffix, DefaultAirlineID: airline.ID}
		create(t, tx, &program)

		// Three distinct valid documents: the fingerprint is unique per programme, so
		// reusing one number here would fail the insert rather than the assertion.
		for _, seed := range []struct {
			name   string
			cpf    string
			active bool
		}{
			{"carla", "52998224725", true},
			{"ana", "71460238001", true},
			{"bruno", "11144477735", false},
		} {
			document := data.LoyaltyDocument{
				LoyaltyProgramID: program.ID,
				Name:             prefix + seed.name,
				CPF:              seed.cpf,
				Active:           seed.active,
			}
			create(t, tx, &document)
		}

		page := callListLoyaltyDocuments(t, tx, "q="+prefix+"&limit=2&page=1")
		if page.Total != 3 {
			t.Errorf("total: got %d, want 3", page.Total)
		}
		if len(page.Documents) != 2 {
			t.Fatalf("page 1: got %d documents, want 2", len(page.Documents))
		}

		onlyActive := callListLoyaltyDocuments(t, tx, "q="+prefix+"&active=true")
		if onlyActive.Total != 2 {
			t.Fatalf("active=true: got %d, want 2 - the filter must survive the count", onlyActive.Total)
		}
	})
}

func TestPaginatedListings_ClampTheLimitAndFallBackOnGarbage(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		cases := map[string]func(string) (int, int){
			"account-holders": func(q string) (int, int) {
				values, err := url.ParseQuery(q)
				if err != nil {
					t.Fatal(err)
				}
				got := callListAccountHolders(t, tx, values)
				return got.Page, got.Limit
			},
			"clients": func(q string) (int, int) {
				got := callManageClients(t, tx, q)
				return got.Page, got.Limit
			},
			"loyalty-documents": func(q string) (int, int) {
				got := callListLoyaltyDocuments(t, tx, q)
				return got.Page, got.Limit
			},
		}

		for name, call := range cases {
			if _, limit := call("limit=5000"); limit != listMaxLimit {
				t.Errorf("%s limit=5000: got %d, want it clamped to %d", name, limit, listMaxLimit)
			}
			page, limit := call("limit=abc&page=-3")
			if limit != listDefaultLimit || page != 1 {
				t.Errorf("%s garbage paging: got page=%d limit=%d, want 1 and %d",
					name, page, limit, listDefaultLimit)
			}
		}
	})
}
