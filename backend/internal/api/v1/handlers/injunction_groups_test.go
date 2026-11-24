package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callInjunctionGroup(tx *gorm.DB, caller data.User, method, path, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if id != "" {
		req = withIDs(req, map[string]string{"id": id})
	}
	req = req.WithContext(asUser(req.Context(), caller))
	rec := httptest.NewRecorder()
	h := NewInjunctionGroupHandler(tx, nil, nil)
	switch method {
	case http.MethodPost:
		h.CreateInjunctionGroup(rec, req)
	case http.MethodPut:
		h.UpdateInjunctionGroup(rec, req)
	default:
		h.GetInjunctionGroup(rec, req)
	}
	return rec
}

func decodeGroup(t *testing.T, rec *httptest.ResponseRecorder) schemas.InjunctionGroupResponse {
	t.Helper()
	var envelope struct {
		Data schemas.InjunctionGroupResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	return envelope.Data
}

func TestInjunctionGroupKeepsOnlyInjunctionsAndHidesTheLink(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, _, agency := seedStatementTarget(t, tx)
		plain, _, _ := seedStatementTarget(t, tx)
		dev := createUser(t, tx, "dev")
		issuer := createUser(t, tx, "pedro")
		if err := tx.Model(&account).Update("category", data.AccountCategoryInjunction).Error; err != nil {
			t.Fatal(err)
		}
		const link = "https://onedrive.live.com/:x:/g/personal/ABC/SECRETPART?resid=1&e=2"

		body := func(accountID string) string {
			return fmt.Sprintf(`{"name":"Tropa Teste %s","agency_id":"%s","whatsapp_chat_name":"Tropa Azul","spreadsheet_url":%q,`+
				`"accounts":[{"account_label":" hudson ","account_id":"%s"}],"issuers":[{"issuer_name":"Pedro","user_id":"%s"}]}`,
				accountID[:6], agency.ID, link, accountID, issuer.ID)
		}

		if rec := callInjunctionGroup(tx, dev, http.MethodPost, "/injunction-groups", "", body(plain.ID)); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("a non-injunction account: got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}

		rec := callInjunctionGroup(tx, dev, http.MethodPost, "/injunction-groups", "", body(account.ID))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d (%s)", rec.Code, rec.Body.String())
		}
		group := decodeGroup(t, rec)
		if len(group.Accounts) != 1 || group.Accounts[0].AccountLabel != "HUDSON" || group.Accounts[0].ProgramCode == "" {
			t.Errorf("accounts: %+v", group.Accounts)
		}
		if len(group.Issuers) != 1 || group.Issuers[0].IssuerName != "PEDRO" {
			t.Errorf("issuers: %+v", group.Issuers)
		}
		if strings.Contains(rec.Body.String(), "SECRETPART") || !group.HasSpreadsheet {
			t.Errorf("the link leaked or went missing: %s", rec.Body.String())
		}

		var stored string
		if err := tx.Raw("SELECT spreadsheet_url FROM injunction_groups WHERE id = ?", group.ID).Scan(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(stored, "enc:") {
			t.Errorf("the link is stored in clear")
		}

		// An update without spreadsheet_url keeps the stored link.
		update := strings.Replace(body(account.ID), fmt.Sprintf(`"spreadsheet_url":%q,`, link), "", 1)
		if rec := callInjunctionGroup(tx, dev, http.MethodPut, "/injunction-groups/"+group.ID, group.ID, update); rec.Code != http.StatusOK {
			t.Fatalf("update: got %d (%s)", rec.Code, rec.Body.String())
		}
		var reloaded data.InjunctionGroup
		if err := tx.First(&reloaded, "id = ?", group.ID).Error; err != nil {
			t.Fatal(err)
		}
		if reloaded.SpreadsheetURL == nil || *reloaded.SpreadsheetURL != link {
			t.Errorf("the link was not kept on update")
		}
	})
}
