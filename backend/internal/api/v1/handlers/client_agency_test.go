package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/gorm"
)

func seedClientIn(t *testing.T, tx *gorm.DB, name string, agencyID *string) data.Client {
	t.Helper()
	client := data.Client{Name: name, AgencyID: agencyID}
	create(t, tx, &client)
	return client
}

func listClientsAs(t *testing.T, tx *gorm.DB, query string, scope access.AgencyScope) map[string]bool {
	t.Helper()
	rec := httptest.NewRecorder()
	NewClientHandler(tx).ListClients(rec, withScope(httptest.NewRequest(http.MethodGet, "/clients?"+query, nil), scope))
	var resp clientListResponse
	decodeOK(t, rec, "ListClients", &resp)
	seen := map[string]bool{}
	for _, client := range resp.Data {
		seen[client.ID] = true
	}
	return seen
}

func TestListClients_OnlyTheCallersAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedNamedAgency(t, tx, "Mine")
		other := seedNamedAgency(t, tx, "Other")
		prefix := "Scoped-" + uuid.New().String()[:8] + "-"
		visible := seedClientIn(t, tx, prefix+"visible", &mine.ID)
		hidden := seedClientIn(t, tx, prefix+"hidden", &other.ID)
		pending := seedClientIn(t, tx, prefix+"pending", nil)

		seen := listClientsAs(t, tx, "q="+prefix, access.AgencyScope{IDs: []string{mine.ID}})
		if !seen[visible.ID] || seen[hidden.ID] || seen[pending.ID] {
			t.Errorf("restricted: want only the client of their agency, saw %v", seen)
		}
		if seen := listClientsAs(t, tx, "q="+prefix+"&pending_agency=true", access.FullScope()); len(seen) != 1 || !seen[pending.ID] {
			t.Errorf("dev, pending: want only the client without an agency, saw %v", seen)
		}
		if seen := listClientsAs(t, tx, "q="+prefix+"&agency_id="+other.ID, access.FullScope()); len(seen) != 1 || !seen[hidden.ID] {
			t.Errorf("agency filter: want only the other agency's client, saw %v", seen)
		}
	})
}

func TestCreateClient_NameIsUniqueWithinAnAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		first := seedNamedAgency(t, tx, "First")
		second := seedNamedAgency(t, tx, "Second")
		name := "Homônimo " + uuid.New().String()[:8]

		create := func(agencyID string) int {
			body := `{"name": "` + name + `", "agency_id": "` + agencyID + `"}`
			req := withScope(httptest.NewRequest(http.MethodPost, "/clients", strings.NewReader(body)), access.FullScope())
			rec := httptest.NewRecorder()
			NewClientHandler(tx).CreateClient(rec, req)
			return rec.Code
		}
		if code := create(first.ID); code != http.StatusCreated {
			t.Fatalf("first: got %d", code)
		}
		if code := create(second.ID); code != http.StatusCreated {
			t.Errorf("same name, other agency: got %d, want 201", code)
		}

		restricted := access.AgencyScope{IDs: []string{first.ID}}
		body := `{"name": "Outro ` + name + `", "agency_id": "` + second.ID + `"}`
		req := withScope(httptest.NewRequest(http.MethodPost, "/clients", strings.NewReader(body)), restricted)
		rec := httptest.NewRecorder()
		NewClientHandler(tx).CreateClient(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("client in a foreign agency: got %d, want 404", rec.Code)
		}

		// No agency named: the caller's default, as an old panel tab would send it.
		req = withScope(httptest.NewRequest(http.MethodPost, "/clients", strings.NewReader(`{"name": "Sem agência `+name+`"}`)), restricted)
		rec = httptest.NewRecorder()
		NewClientHandler(tx).CreateClient(rec, req)
		var created struct {
			Data schemas.ClientResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || rec.Code != http.StatusCreated {
			t.Fatalf("no agency: got %d (%s)", rec.Code, rec.Body.String())
		}
		if created.Data.AgencyID == nil || *created.Data.AgencyID != first.ID {
			t.Errorf("no agency: landed on %v, want the caller's %s", created.Data.AgencyID, first.ID)
		}

		// Last: the unique violation aborts the test's outer transaction.
		if code := create(first.ID); code != http.StatusUnprocessableEntity {
			t.Errorf("same name, same agency: got %d, want 422", code)
		}
	})
}

func TestUpdateSale_RefusesAClientOfAnotherAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		_, sale := seedSaleFixture(t, tx, defaultAgency(t, tx).ID)
		other := seedNamedAgency(t, tx, "Other")
		stranger := seedClientIn(t, tx, "Estranho "+uuid.New().String()[:8], &other.ID)
		pending := seedClientIn(t, tx, "Pendente "+uuid.New().String()[:8], nil)

		for name, client := range map[string]data.Client{"other agency": stranger, "no agency": pending} {
			if rec := callUpdateSale(t, tx, sale.ID, `{"client_id":"`+client.ID+`"}`); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s: got %d, want 422 (%s)", name, rec.Code, rec.Body.String())
			}
		}
	})
}

func TestUpdateIssuance_RefusesToMoveAwayFromItsSalesClient(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, sale := seedSaleFixture(t, tx, defaultAgency(t, tx).ID)
		client := seedClient(t, tx)
		if err := tx.Model(&sale).Update("client_id", client.ID).Error; err != nil {
			t.Fatal(err)
		}
		other := seedNamedAgency(t, tx, "Other")

		req := httptest.NewRequest(http.MethodPatch, "/issuances/"+issuance.ID,
			strings.NewReader(`{"agency_id": "`+other.ID+`"}`))
		req = withIDs(req, map[string]string{"id": issuance.ID})
		req = req.WithContext(asUser(req.Context(), data.User{ID: issuance.IssuerID, Role: data.RoleAdmin}))
		rec := httptest.NewRecorder()
		NewIssuanceHandler(tx, events.NewBroker(), nil).UpdateIssuance(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("got %d, want 422 (%s)", rec.Code, rec.Body.String())
		}
	})
}

func TestClientPayment_OutsideScopeIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		other := seedNamedAgency(t, tx, "Other")
		stranger := seedClientIn(t, tx, "Estranho "+uuid.New().String()[:8], &other.ID)
		actor := createUser(t, tx, "Operador")

		req := httptest.NewRequest(http.MethodPost, "/sales/client-payments", strings.NewReader(clientPaymentBody(stranger.ID, 1000)))
		req = withScope(req.WithContext(asUser(req.Context(), actor)), access.AgencyScope{IDs: []string{defaultAgency(t, tx).ID}})
		rec := httptest.NewRecorder()
		NewSaleHandler(tx, events.NewBroker()).CreateClientPayment(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("got %d, want 404 (%s)", rec.Code, rec.Body.String())
		}
		var count int64
		tx.Model(&data.ClientPayment{}).Where("client_id = ?", stranger.ID).Count(&count)
		if count != 0 {
			t.Error("a payment was recorded for a client outside the caller's agencies")
		}
	})
}
