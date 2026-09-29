package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callMergeAccount(tx *gorm.DB, keepID, actorID, body string) *httptest.ResponseRecorder {
	handler := NewAccountHandler(tx)
	handler.Validator = validator.New()

	req := httptest.NewRequest(http.MethodPost, "/accounts/"+keepID+"/merge", strings.NewReader(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", keepID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: actorID, Role: data.RoleAdmin})

	rec := httptest.NewRecorder()
	handler.MergeAccount(rec, req.WithContext(ctx))
	return rec
}

func callPreviewMergeAccount(tx *gorm.DB, keepID, sourceID string) *httptest.ResponseRecorder {
	handler := NewAccountHandler(tx)

	req := httptest.NewRequest(http.MethodGet, "/accounts/"+keepID+"/merge-preview?source_id="+sourceID, nil)
	rec := httptest.NewRecorder()
	handler.PreviewMergeAccount(rec, withMergeContext(req, keepID))
	return rec
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

	keep := data.Account{LoyaltyProgramID: program.ID}
	create(t, tx, &keep)
	drop := data.Account{LoyaltyProgramID: program.ID}
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

func TestMergeAccount_MovesAliasesAndInheritsTheHolder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		holder := seedPerson(t, tx, "Heraldo Ribeiro")

		if err := tx.Model(&data.Account{}).Where("id = ?", world.drop.ID).
			UpdateColumn("holder_id", holder.ID).Error; err != nil {
			t.Fatalf("setting the holder: %v", err)
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
		if merged.HolderID == nil || *merged.HolderID != holder.ID {
			t.Errorf("holder_id: got %v, want %q", merged.HolderID, holder.ID)
		}
		if len(merged.Nicknames) != 1 || merged.Nicknames[0].ID != alias.ID {
			t.Errorf("nicknames: got %+v, want the absorbed alias", merged.Nicknames)
		}
	})
}

// An account belongs to one program, and its issuances were flown on that program's miles.
// Moving them under another program would be inventing history.
func TestMergeAccount_RefusesDifferentPrograms(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		other := seedProgram(t, tx)
		stranger := data.Account{LoyaltyProgramID: other.ID}
		create(t, tx, &stranger)

		rec := callMergeAccount(tx, world.keep.ID, world.user.ID, `{"source_id":"`+stranger.ID+`"}`)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
		var left int64
		if err := tx.Unscoped().Model(&data.Account{}).Where("id = ?", stranger.ID).Count(&left).Error; err != nil {
			t.Fatalf("counting the refused account: %v", err)
		}
		if left != 1 {
			t.Error("the refused merge still deleted the account")
		}
	})
}

// Archiving is all the panel offers today, so the duplicate the operator wants gone is
// usually already archived. Absorbing it has to work anyway - that is the point.
func TestMergeAccount_AbsorbsAnArchivedDuplicate(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		issuance := world.issue(t, tx, world.drop.ID)
		if err := tx.Delete(&data.Account{}, "id = ?", world.drop.ID).Error; err != nil {
			t.Fatalf("archiving the duplicate: %v", err)
		}

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
	})
}

func TestPreviewMergeAccount_CountsWithoutWriting(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		holder := seedPerson(t, tx, "Heraldo Ribeiro")
		if err := tx.Model(&data.Account{}).Where("id = ?", world.drop.ID).
			UpdateColumn("holder_id", holder.ID).Error; err != nil {
			t.Fatalf("setting the holder: %v", err)
		}

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

		rec := callPreviewMergeAccount(tx, world.keep.ID, world.drop.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp struct {
			Data schemas.MergeAccountPreviewResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Data.Issuances != 2 {
			t.Errorf("issuances: got %d, want 2 (the live one and the soft-deleted one)", resp.Data.Issuances)
		}
		if resp.Data.Nicknames != 1 {
			t.Errorf("nicknames: got %d, want 1", resp.Data.Nicknames)
		}
		if len(resp.Data.CollidingNicknames) != 1 {
			t.Errorf("colliding_nicknames: got %v, want one entry", resp.Data.CollidingNicknames)
		}
		if len(resp.Data.FieldsToFill) != 1 || resp.Data.FieldsToFill[0] != "titular" {
			t.Errorf("fields_to_fill: got %v, want [titular]", resp.Data.FieldsToFill)
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

// Archiving was the only way to get rid of a duplicate before merging existed, so the
// accounts that most need absorbing are the archived ones - and the picker cannot offer
// what the listing hides.
func TestGetAccounts_IncludeArchivedReachesTheDuplicateThatWasArchived(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedAccountMergeWorld(t, tx)
		if err := tx.Delete(&data.Account{}, "id = ?", world.drop.ID).Error; err != nil {
			t.Fatalf("archiving the duplicate: %v", err)
		}

		found := func(accounts []schemas.AccountResponse) bool {
			for _, account := range accounts {
				if account.ID == world.drop.ID {
					return true
				}
			}
			return false
		}

		if found(callListAccounts(tx, "")) {
			t.Error("the plain listing returned an archived account; every picker would show it")
		}
		if !found(callListAccounts(tx, "include_archived=true")) {
			t.Error("include_archived=true did not return the archived account")
		}
		// false must behave like absent, or the flag would be on whenever it is mentioned.
		if found(callListAccounts(tx, "include_archived=false")) {
			t.Error("include_archived=false returned an archived account")
		}
	})
}
