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

// callMergePerson drives MergePerson directly, injecting the chi {id} param and the
// authenticated admin the same way the real middleware chain would.
func callMergePerson(tx *gorm.DB, keepID, body string) *httptest.ResponseRecorder {
	handler := NewPeopleHandler(tx)
	handler.Validator = validator.New()

	req := httptest.NewRequest(http.MethodPost, "/people/"+keepID+"/merge", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.MergePerson(rec, withMergeContext(req, keepID))
	return rec
}

func callPreviewMergePerson(tx *gorm.DB, keepID, sourceID string) *httptest.ResponseRecorder {
	handler := NewPeopleHandler(tx)

	req := httptest.NewRequest(http.MethodGet, "/people/"+keepID+"/merge-preview?source_id="+sourceID, nil)
	rec := httptest.NewRecorder()
	handler.PreviewMergePerson(rec, withMergeContext(req, keepID))
	return rec
}

func withMergeContext(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{
		ID: uuid.New().String(), Role: data.RoleAdmin,
	})
	return req.WithContext(ctx)
}

func seedPerson(t *testing.T, tx *gorm.DB, name string) data.Person {
	t.Helper()
	person := data.Person{Name: name}
	create(t, tx, &person)
	return person
}

// seedProgram creates the airline → loyalty program chain an account needs.
func seedProgram(t *testing.T, tx *gorm.DB) data.LoyaltyProgram {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air", Code: "T" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Test Program", Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	return program
}

func mergePersonResult(t *testing.T, rec *httptest.ResponseRecorder) schemas.MergePersonResultResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data schemas.MergePersonResultResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp.Data
}

func TestMergePerson_AbsorbedPersonIsReallyGone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")
		drop := seedPerson(t, tx, "Heraldo R.")

		callMergePerson(tx, keep.ID, `{"source_id":"`+drop.ID+`"}`)

		// Unscoped: people are hard-deleted, so a row still there would mean the merge
		// only hid it - which is what the whole feature exists to avoid.
		var found int64
		if err := tx.Unscoped().Model(&data.Person{}).Where("id = ?", drop.ID).Count(&found).Error; err != nil {
			t.Fatalf("counting the absorbed person: %v", err)
		}
		if found != 0 {
			t.Errorf("absorbed person is still in the table (%d rows)", found)
		}
	})
}

// The case the panel cannot resolve today: an archived account keeps holding holder_id,
// so DeletePerson answers 422 forever. The merge only clears it because it repoints the
// archived account too.
func TestMergePerson_RepointsArchivedAccounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")
		drop := seedPerson(t, tx, "Heraldo R.")
		program := seedProgram(t, tx)

		account := data.Account{LoyaltyProgramID: program.ID, HolderID: &drop.ID, AdministratorID: &drop.ID}
		create(t, tx, &account)
		if err := tx.Delete(&data.Account{}, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("archiving the account: %v", err)
		}

		rec := callMergePerson(tx, keep.ID, `{"source_id":"`+drop.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var archived data.Account
		if err := tx.Unscoped().First(&archived, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading the archived account: %v", err)
		}
		if archived.HolderID == nil || *archived.HolderID != keep.ID {
			t.Errorf("holder_id: got %v, want %q", archived.HolderID, keep.ID)
		}
		if archived.AdministratorID == nil || *archived.AdministratorID != keep.ID {
			t.Errorf("administrator_id: got %v, want %q", archived.AdministratorID, keep.ID)
		}
	})
}

func TestMergePerson_CarriesBlankFieldsAndKeepsItsOwn(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keepEmail := "heraldo@example.com"
		keep := data.Person{Name: "Heraldo Ribeiro", Email: &keepEmail}
		create(t, tx, &keep)

		dropCPF, dropEmail, dropCity := "12345678901", "outro@example.com", "Salvador"
		drop := data.Person{Name: "Heraldo R.", CPF: &dropCPF, Email: &dropEmail, City: &dropCity}
		create(t, tx, &drop)

		result := mergePersonResult(t, callMergePerson(tx, keep.ID, `{"source_id":"`+drop.ID+`"}`))

		// The CPF proves the ordering: idx_people_cpf is unique, so it can only land here
		// because the absorbed row was deleted first.
		if result.Person.CPF == nil || *result.Person.CPF != dropCPF {
			t.Errorf("cpf: got %v, want %q", result.Person.CPF, dropCPF)
		}
		if result.Person.City == nil || *result.Person.City != dropCity {
			t.Errorf("city: got %v, want %q", result.Person.City, dropCity)
		}
		if result.Person.Email == nil || *result.Person.Email != keepEmail {
			t.Errorf("email: got %v, want the survivor's %q", result.Person.Email, keepEmail)
		}
		if result.Person.Name != "Heraldo Ribeiro" {
			t.Errorf("name: got %q, want the survivor's", result.Person.Name)
		}
	})
}

