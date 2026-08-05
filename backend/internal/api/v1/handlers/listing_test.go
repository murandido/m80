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

type personListResponse struct {
	Data []schemas.PersonResponse `json:"data"`
}

type personPaginatedResponse struct {
	Data schemas.PersonPaginatedData `json:"data"`
}

func callGetPeople(t *testing.T, tx *gorm.DB, query string) []schemas.PersonResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	NewPeopleHandler(tx).GetPeople(rec, httptest.NewRequest(http.MethodGet, "/people?"+query, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GetPeople status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp personListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode GetPeople response: %v", err)
	}
	return resp.Data
}

func callListPeople(t *testing.T, tx *gorm.DB, query string) schemas.PersonPaginatedData {
	t.Helper()
	rec := httptest.NewRecorder()
	NewPeopleHandler(tx).ListPeople(rec, httptest.NewRequest(http.MethodGet, "/people/manage?"+query, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ListPeople status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp personPaginatedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode ListPeople response: %v", err)
	}
	return resp.Data
}

// namesOf keeps only the seeded names, so a database that already holds unrelated
// people does not make the assertions depend on fixture data we did not create.
func namesOf(people []schemas.PersonResponse, wanted map[string]bool) []string {
	var got []string
	for _, person := range people {
		if wanted[person.Name] {
			got = append(got, person.Name)
		}
	}
	return got
}

func seedPeopleForSorting(t *testing.T, tx *gorm.DB) (prefix string, wanted map[string]bool) {
	t.Helper()
	prefix = "zzz-" + uuid.New().String()[:8] + "-"

	// Inserted out of alphabetical order on purpose: the listing has to sort them,
	// not echo the insertion order back.
	for _, suffix := range []string{"carla", "ana", "bruno"} {
		person := data.Person{Name: prefix + suffix}
		create(t, tx, &person)
	}

	return prefix, map[string]bool{
		prefix + "ana":   true,
		prefix + "bruno": true,
		prefix + "carla": true,
	}
}

func TestGetPeople_OrdersAlphabeticallyByDefault(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix, wanted := seedPeopleForSorting(t, tx)

		got := namesOf(callGetPeople(t, tx, ""), wanted)
		expected := []string{prefix + "ana", prefix + "bruno", prefix + "carla"}

		if len(got) != len(expected) {
			t.Fatalf("got %d seeded people, want %d: %v", len(got), len(expected), got)
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Fatalf("default order: got %v, want %v", got, expected)
			}
		}
	})
}

func TestGetPeople_HonoursTheDescendingOrder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix, wanted := seedPeopleForSorting(t, tx)

		got := namesOf(callGetPeople(t, tx, "order=name_desc"), wanted)
		expected := []string{prefix + "carla", prefix + "bruno", prefix + "ana"}

		for i := range expected {
			if i >= len(got) || got[i] != expected[i] {
				t.Fatalf("name_desc: got %v, want %v", got, expected)
			}
		}
	})
}

// The order key is client-supplied and ends up in an ORDER BY, so it is resolved
// against a whitelist and never interpolated. An unknown key - whether a typo or an
// injection attempt - has to fall back to the default instead of erroring or executing.
func TestGetPeople_RejectsAnUnknownOrderKeyWithoutFailing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix, wanted := seedPeopleForSorting(t, tx)
		expected := []string{prefix + "ana", prefix + "bruno", prefix + "carla"}

		for _, order := range []string{
			"nonsense",
			"name_asc; DROP TABLE people",
			"' OR 1=1--",
			"people.name DESC",
		} {
			got := namesOf(callGetPeople(t, tx, "order="+url.QueryEscape(order)), wanted)
			for i := range expected {
				if i >= len(got) || got[i] != expected[i] {
					t.Fatalf("order=%q: got %v, want the default order %v", order, got, expected)
				}
			}
		}
	})
}

func TestGetPeople_SearchesByNicknameAndNotOnlyByName(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		person := data.Person{Name: "Fulano " + uuid.New().String()[:8]}
		create(t, tx, &person)

		alias := "apelido-" + uuid.New().String()[:8]
		nickname := data.PersonNickname{PersonID: person.ID, Nickname: alias}
		create(t, tx, &nickname)

		found := callGetPeople(t, tx, "q="+alias)
		if len(found) != 1 || found[0].ID != person.ID {
			t.Fatalf("searching by nickname %q returned %d rows, want exactly the seeded person", alias, len(found))
		}

		// The nickname must not widen the search into a match-anything wildcard.
		if others := callGetPeople(t, tx, "q="+uuid.New().String()); len(others) != 0 {
			t.Errorf("a term matching nothing returned %d rows", len(others))
		}
	})
}

func TestListPeople_PaginatesAndCountsTheFilteredSet(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix, _ := seedPeopleForSorting(t, tx)

		first := callListPeople(t, tx, "q="+prefix+"&limit=2&page=1")
		if first.Total != 3 {
			t.Errorf("total: got %d, want 3 — the count must apply the same filter as the page", first.Total)
		}
		if len(first.People) != 2 {
			t.Fatalf("page 1: got %d people, want 2", len(first.People))
		}
		if first.People[0].Name != prefix+"ana" {
			t.Errorf("page 1 starts at %q, want %q", first.People[0].Name, prefix+"ana")
		}

		second := callListPeople(t, tx, "q="+prefix+"&limit=2&page=2")
		if len(second.People) != 1 {
			t.Fatalf("page 2: got %d people, want 1", len(second.People))
		}
		if second.People[0].Name != prefix+"carla" {
			t.Errorf("page 2 holds %q, want %q", second.People[0].Name, prefix+"carla")
		}
	})
}

func TestListPeople_ClampsTheLimitAndFallsBackOnGarbage(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		if got := callListPeople(t, tx, "limit=5000"); got.Limit != listMaxLimit {
			t.Errorf("limit=5000: got %d, want it clamped to %d", got.Limit, listMaxLimit)
		}
		if got := callListPeople(t, tx, "limit=abc&page=-3"); got.Limit != listDefaultLimit || got.Page != 1 {
			t.Errorf("garbage paging: got limit=%d page=%d, want %d and 1", got.Limit, got.Page, listDefaultLimit)
		}
	})
}
