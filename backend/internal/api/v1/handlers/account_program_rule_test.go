package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func TestCreateAccount_RefusesASecondAccountInTheSameProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		program := seedProgram(t, tx)
		holder := seedPerson(t, tx, "Holder")
		body := `{"loyalty_program_id":"` + program.ID + `","holder_id":"` + holder.ID + `"}`

		if rec := callCreateAccount(tx, body); rec.Code != http.StatusCreated {
			t.Fatalf("first: got %d (%s)", rec.Code, rec.Body.String())
		}
		rec := callCreateAccount(tx, body)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "já tem uma conta") {
			t.Fatalf("second: got %d %s, want 422 naming the program", rec.Code, rec.Body.String())
		}
	})
}

func TestUpdateAccount_RefusesMovingIntoATakenProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		smiles, azul := seedProgram(t, tx), seedProgram(t, tx)
		holder := seedPerson(t, tx, "Holder")
		create(t, tx, &data.Account{LoyaltyProgramID: smiles.ID, HolderID: &holder.ID})
		other := data.Account{LoyaltyProgramID: azul.ID, HolderID: &holder.ID}
		create(t, tx, &other)

		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"loyalty_program_id":"`+smiles.ID+`"}`))
		rec := httptest.NewRecorder()
		NewAccountHandler(tx).UpdateAccount(rec, withIDs(req, map[string]string{"id": other.ID}))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d (%s), want 422", rec.Code, rec.Body.String())
		}
	})
}

func TestCreateAccount_ArchivedAccountDoesNotBlockTheProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		program := seedProgram(t, tx)
		holder := seedPerson(t, tx, "Holder")
		old := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
		create(t, tx, &old)
		if err := tx.Delete(&old).Error; err != nil {
			t.Fatal(err)
		}

		rec := callCreateAccount(tx, `{"loyalty_program_id":"`+program.ID+`","holder_id":"`+holder.ID+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("got %d (%s), want 201", rec.Code, rec.Body.String())
		}
	})
}

// The database says it too since migration 76, so a write that skips the API's check - a
// race, a hand-written fix - still cannot leave a holder with two live accounts in a program.
func TestDatabase_RefusesASecondLiveAccountInAProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		program := seedProgram(t, tx)
		holder := seedPerson(t, tx, "Holder")
		first := data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}
		create(t, tx, &first)

		// A savepoint, so the refused insert does not abort the test's transaction.
		err := tx.Transaction(func(sp *gorm.DB) error {
			return sp.Create(&data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID}).Error
		})
		if err == nil {
			t.Fatal("the database accepted a second live account in the same program")
		}

		// Archiving the first makes room, which is how an operator replaces an account.
		if err := tx.Delete(&first).Error; err != nil {
			t.Fatal(err)
		}
		create(t, tx, &data.Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID})
	})
}
