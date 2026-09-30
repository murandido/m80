package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

type personListResponse struct {
	Data []schemas.PersonResponse `json:"data"`
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

// Operators type names without diacritics. Before unaccent() the listing answered that
// "Ágata Conceição" did not exist, and the same person was registered a second time -
// the duplicate the panel's comboboxes already stopped producing on their side.
func TestGetPeople_SearchesIgnoringAccents(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		suffix := uuid.New().String()[:8]
		person := data.Person{Name: "Ágata-Conceição-" + suffix}
		create(t, tx, &person)

		// The accented spelling is searched too: folding diacritics must widen what
		// matches, never move the goalposts for whoever types the name correctly.
		for _, term := range []string{
			"agata-conceicao-" + suffix,
			"Ágata-Conceição-" + suffix,
			"conceicao-" + suffix,
		} {
			found := callGetPeople(t, tx, "q="+url.QueryEscape(term))
			if len(found) != 1 || found[0].ID != person.ID {
				t.Errorf("searching %q returned %d rows, want exactly the seeded person", term, len(found))
			}
		}
	})
}

// nicknameMatch is the one search clause every listing shares, so it gets its own case:
// a regression here would silently narrow accounts, agencies and people at once.
func TestGetPeople_SearchesNicknamesIgnoringAccents(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		suffix := uuid.New().String()[:8]
		person := data.Person{Name: "Fulano " + suffix}
		create(t, tx, &person)

		alias := "Conceição-" + suffix
		nickname := data.PersonNickname{PersonID: person.ID, Nickname: alias}
		create(t, tx, &nickname)

		found := callGetPeople(t, tx, "q="+url.QueryEscape("conceicao-"+suffix))
		if len(found) != 1 || found[0].ID != person.ID {
			t.Fatalf("searching the unaccented nickname returned %d rows, want exactly the seeded person", len(found))
		}
	})
}

type accountListResponse struct {
	Data []schemas.AccountResponse `json:"data"`
}

func callGetAccounts(t *testing.T, tx *gorm.DB, query string) []schemas.AccountResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	NewAccountHandler(tx).GetAccounts(rec, httptest.NewRequest(http.MethodGet, "/accounts?"+query, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GetAccounts status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp accountListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode GetAccounts response: %v", err)
	}
	return resp.Data
}

// The panel labels an account by holder, then login, then alias. Ordering has to
// follow the same chain: sorting on holder and login alone drops an alias-named
// account into the NULL bucket at the end, and the list reads as unsorted.
func TestGetAccounts_OrdersAliasNamedAccountsAlongsideTheRest(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		airline := data.Airline{
			Name: "CIA " + uuid.New().String()[:8],
			Code: uuid.New().String()[:6],
		}
		create(t, tx, &airline)

		program := data.LoyaltyProgram{
			Name:             "Programa " + uuid.New().String()[:8],
			Code:             uuid.New().String()[:6],
			DefaultAirlineID: airline.ID,
		}
		create(t, tx, &program)

		// Holder-named account sorting after the alias-named one alphabetically.
		holder := data.Person{Name: "zzz-holder-" + uuid.New().String()[:8]}
		create(t, tx, &holder)
		withHolder := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
		create(t, tx, &withHolder)

		// Alias-named account: no holder, no login. Before the fix this sorted last
		// no matter what its alias said.
		aliasOnly := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID}
		create(t, tx, &aliasOnly)
		alias := data.AccountNickname{
			AccountID:        aliasOnly.ID,
			LoyaltyProgramID: program.ID,
			Nickname:         "aaa-alias-" + uuid.New().String()[:8],
		}
		create(t, tx, &alias)

		var seenAlias, seenHolder int
		accounts := callGetAccounts(t, tx, "loyalty_program_id="+program.ID)
		for i, account := range accounts {
			switch account.ID {
			case aliasOnly.ID:
				seenAlias = i
			case withHolder.ID:
				seenHolder = i
			}
		}

		if len(accounts) != 2 {
			t.Fatalf("got %d accounts for the seeded program, want 2", len(accounts))
		}
		if seenAlias > seenHolder {
			t.Errorf(
				"the account named %q sorted after %q; alias-named accounts must sort by their alias, not land in the NULL bucket",
				alias.Nickname, holder.Name,
			)
		}
	})
}

