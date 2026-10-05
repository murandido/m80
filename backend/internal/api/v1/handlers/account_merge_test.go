package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The per-account merge endpoint went once the database stopped allowing two live accounts in
// one program for a holder; mergeAccountsTx lives on inside the row merge, which folds the
// absorbed holder's account into the survivor's in the same program. These tests drive it
// through there: keepID and the source_id in body are accounts, and the merge runs between
// their holders. keep_nickname is off so only the accounts' own aliases are in play.
func callMergeAccount(tx *gorm.DB, keepID, actorID, body string) *httptest.ResponseRecorder {
	var request struct {
		SourceID string `json:"source_id"`
	}
	_ = json.Unmarshal([]byte(body), &request)

	holderOf := func(accountID string) string {
		var account data.Account
		if err := tx.Unscoped().First(&account, "id = ?", accountID).Error; err != nil {
			return uuid.New().String()
		}
		return *account.HolderID
	}

	return callMergeRow(tx, holderOf(keepID), actorID,
		`{"source_person_id":"`+holderOf(request.SourceID)+`","keep_nickname":false}`)
}

// accountMergeWorld is the pair of duplicated accounts a merge acts on, plus the cast an
// issuance on the absorbed one needs.
type accountMergeWorld struct {
	program data.LoyaltyProgram
	airline data.Airline
	agency  data.Agency
	user    data.User
	keep    data.Account
	drop    data.Account
}

func seedAccountMergeWorld(t *testing.T, tx *gorm.DB) accountMergeWorld {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air", Code: "T" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Test Program", Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	agency := data.Agency{Name: "Test Agency " + suffix}
	create(t, tx, &agency)
	user := createUser(t, tx, "Tester")

	keep := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID}
	create(t, tx, &keep)
	drop := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID}
	create(t, tx, &drop)

	return accountMergeWorld{program: program, airline: airline, agency: agency, user: user, keep: keep, drop: drop}
}

func (w accountMergeWorld) issue(t *testing.T, tx *gorm.DB, accountID string) data.Issuance {
	t.Helper()
	issuance := data.Issuance{
		LoyaltyProgramID:   w.program.ID,
		AirlineID:          w.airline.ID,
		ProgramLocator:     "PL" + uuid.New().String()[:8],
		Points:             1000,
		ValueOfTheThousand: 50,
		AccountID:          accountID,
		IssuerID:           w.user.ID,
		AgencyID:           w.agency.ID,
	}
	create(t, tx, &issuance)
	return issuance
}

func TestMergeAccount_MovesIssuancesAndRemovesTheDuplicate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		issuance := world.issue(t, tx, world.drop.ID)

		rec := callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+world.drop.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var moved data.Issuance
		if err := tx.First(&moved, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("reloading the issuance: %v", err)
		}
		if moved.AccountID != world.keep.ID {
			t.Errorf("account_id: got %q, want %q", moved.AccountID, world.keep.ID)
		}

		// Unscoped: archiving is what the panel already does, so a row still here would
		// mean the merge achieved nothing the delete button could not.
		var left int64
		if err := tx.Unscoped().Model(&data.Account{}).Where("id = ?", world.drop.ID).Count(&left).Error; err != nil {
			t.Fatalf("counting the absorbed account: %v", err)
		}
		if left != 0 {
			t.Errorf("absorbed account is still in the table (%d rows)", left)
		}
	})
}

// A soft-deleted issuance holds account_id exactly like a live one - it is the reason an
// account that ever issued cannot be deleted, so missing it would make the merge fail on
// the foreign key at the very end.
func TestMergeAccount_MovesSoftDeletedIssuancesToo(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		issuance := world.issue(t, tx, world.drop.ID)
		if err := tx.Delete(&data.Issuance{}, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("soft-deleting the issuance: %v", err)
		}

		rec := callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+world.drop.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var moved data.Issuance
		if err := tx.Unscoped().First(&moved, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("reloading the deleted issuance: %v", err)
		}
		if moved.AccountID != world.keep.ID {
			t.Errorf("account_id: got %q, want %q", moved.AccountID, world.keep.ID)
		}
	})
}

