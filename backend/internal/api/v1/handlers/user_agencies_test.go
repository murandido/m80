package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func callUserRoute(
	tx *gorm.DB,
	handle func(*UserHandler, http.ResponseWriter, *http.Request),
	caller data.User, method, targetID, body string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/users", strings.NewReader(body))
	if targetID != "" {
		req = withIDs(req, map[string]string{"id": targetID})
	}
	req = req.WithContext(asUser(req.Context(), caller))
	rec := httptest.NewRecorder()
	handle(NewUserHandler(tx), rec, req)
	return rec
}

func newUserBody(role, agencies string) string {
	phone := fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	body := `{"name": "Novo", "phone": "` + phone + `", "password": "secret1", "role": "` + role + `"`
	if agencies != "" {
		body += `, "agency_ids": ` + agencies
	}
	return body + "}"
}

func grantsOf(t *testing.T, tx *gorm.DB, userID string) []string {
	t.Helper()
	var ids []string
	if err := tx.Model(&data.UserAgency{}).Where("user_id = ?", userID).Order("agency_id").Pluck("agency_id", &ids).Error; err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestUsers_OnlyDevsHandOutAgenciesAndTheDevRole(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		admin := data.User{ID: createUser(t, tx, "Admin").ID, Role: data.RoleAdmin}
		agency := seedNamedAgency(t, tx, "A")

		create := (*UserHandler).CreateUser
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", newUserBody("dev", "")); rec.Code != http.StatusForbidden {
			t.Errorf("admin creating a dev: got %d, want 403", rec.Code)
		}
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", newUserBody("user", `["`+agency.ID+`"]`)); rec.Code != http.StatusForbidden {
			t.Errorf("admin granting agencies: got %d, want 403", rec.Code)
		}
		if rec := callUserRoute(tx, create, admin, http.MethodPost, "", newUserBody("user", "")); rec.Code != http.StatusCreated {
			t.Errorf("admin creating a plain user: got %d, want 201 (%s)", rec.Code, rec.Body.String())
		}

		dev := createUser(t, tx, "Dev")
		tx.Model(&dev).Update("role", data.RoleDev)
		update := (*UserHandler).UpdateUser
		if rec := callUserRoute(tx, update, admin, http.MethodPut, dev.ID, `{"name": "Hijacked"}`); rec.Code != http.StatusForbidden {
			t.Errorf("admin editing a dev: got %d, want 403", rec.Code)
		}
		plain := createUser(t, tx, "Plain")
		if rec := callUserRoute(tx, update, admin, http.MethodPut, plain.ID, `{"role": "dev"}`); rec.Code != http.StatusForbidden {
			t.Errorf("admin promoting to dev: got %d, want 403", rec.Code)
		}
		if rec := callUserRoute(tx, (*UserHandler).DeleteUser, admin, http.MethodDelete, dev.ID, ""); rec.Code != http.StatusForbidden {
			t.Errorf("admin deleting a dev: got %d, want 403", rec.Code)
		}
	})
}

func TestUsers_DevAssignsAgencies(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		dev := data.User{ID: createUser(t, tx, "Dev").ID, Role: data.RoleDev}
		first := seedNamedAgency(t, tx, "First")
		second := seedNamedAgency(t, tx, "Second")

		rec := callUserRoute(tx, (*UserHandler).CreateUser, dev, http.MethodPost, "",
			newUserBody("admin", `["`+first.ID+`", "`+first.ID+`"]`))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create: got %d (%s)", rec.Code, rec.Body.String())
		}
		var created struct {
			Data schemas.UserResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if got := grantsOf(t, tx, created.Data.ID); !slices.Equal(got, []string{first.ID}) {
			t.Errorf("create: want one grant of %s, got %v", first.ID, got)
		}

		update := (*UserHandler).UpdateUser
		// Absent agency_ids is what an old panel tab sends: grants stay as they were.
		if rec := callUserRoute(tx, update, dev, http.MethodPut, created.Data.ID, `{"name": "Renamed"}`); rec.Code != http.StatusOK {
			t.Fatalf("rename: got %d", rec.Code)
		}
		if got := grantsOf(t, tx, created.Data.ID); !slices.Equal(got, []string{first.ID}) {
			t.Errorf("rename touched the grants: %v", got)
		}

		if rec := callUserRoute(tx, update, dev, http.MethodPut, created.Data.ID, `{"agency_ids": ["`+second.ID+`"]}`); rec.Code != http.StatusOK {
			t.Fatalf("replace: got %d", rec.Code)
		}
		if got := grantsOf(t, tx, created.Data.ID); !slices.Equal(got, []string{second.ID}) {
			t.Errorf("replace: want only %s, got %v", second.ID, got)
		}

		if rec := callUserRoute(tx, update, dev, http.MethodPut, created.Data.ID, `{"agency_ids": ["`+uuid.New().String()+`"]}`); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("unknown agency: got %d, want 422", rec.Code)
		}
		if got := grantsOf(t, tx, created.Data.ID); !slices.Equal(got, []string{second.ID}) {
			t.Errorf("a refused update changed the grants: %v", got)
		}

		if rec := callUserRoute(tx, update, dev, http.MethodPut, created.Data.ID, `{"role": "dev"}`); rec.Code != http.StatusOK {
			t.Fatalf("promote: got %d", rec.Code)
		}
		if got := grantsOf(t, tx, created.Data.ID); len(got) != 0 {
			t.Errorf("promoted to dev but kept grants: %v", got)
		}
	})
}
