package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func withIDs(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for key, value := range params {
		rctx.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func postPersonNickname(tx *gorm.DB, personID, nickname string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nickname":"`+nickname+`"}`))
	rec := httptest.NewRecorder()
	NewPersonNicknameHandler(tx).CreatePersonNickname(rec, withIDs(req, map[string]string{"id": personID}))
	return rec
}

func postAccountNickname(tx *gorm.DB, account data.Account, nickname string) *httptest.ResponseRecorder {
	body := `{"nickname":"` + nickname + `","loyalty_program_id":"` + account.LoyaltyProgramID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewAccountNicknameHandler(tx).CreateAccountNickname(rec, withIDs(req, map[string]string{"id": account.ID}))
	return rec
}

func deleteAccountNickname(tx *gorm.DB, accountID, nicknameID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	NewAccountNicknameHandler(tx).DeleteAccountNickname(rec,
		withIDs(req, map[string]string{"id": accountID, "nicknameID": nicknameID}))
	return rec
}

func personNicknames(t *testing.T, tx *gorm.DB, personID string) []string {
	t.Helper()
	var names []string
	if err := tx.Model(&data.PersonNickname{}).Where("person_id = ?", personID).Pluck("nickname", &names).Error; err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

func accountNicknames(t *testing.T, tx *gorm.DB, accountID string) []string {
	t.Helper()
	var names []string
	if err := tx.Model(&data.AccountNickname{}).Where("account_id = ?", accountID).Pluck("nickname", &names).Error; err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

// sharedWorld is a holder with a Smiles and an Azul account.
type sharedWorld struct {
	holder data.Person
	smiles data.Account
	azul   data.Account
}

func seedSharedWorld(t *testing.T, tx *gorm.DB) sharedWorld {
	t.Helper()
	world := sharedWorld{holder: seedPerson(t, tx, "Holder "+uuid.New().String()[:8])}
	world.smiles = data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &world.holder.ID}
	create(t, tx, &world.smiles)
	world.azul = data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &world.holder.ID}
	create(t, tx, &world.azul)
	return world
}

func equal(a, b []string) bool {
	return strings.Join(a, "|") == strings.Join(b, "|")
}

func TestSharedNicknames_PersonAliasReachesEveryAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedSharedWorld(t, tx)
		alias := "sn" + uuid.New().String()[:8]

		if rec := postPersonNickname(tx, world.holder.ID, alias); rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d (%s)", rec.Code, rec.Body.String())
		}

		want := []string{alias}
		for _, account := range []data.Account{world.smiles, world.azul} {
			if got := accountNicknames(t, tx, account.ID); !equal(got, want) {
				t.Errorf("account %s: got %v, want %v", account.ID, got, want)
			}
		}
	})
}

func TestSharedNicknames_AccountAliasReachesHolderAndSiblings(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedSharedWorld(t, tx)
		alias := "sn" + uuid.New().String()[:8]

		if rec := postAccountNickname(tx, world.smiles, alias); rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d (%s)", rec.Code, rec.Body.String())
		}

		want := []string{alias}
		if got := personNicknames(t, tx, world.holder.ID); !equal(got, want) {
			t.Errorf("holder: got %v, want %v", got, want)
		}
		if got := accountNicknames(t, tx, world.azul.ID); !equal(got, want) {
			t.Errorf("sibling account: got %v, want %v", got, want)
		}
	})
}

func TestSharedNicknames_RemovingFromOneAccountRemovesFromTheRow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedSharedWorld(t, tx)
		alias := "sn" + uuid.New().String()[:8]
		if rec := postPersonNickname(tx, world.holder.ID, alias); rec.Code != http.StatusCreated {
			t.Fatalf("seed: %d", rec.Code)
		}

		var onAzul data.AccountNickname
		if err := tx.First(&onAzul, "account_id = ?", world.azul.ID).Error; err != nil {
			t.Fatal(err)
		}
		if rec := deleteAccountNickname(tx, world.azul.ID, onAzul.ID); rec.Code != http.StatusOK {
			t.Fatalf("delete: got %d (%s)", rec.Code, rec.Body.String())
		}

		if got := personNicknames(t, tx, world.holder.ID); len(got) != 0 {
			t.Errorf("holder still has %v", got)
		}
		if got := accountNicknames(t, tx, world.smiles.ID); len(got) != 0 {
			t.Errorf("sibling still has %v", got)
		}
	})
}

func TestSharedNicknames_ConflictOnAnyAccountWritesNothing(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedSharedWorld(t, tx)
		alias := "sn" + uuid.New().String()[:8]

		// Somebody else's Azul account already answers to it.
		other := seedPerson(t, tx, "Other "+uuid.New().String()[:8])
		otherAzul := data.Account{LoyaltyProgramID: world.azul.LoyaltyProgramID, HolderID: &other.ID}
		create(t, tx, &otherAzul)
		create(t, tx, &data.AccountNickname{AccountID: otherAzul.ID, LoyaltyProgramID: otherAzul.LoyaltyProgramID, Nickname: alias})

		rec := postPersonNickname(tx, world.holder.ID, alias)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}
		if got := personNicknames(t, tx, world.holder.ID); len(got) != 0 {
			t.Errorf("holder got %v despite the refusal", got)
		}
		if got := accountNicknames(t, tx, world.smiles.ID); len(got) != 0 {
			t.Errorf("Smiles got %v despite the refusal", got)
		}
	})
}

func TestSharedNicknames_NewAccountInheritsTheHolderAliases(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedSharedWorld(t, tx)
		alias := "sn" + uuid.New().String()[:8]
		if rec := postPersonNickname(tx, world.holder.ID, alias); rec.Code != http.StatusCreated {
			t.Fatalf("seed: %d", rec.Code)
		}

		program := seedProgram(t, tx)
		rec := callCreateAccount(tx, `{"loyalty_program_id":"`+program.ID+`","holder_id":"`+world.holder.ID+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d (%s)", rec.Code, rec.Body.String())
		}

		var created data.Account
		if err := tx.First(&created, "holder_id = ? AND loyalty_program_id = ?", world.holder.ID, program.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got := accountNicknames(t, tx, created.ID); !equal(got, []string{alias}) {
			t.Errorf("new account: got %v, want [%s]", got, alias)
		}
	})
}
