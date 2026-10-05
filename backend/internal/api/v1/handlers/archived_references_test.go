package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	acct "github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"gorm.io/gorm"
)

func TestIssuanceCreate_RefusesAnArchivedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		if err := tx.Delete(&world.drop).Error; err != nil {
			t.Fatal(err)
		}

		_, err := issuancesvc.Create(tx, issuancesvc.CreateInput{
			LoyaltyProgramID:   world.program.ID,
			AirlineID:          &world.airline.ID,
			ProgramLocator:     "PL" + uuid.New().String()[:8],
			Points:             1000,
			ValueOfTheThousand: 50,
			AccountID:          world.drop.ID,
			AgencyID:           &world.agency.ID,
		}, world.user.ID)

		var archived *acct.ArchivedError
		if !errors.As(err, &archived) {
			t.Fatalf("got %v, want an ArchivedError", err)
		}
	})
}

func TestCreateAccount_RefusesAnArchivedHolder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		program := seedProgram(t, tx)
		holder := seedPerson(t, tx, "Archived")
		if err := tx.Delete(&holder).Error; err != nil {
			t.Fatal(err)
		}

		rec := callCreateAccount(tx, `{"loyalty_program_id":"`+program.ID+`","holder_id":"`+holder.ID+`"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d (%s), want 422", rec.Code, rec.Body.String())
		}
	})
}

func TestListAccountHolders_HidesArchivedPeople(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedHolderWorld(t, tx)
		if err := tx.Delete(&world.beltrano).Error; err != nil {
			t.Fatal(err)
		}

		page := callListAccountHolders(t, tx, url.Values{"q": {world.marker}})
		for _, row := range page.Rows {
			if row.Holder.ID == world.beltrano.ID {
				t.Fatal("an archived person is still a row")
			}
		}
		// Archiving marks the row; it does not remove it.
		var count int64
		tx.Unscoped().Model(&data.Person{}).Where("id = ?", world.beltrano.ID).Count(&count)
		if count != 1 {
			t.Fatalf("archived person rows: got %d, want 1", count)
		}
	})
}
