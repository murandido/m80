package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func administratorsOf(t *testing.T, tx *gorm.DB, holderID string) []string {
	t.Helper()
	var admins []string
	if err := tx.Model(&data.Account{}).Where("holder_id = ?", holderID).Order("created_at").
		Pluck("COALESCE(administrator_id::text, '')", &admins).Error; err != nil {
		t.Fatal(err)
	}
	return admins
}

func allEqual(values []string, want string) bool {
	for _, value := range values {
		if value != want {
			return false
		}
	}
	return len(values) > 0
}

func TestCreateAccount_InheritsTheHolderAdministrator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		holder, admin := seedPerson(t, tx, "Holder"), seedPerson(t, tx, "Admin")
		create(t, tx, &data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID, AdministratorID: &admin.ID})

		program := seedProgram(t, tx)
		rec := callCreateAccount(tx, `{"loyalty_program_id":"`+program.ID+`","holder_id":"`+holder.ID+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
		if got := administratorsOf(t, tx, holder.ID); !allEqual(got, admin.ID) {
			t.Errorf("administrators: got %v, want all %s", got, admin.ID)
		}
	})
}

func TestUpdateAccount_AdministratorReachesEverySibling(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		holder, admin := seedPerson(t, tx, "Holder"), seedPerson(t, tx, "Admin")
		first := data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID}
		create(t, tx, &first)
		create(t, tx, &data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID})

		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"administrator_id":"`+admin.ID+`"}`))
		rec := httptest.NewRecorder()
		NewAccountHandler(tx).UpdateAccount(rec, withIDs(req, map[string]string{"id": first.ID}))
		if rec.Code != http.StatusOK {
			t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
		}
		if got := administratorsOf(t, tx, holder.ID); !allEqual(got, admin.ID) {
			t.Errorf("administrators: got %v, want all %s", got, admin.ID)
		}
	})
}

func TestSetHolderAdministrator_ResolvesAConflict(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		holder := seedPerson(t, tx, "Holder")
		adminA, adminB := seedPerson(t, tx, "Admin A"), seedPerson(t, tx, "Admin B")
		create(t, tx, &data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID, AdministratorID: &adminA.ID})
		create(t, tx, &data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID, AdministratorID: &adminB.ID})

		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"administrator_id":"`+adminB.ID+`"}`))
		rec := httptest.NewRecorder()
		NewPeopleHandler(tx).SetHolderAdministrator(rec, withIDs(req, map[string]string{"id": holder.ID}))
		if rec.Code != http.StatusOK {
			t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
		}
		if got := administratorsOf(t, tx, holder.ID); !allEqual(got, adminB.ID) {
			t.Errorf("administrators: got %v, want all %s", got, adminB.ID)
		}
	})
}
