package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// seedProgramAccount creates an account in the program with the given code. The code is the
// real one ("AZUL", "SMILES") because that is what grants and the portal check; it is unique,
// so the program is reused when the transaction already has it. Two parallel tests creating
// the same code wait on each other's rollback instead of colliding.
func seedProgramAccount(t *testing.T, tx *gorm.DB, code string) data.Account {
	t.Helper()
	var program data.LoyaltyProgram
	if err := tx.Where("code = ?", code).First(&program).Error; err != nil {
		suffix := uuid.New().String()[:8]
		airline := data.Airline{Name: "Air " + suffix, Code: "T" + suffix}
		create(t, tx, &airline)
		program = data.LoyaltyProgram{Name: code, Code: code, DefaultAirlineID: airline.ID}
		create(t, tx, &program)
	}
	//nolint:gosec // G101: fixed test fixture, not a real credential
	login, password := "12345678900", "senha-da-conta"
	account := data.Account{
		HolderID:         testHolder(t, tx),
		LoyaltyProgramID: program.ID,
		TracksStatement:  true,
		Login:            &login,
		Password:         &password,
	}
	create(t, tx, &account)
	return account
}

func newClientBody(accounts string) string {
	phone := fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	body := `{"name": "Cliente", "phone": "` + phone + `", "password": "secret1", "role": "statement_client"`
	if accounts != "" {
		body += `, "account_ids": ` + accounts
	}
	return body + "}"
}

func accountGrantsOf(t *testing.T, tx *gorm.DB, userID string) []string {
	t.Helper()
	var ids []string
	if err := tx.Model(&data.UserAccount{}).Where("user_id = ?", userID).Order("account_id").Pluck("account_id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	return ids
}

func decodeUser(t *testing.T, body []byte) schemas.UserResponse {
	t.Helper()
	var resp struct {
		Data schemas.UserResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	return resp.Data
}

func TestUsers_AdminGrantsAccountsToAStatementClient(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		admin := data.User{ID: createUser(t, tx, "Admin").ID, Role: data.RoleAdmin}
		azul := seedProgramAccount(t, tx, "AZUL")
		smiles := seedProgramAccount(t, tx, "SMILES")

		create := (*UserHandler).CreateUser
		rec := callUserRoute(tx, create, admin, http.MethodPost, "", newClientBody(`["`+azul.ID+`", "`+smiles.ID+`"]`))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d: %s", rec.Code, rec.Body.String())
		}
		created := decodeUser(t, rec.Body.Bytes())
		want := []string{azul.ID, smiles.ID}
		slices.Sort(want)
		if got := accountGrantsOf(t, tx, created.ID); !slices.Equal(got, want) {
			t.Fatalf("grants = %v, want %v", got, want)
		}
		got := slices.Clone(created.AccountIDs)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("response account_ids = %v, want %v", created.AccountIDs, want)
		}

		// Replacing with one account drops the other.
		update := (*UserHandler).UpdateUser
		rec = callUserRoute(tx, update, admin, http.MethodPut, created.ID, `{"account_ids": ["`+azul.ID+`"]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update: got %d: %s", rec.Code, rec.Body.String())
		}
		if got := accountGrantsOf(t, tx, created.ID); !slices.Equal(got, []string{azul.ID}) {
			t.Fatalf("grants after update = %v, want [%s]", got, azul.ID)
		}

		// Turning the client into staff clears the grants: they would come back into force
		// on a later change back otherwise.
		rec = callUserRoute(tx, update, admin, http.MethodPut, created.ID, `{"role": "user"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("role change: got %d: %s", rec.Code, rec.Body.String())
		}
		if got := accountGrantsOf(t, tx, created.ID); len(got) != 0 {
			t.Fatalf("grants after becoming staff = %v, want none", got)
		}
	})
}

func TestUsers_AccountGrantsAreRejectedOutsideTheirRoleAndProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		admin := data.User{ID: createUser(t, tx, "Admin").ID, Role: data.RoleAdmin}
		azul := seedProgramAccount(t, tx, "AZUL")
		latam := seedProgramAccount(t, tx, "LATAM")

		create := (*UserHandler).CreateUser
		staffWithAccounts := newUserBody("user", "")
		staffWithAccounts = staffWithAccounts[:len(staffWithAccounts)-1] + `, "account_ids": ["` + azul.ID + `"]}`
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", staffWithAccounts); rec.Code != http.StatusBadRequest {
			t.Errorf("accounts on a staff user: got %d, want 400", rec.Code)
		}
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", newClientBody(`["`+latam.ID+`"]`)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a LATAM account: got %d, want 422", rec.Code)
		}
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", newClientBody(`["`+uuid.New().String()+`"]`)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("an unknown account: got %d, want 422", rec.Code)
		}
		if err := tx.Delete(&azul).Error; err != nil {
			t.Fatal(err)
		}
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", newClientBody(`["`+azul.ID+`"]`)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("a deleted account: got %d, want 422", rec.Code)
		}
	})
}

// GET /users feeds the issuer pickers; an outside customer must not appear there.
func TestUsers_ListHidesStatementClientsUnlessAsked(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := createUser(t, tx, "Cliente Extrato "+uuid.New().String()[:8])
		if err := tx.Model(&client).Update("role", data.RoleStatementClient).Error; err != nil {
			t.Fatal(err)
		}

		listed := func(query string) bool {
			req := httptest.NewRequest(http.MethodGet, "/users?"+query, nil)
			req = req.WithContext(asUser(req.Context(), data.User{ID: client.ID, Role: data.RoleAdmin}))
			rec := httptest.NewRecorder()
			NewUserHandler(tx).GetUsers(rec, req)
			var resp struct {
				Data []schemas.UserResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			for _, user := range resp.Data {
				if user.ID == client.ID {
					return true
				}
			}
			return false
		}

		if listed("") {
			t.Error("default list includes a statement client")
		}
		if !listed("include_clients=true") {
			t.Error("include_clients=true leaves the statement client out")
		}
		if !listed("role=statement_client") {
			t.Error("role=statement_client leaves the statement client out")
		}
	})
}
