package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func rowMergeRequest(req *http.Request, keepID, actorID string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", keepID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: actorID, Role: data.RoleAdmin})
	return req.WithContext(ctx)
}

func callMergeRow(tx *gorm.DB, keepID, actorID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/people/"+keepID+"/merge-row", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewPeopleHandler(tx).MergeRow(rec, rowMergeRequest(req, keepID, actorID))
	return rec
}

func callPreviewMergeRow(t *testing.T, tx *gorm.DB, keepID, query string) schemas.MergeRowPreviewResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/people/"+keepID+"/merge-row-preview?"+query, nil)
	rec := httptest.NewRecorder()
	NewPeopleHandler(tx).PreviewMergeRow(rec, rowMergeRequest(req, keepID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Data schemas.MergeRowPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Data
}

// rowMergeWorld is the bot's typical duplicate: Fulana with a Smiles, and a second "Fulana"
// the bot created with her own Smiles (holding an issuance) plus an Azul the first lacks.
type rowMergeWorld struct {
	accountMergeWorld
	azul       data.LoyaltyProgram
	keepPerson data.Person
	dropPerson data.Person
	keepSmiles data.Account
	dropSmiles data.Account
	dropAzul   data.Account
	dropIssued data.Issuance
}

func seedRowMergeWorld(t *testing.T, tx *gorm.DB) rowMergeWorld {
	t.Helper()
	base := seedAccountMergeWorld(t, tx)
	world := rowMergeWorld{
		accountMergeWorld: base,
		azul:              seedProgram(t, tx),
		keepPerson:        seedPerson(t, tx, "Fulana de Tal"),
		dropPerson:        seedPerson(t, tx, "Fulana"),
	}

	// Reuse the two same-program accounts the base world made, handing one to each person.
	world.keepSmiles, world.dropSmiles = base.keep, base.drop
	if err := tx.Model(&world.keepSmiles).UpdateColumn("holder_id", world.keepPerson.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Model(&world.dropSmiles).UpdateColumn("holder_id", world.dropPerson.ID).Error; err != nil {
		t.Fatal(err)
	}
	world.dropAzul = data.Account{LoyaltyProgramID: world.azul.ID, HolderID: &world.dropPerson.ID}
	create(t, tx, &world.dropAzul)
	world.dropIssued = base.issue(t, tx, world.dropSmiles.ID)

	return world
}

func TestPreviewMergeRow_ListsSameProgramPairsAndMovedAccounts(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedRowMergeWorld(t, tx)

		preview := callPreviewMergeRow(t, tx, world.keepPerson.ID, "source_person_id="+world.dropPerson.ID)

		if len(preview.AccountsMerged) != 1 {
			t.Fatalf("accounts merged: got %d, want 1 (the Smiles pair)", len(preview.AccountsMerged))
		}
		pair := preview.AccountsMerged[0]
		if pair.KeepAccountID != world.keepSmiles.ID || pair.DropAccountID != world.dropSmiles.ID || pair.Issuances != 1 {
			t.Errorf("pair: got %+v", pair)
		}
		if preview.AccountsMoved != 1 {
			t.Errorf("accounts moved: got %d, want 1 (the Azul)", preview.AccountsMoved)
		}
		if preview.NicknameFromName != "Fulana" {
			t.Errorf("nickname from name: got %q, want Fulana", preview.NicknameFromName)
		}
	})
}

func TestMergeRow_MergesThePersonAndTheRepeatedProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedRowMergeWorld(t, tx)

		rec := callMergeRow(tx, world.keepPerson.ID, world.user.ID, `{"source_person_id":"`+world.dropPerson.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		if err := tx.First(&data.Person{}, "id = ?", world.dropPerson.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("absorbed person: want gone, got err=%v", err)
		}
		if err := tx.Unscoped().First(&data.Account{}, "id = ?", world.dropSmiles.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("duplicate Smiles: want deleted for real, got err=%v", err)
		}

		var issuance data.Issuance
		if err := tx.First(&issuance, "id = ?", world.dropIssued.ID).Error; err != nil {
			t.Fatal(err)
		}
		if issuance.AccountID != world.keepSmiles.ID {
			t.Errorf("issuance account: got %s, want the surviving Smiles", issuance.AccountID)
		}

		var azul data.Account
		if err := tx.First(&azul, "id = ?", world.dropAzul.ID).Error; err != nil {
			t.Fatal(err)
		}
		if azul.HolderID == nil || *azul.HolderID != world.keepPerson.ID {
			t.Errorf("Azul holder: got %v, want the survivor", azul.HolderID)
		}
	})
}

func TestMergeRow_RefusesAnAccountThatHasAHolder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedRowMergeWorld(t, tx)

		rec := callMergeRow(tx, world.keepPerson.ID, world.user.ID, `{"source_account_id":"`+world.dropAzul.ID+`"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", rec.Code)
		}
	})
}

func TestMergeRow_RefusesBothSourcesAtOnce(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		world := seedRowMergeWorld(t, tx)

		body := `{"source_person_id":"` + world.dropPerson.ID + `","source_account_id":"` + world.dropAzul.ID + `"}`
		if rec := callMergeRow(tx, world.keepPerson.ID, world.user.ID, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want 400", rec.Code)
		}
	})
}