func TestMergeAccount_RecordsWhyTheIssuanceChangedAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		issuance := world.issue(t, tx, world.drop.ID)

		callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+world.drop.ID+`"}`)

		var entry data.AuditLog
		if err := tx.Where("issuance_id = ? AND action = ?", issuance.ID, "updated").
			First(&entry).Error; err != nil {
			t.Fatalf("no history entry for the moved issuance: %v", err)
		}

		var details struct {
			Before map[string]string `json:"before"`
			After  map[string]string `json:"after"`
			Reason string            `json:"reason"`
		}
		if err := json.Unmarshal(entry.Details, &details); err != nil {
			t.Fatalf("decoding the details: %v", err)
		}
		if details.Reason != "account_merge" {
			t.Errorf("reason: got %q, want %q", details.Reason, "account_merge")
		}
		if details.Before["account_id"] != world.drop.ID || details.After["account_id"] != world.keep.ID {
			t.Errorf("details: got %+v, want the move from the absorbed to the surviving account", details)
		}
	})
}

func TestMergeAccount_ArchivesACollidingNicknameAndKeepsItsIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		shared := "heraldo" + uuid.New().String()[:8]

		keepAlias := data.AccountNickname{
			AccountID: world.keep.ID, LoyaltyProgramID: world.program.ID, Nickname: shared,
		}
		create(t, tx, &keepAlias)
		// Same alias, different case: the unique index would allow it, but the bot matches
		// case-insensitively, so leaving both alive would make routing ambiguous.
		dropAlias := data.AccountNickname{
			AccountID: world.drop.ID, LoyaltyProgramID: world.program.ID, Nickname: strings.ToUpper(shared),
		}
		create(t, tx, &dropAlias)

		issuance := world.issue(t, tx, world.drop.ID)
		if err := tx.Model(&data.Issuance{}).Where("id = ?", issuance.ID).
			UpdateColumn("account_nickname_id", dropAlias.ID).Error; err != nil {
			t.Fatalf("pointing the issuance at the alias: %v", err)
		}

		rec := callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+world.drop.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var archived data.AccountNickname
		if err := tx.Unscoped().First(&archived, "id = ?", dropAlias.ID).Error; err != nil {
			t.Fatalf("the colliding alias was deleted instead of archived: %v", err)
		}
		if !archived.DeletedAt.Valid {
			t.Error("the colliding alias is still live; the bot would have two ways to the same account")
		}
		if archived.AccountID != world.keep.ID {
			t.Errorf("account_id: got %q, want the surviving account", archived.AccountID)
		}

		var moved data.Issuance
		if err := tx.First(&moved, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("reloading the issuance: %v", err)
		}
		if moved.AccountNicknameID == nil || *moved.AccountNicknameID != dropAlias.ID {
			t.Errorf("account_nickname_id: got %v, want %q - the issuance lost which alias it came in by",
				moved.AccountNicknameID, dropAlias.ID)
		}
	})
}

// Every account has a holder now (migration 75), so what the survivor can still inherit is
// the administrator.
func TestMergeAccount_MovesAliasesAndInheritsTheAdministrator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		admin := seedPerson(t, tx, "Heraldo Ribeiro")

		if err := tx.Model(&data.Account{}).Where("id = ?", world.drop.ID).
			UpdateColumn("administrator_id", admin.ID).Error; err != nil {
			t.Fatalf("setting the administrator: %v", err)
		}
		alias := data.AccountNickname{
			AccountID:        world.drop.ID,
			LoyaltyProgramID: world.program.ID,
			Nickname:         "heraldo" + uuid.New().String()[:8],
		}
		create(t, tx, &alias)

		rec := callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+world.drop.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var merged data.Account
		if err := tx.Preload("Nicknames").First(&merged, "id = ?", world.keep.ID).Error; err != nil {
			t.Fatalf("reloading the surviving account: %v", err)
		}
		if merged.AdministratorID == nil || *merged.AdministratorID != admin.ID {
			t.Errorf("administrator_id: got %v, want %q", merged.AdministratorID, admin.ID)
		}
		if len(merged.Nicknames) != 1 || merged.Nicknames[0].ID != alias.ID {
			t.Errorf("nicknames: got %+v, want the absorbed alias", merged.Nicknames)
		}
	})
}

