package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callDeletePerson(tx *gorm.DB, id string) (*httptest.ResponseRecorder, schemas.PersonRemovalResponse) {
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	NewPeopleHandler(tx).DeletePerson(rec, withIDs(req, map[string]string{"id": id}))
	var body struct {
		Data schemas.PersonRemovalResponse `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body.Data
}

func callDeleteAccount(tx *gorm.DB, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	NewAccountHandler(tx).DeleteAccount(rec, withIDs(req, map[string]string{"id": id}))
	return rec
}

func rowExists(t *testing.T, tx *gorm.DB, model any, id string) (exists, archived bool) {
	t.Helper()
	var count int64
	tx.Unscoped().Model(model).Where("id = ?", id).Count(&count)
	var live int64
	tx.Model(model).Where("id = ?", id).Count(&live)
	return count == 1, count == 1 && live == 0
}

// The production case: a holder the bot made from "119,08", whose only account is already
// archived and never issued anything. Both go for real.
func TestDeletePerson_JunkHolderWithAnArchivedEmptyAccountGoesForReal(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		holder := seedPerson(t, tx, "119,08")
		create(t, tx, &data.PersonNickname{PersonID: holder.ID, Nickname: "119,08"})
		account := data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID}
		create(t, tx, &account)
		if err := tx.Delete(&account).Error; err != nil {
			t.Fatal(err)
		}

		rec, removal := callDeletePerson(tx, holder.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
		}
		if !removal.PersonDeleted || removal.AccountsDeleted != 1 {
			t.Errorf("removal: got %+v, want person and one account deleted", removal)
		}
		if exists, _ := rowExists(t, tx, &data.Person{}, holder.ID); exists {
			t.Error("the person is still in the table")
		}
		if exists, _ := rowExists(t, tx, &data.Account{}, account.ID); exists {
			t.Error("the account is still in the table")
		}
	})
}

// An account with an issuance - even one in the trash - is history: archived, and so is its
// holder, whose aliases are released for the bot.
func TestDeletePerson_AccountWithTrashedIssuanceIsArchived(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		holder := seedPerson(t, tx, "Holder")
		create(t, tx, &data.PersonNickname{PersonID: holder.ID, Nickname: "junk-" + holder.ID[:8]})
		if err := tx.Model(&world.drop).UpdateColumn("holder_id", holder.ID).Error; err != nil {
			t.Fatal(err)
		}
		issuance := world.issue(t, tx, world.drop.ID)
		if err := tx.Delete(&issuance).Error; err != nil {
			t.Fatal(err)
		}

		rec, removal := callDeletePerson(tx, holder.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
		}
		if removal.PersonDeleted || removal.AccountsArchived != 1 {
			t.Errorf("removal: got %+v, want the account archived and the person archived", removal)
		}
		if _, archived := rowExists(t, tx, &data.Person{}, holder.ID); !archived {
			t.Error("the person should be archived, not gone and not live")
		}
		if _, archived := rowExists(t, tx, &data.Account{}, world.drop.ID); !archived {
			t.Error("the account should be archived")
		}
		var aliases int64
		tx.Model(&data.PersonNickname{}).Where("person_id = ?", holder.ID).Count(&aliases)
		if aliases != 0 {
			t.Errorf("aliases: got %d, want the names released", aliases)
		}
	})
}

func TestDeletePerson_RefusesSomebodyWhoAdministersOthers(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		admin, other := seedPerson(t, tx, "Admin"), seedPerson(t, tx, "Other")
		create(t, tx, &data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &other.ID, AdministratorID: &admin.ID})

		rec, _ := callDeletePerson(tx, admin.ID)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d (%s), want 422", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteAccount_WithoutHistoryGoesForReal(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		holder := seedPerson(t, tx, "Holder")
		account := data.Account{LoyaltyProgramID: seedProgram(t, tx).ID, HolderID: &holder.ID}
		create(t, tx, &account)

		if rec := callDeleteAccount(tx, account.ID); rec.Code != http.StatusOK {
			t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
		}
		if exists, _ := rowExists(t, tx, &data.Account{}, account.ID); exists {
			t.Error("an account nothing refers to should be deleted for real")
		}
	})
}

func TestDeleteAccount_WithAnIssuanceIsArchived(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		world.issue(t, tx, world.keep.ID)

		if rec := callDeleteAccount(tx, world.keep.ID); rec.Code != http.StatusOK {
			t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
		}
		if _, archived := rowExists(t, tx, &data.Account{}, world.keep.ID); !archived {
			t.Error("an account with an issuance should be archived")
		}
	})
}