// The database collates by byte value, so an uppercase Z sorts before a lowercase a
// and an accented name lands after z. Neither reads as alphabetical to a Portuguese
// speaker, which is what these listings are sorted for.
func TestGetPeople_SortsCaseAndAccentInsensitively(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		prefix := "zzz-" + uuid.New().String()[:8] + "-"
		names := []string{prefix + "Bruno", prefix + "ana", prefix + "Ágata", prefix + "Zeca"}
		for _, name := range names {
			person := data.Person{Name: name}
			create(t, tx, &person)
		}

		wanted := map[string]bool{}
		for _, name := range names {
			wanted[name] = true
		}

		got := namesOf(callGetPeople(t, tx, ""), wanted)
		expected := []string{
			prefix + "Ágata",
			prefix + "ana",
			prefix + "Bruno",
			prefix + "Zeca",
		}

		if len(got) != len(expected) {
			t.Fatalf("got %d seeded people, want %d: %v", len(got), len(expected), got)
		}
		for i := range expected {
			if got[i] != expected[i] {
				t.Fatalf("collation order: got %v, want %v", got, expected)
			}
		}
	})
}

type airportPaginatedResponse struct {
	Data schemas.AirportPaginatedData `json:"data"`
}

func callListAirports(t *testing.T, tx *gorm.DB, query string) schemas.AirportPaginatedData {
	t.Helper()
	rec := httptest.NewRecorder()
	NewAirportHandler(tx).ListAirports(rec, httptest.NewRequest(http.MethodGet, "/airports/manage?"+query, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ListAirports status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp airportPaginatedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode ListAirports response: %v", err)
	}
	return resp.Data
}

// Airports rank by relevance when searching, which is the right default - but an
// explicit sort has to win, or asking for Z–A and getting closest-match-first looks
// like the control did nothing.
func TestListAirports_ExplicitOrderBeatsSearchRelevance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		token := strings.ToUpper(uuid.New().String()[:6])
		// The first is matched by its code, the second only by its name - so under
		// relevance ordering the code match always comes first.
		//
		// The search term has to be the whole code, not a slice of it: relevance
		// ranks a code hit above a name hit, but a row whose code merely shares a
		// prefix with the term is a name hit like any other, and the two then tie
		// and fall back to `iata_code ASC`. Both codes are also longer than a real
		// IATA code, so neither can collide with a seeded airport on the unique
		// index - and `other` keeps the second row out of the code bucket.
		other := strings.ToUpper(uuid.New().String()[:6])
		codeMatch := data.Airport{IATACode: token, Name: "Zzz " + token, Country: "BR"}
		nameMatch := data.Airport{IATACode: other, Name: "Aaa " + token, Country: "BR"}
		create(t, tx, &codeMatch)
		create(t, tx, &nameMatch)

		relevance := callListAirports(t, tx, "q="+token)
		if len(relevance.Airports) < 2 || relevance.Airports[0].IATACode != codeMatch.IATACode {
			t.Fatalf("relevance order should lead with the code match, got %+v", relevance.Airports)
		}

		byName := callListAirports(t, tx, "q="+token+"&order=name_asc")
		if len(byName.Airports) < 2 || byName.Airports[0].IATACode != nameMatch.IATACode {
			t.Errorf("order=name_asc should override relevance, got %+v", byName.Airports)
		}

		byCodeDesc := callListAirports(t, tx, "q="+token+"&order=code_desc")
		first, second := byCodeDesc.Airports[0].IATACode, byCodeDesc.Airports[1].IATACode
		if first < second {
			t.Errorf("order=code_desc: got %q before %q", first, second)
		}
	})
}