func TestMergePerson_MovesNicknamesAndRegistersTheAbsorbedName(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")
		drop := seedPerson(t, tx, "Heraldo R. "+uuid.New().String()[:8])
		create(t, tx, &data.PersonNickname{PersonID: drop.ID, Nickname: "hudinho" + uuid.New().String()[:8]})

		result := mergePersonResult(t, callMergePerson(tx, keep.ID, `{"source_id":"`+drop.ID+`"}`))

		if len(result.Person.Nicknames) != 2 {
			t.Fatalf("nicknames: got %d, want 2 (the moved one plus the absorbed name)", len(result.Person.Nicknames))
		}
		var carriesName bool
		for _, nickname := range result.Person.Nicknames {
			if nickname.Nickname == drop.Name {
				carriesName = true
			}
		}
		if !carriesName {
			t.Errorf("the absorbed name %q did not become an alias: %+v", drop.Name, result.Person.Nicknames)
		}
		if result.Warning != "" {
			t.Errorf("warning: got %q, want empty", result.Warning)
		}
	})
}

// A name already spoken for cannot become an alias, and that must not take the whole
// merge down with it - the operator still wants the duplicate gone.
func TestMergePerson_WarnsWhenTheAbsorbedNameIsTaken(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")
		drop := seedPerson(t, tx, "Heraldo R. "+uuid.New().String()[:8])
		stranger := seedPerson(t, tx, "Outra Pessoa")
		create(t, tx, &data.PersonNickname{PersonID: stranger.ID, Nickname: drop.Name})

		result := mergePersonResult(t, callMergePerson(tx, keep.ID, `{"source_id":"`+drop.ID+`"}`))

		if result.Warning == "" {
			t.Error("expected a warning about the taken nickname")
		}
		if len(result.Person.Nicknames) != 0 {
			t.Errorf("nicknames: got %d, want 0", len(result.Person.Nicknames))
		}
		var stillStranger data.PersonNickname
		if err := tx.Where("nickname = ?", drop.Name).First(&stillStranger).Error; err != nil {
			t.Fatalf("reloading the stranger's nickname: %v", err)
		}
		if stillStranger.PersonID != stranger.ID {
			t.Errorf("the alias changed owner: got %q, want %q", stillStranger.PersonID, stranger.ID)
		}
	})
}

func TestMergePerson_KeepNicknameFalseSkipsTheAlias(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")
		drop := seedPerson(t, tx, "Heraldo R. "+uuid.New().String()[:8])

		result := mergePersonResult(t, callMergePerson(tx, keep.ID,
			`{"source_id":"`+drop.ID+`","keep_nickname":false}`))

		if len(result.Person.Nicknames) != 0 {
			t.Errorf("nicknames: got %d, want 0", len(result.Person.Nicknames))
		}
	})
}

func TestMergePerson_RefusesTheSameRecordTwice(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")

		rec := callMergePerson(tx, keep.ID, `{"source_id":"`+keep.ID+`"}`)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status: got %d, want 422 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestMergePerson_MissingRecordIs404(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")

		rec := callMergePerson(tx, keep.ID, `{"source_id":"`+uuid.New().String()+`"}`)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status: got %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}

func TestPreviewMergePerson_CountsWithoutWriting(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		keep := seedPerson(t, tx, "Heraldo Ribeiro")
		dropCity := "Salvador"
		drop := data.Person{Name: "Heraldo R. " + uuid.New().String()[:8], City: &dropCity}
		create(t, tx, &drop)

		program := seedProgram(t, tx)
		account := data.Account{LoyaltyProgramID: program.ID, HolderID: &drop.ID}
		create(t, tx, &account)
		create(t, tx, &data.PersonNickname{PersonID: drop.ID, Nickname: "hudinho" + uuid.New().String()[:8]})

		rec := callPreviewMergePerson(tx, keep.ID, drop.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp struct {
			Data schemas.MergePersonPreviewResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Data.Accounts != 1 {
			t.Errorf("accounts: got %d, want 1", resp.Data.Accounts)
		}
		if resp.Data.Nicknames != 1 {
			t.Errorf("nicknames: got %d, want 1", resp.Data.Nicknames)
		}
		if len(resp.Data.FieldsToFill) != 1 || resp.Data.FieldsToFill[0] != "cidade" {
			t.Errorf("fields_to_fill: got %v, want [cidade]", resp.Data.FieldsToFill)
		}
		if resp.Data.NicknameFromName != drop.Name {
			t.Errorf("nickname_from_name: got %q, want %q", resp.Data.NicknameFromName, drop.Name)
		}

		var stillThere int64
		if err := tx.Model(&data.Person{}).Where("id = ?", drop.ID).Count(&stillThere).Error; err != nil {
			t.Fatalf("counting the person the preview must not have touched: %v", err)
		}
		if stillThere != 1 {
			t.Error("the preview deleted the person it was only supposed to describe")
		}
	})
}