// An account belongs to one program, and its issuances were flown on that program's miles.
// Absorbing a holder whose account is in another program only hands that account over - it
// is never folded into one of a different program.
func TestMergeAccount_NeverFoldsDifferentPrograms(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		other := seedProgram(t, tx)
		stranger := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: other.ID}
		create(t, tx, &stranger)

		rec := callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+stranger.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var survived data.Account
		if err := tx.First(&survived, "id = ?", stranger.ID).Error; err != nil {
			t.Fatalf("the account of another program was deleted: %v", err)
		}
		if *survived.HolderID != *world.keep.HolderID {
			t.Errorf("holder: got %s, want the survivor's", *survived.HolderID)
		}
	})
}

func TestPreviewMergeRow_AccountPairCountsWithoutWriting(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)

		world.issue(t, tx, world.drop.ID)
		deleted := world.issue(t, tx, world.drop.ID)
		if err := tx.Delete(&data.Issuance{}, "id = ?", deleted.ID).Error; err != nil {
			t.Fatalf("soft-deleting an issuance: %v", err)
		}

		shared := "heraldo" + uuid.New().String()[:8]
		create(t, tx, &data.AccountNickname{
			AccountID: world.keep.ID, LoyaltyProgramID: world.program.ID, Nickname: shared,
		})
		create(t, tx, &data.AccountNickname{
			AccountID: world.drop.ID, LoyaltyProgramID: world.program.ID, Nickname: strings.ToUpper(shared),
		})

		preview := callPreviewMergeRow(t, tx, *world.keep.HolderID, "source_person_id="+*world.drop.HolderID)

		if len(preview.AccountsMerged) != 1 {
			t.Fatalf("accounts merged: got %d, want the one pair", len(preview.AccountsMerged))
		}
		pair := preview.AccountsMerged[0]
		if pair.Issuances != 2 {
			t.Errorf("issuances: got %d, want 2 (the live one and the soft-deleted one)", pair.Issuances)
		}
		if len(pair.CollidingNicknames) != 1 {
			t.Errorf("colliding_nicknames: got %v, want one entry", pair.CollidingNicknames)
		}

		var stillThere int64
		if err := tx.Model(&data.Account{}).Where("id = ?", world.drop.ID).Count(&stillThere).Error; err != nil {
			t.Fatalf("counting the account the preview must not have touched: %v", err)
		}
		if stillThere != 1 {
			t.Error("the preview deleted the account it was only supposed to describe")
		}
	})
}

// callListAccounts drives GetAccounts with a raw query string, as a plain user.
func callListAccounts(tx *gorm.DB, query string) []schemas.AccountResponse {
	handler := NewAccountHandler(tx)

	req := httptest.NewRequest(http.MethodGet, "/accounts?"+query, nil)
	rec := httptest.NewRecorder()
	handler.GetAccounts(rec, req)

	var envelope struct {
		Data []schemas.AccountResponse `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &envelope)
	return envelope.Data
}

// Every picker reads this listing, so an archived account must never come back from it:
// nothing new may point at one (internal/account/live.go).
func TestGetAccounts_HidesArchivedAccounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		if err := tx.Delete(&data.Account{}, "id = ?", world.drop.ID).Error; err != nil {
			t.Fatalf("archiving the duplicate: %v", err)
		}

		for _, account := range callListAccounts(tx, "") {
			if account.ID == world.drop.ID {
				t.Fatal("the listing returned an archived account; every picker would show it")
			}
		}
	})
}
