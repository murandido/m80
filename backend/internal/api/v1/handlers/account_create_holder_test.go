package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func callCreateAccount(tx *gorm.DB, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewAccountHandler(tx).CreateAccount(rec, req)
	return rec
}

func TestCreateAccount_RequiresAHolder(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		program := seedProgram(t, tx)

		rec := callCreateAccount(tx, `{"loyalty_program_id":"`+program.ID+`"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "titular") {
			t.Fatalf("without holder: got %d %s, want 400 naming the holder", rec.Code, rec.Body.String())
		}

		holder := seedPerson(t, tx, "Titular")
		rec = callCreateAccount(tx, `{"loyalty_program_id":"`+program.ID+`","holder_id":"`+holder.ID+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("with holder: got %d %s, want 201", rec.Code, rec.Body.String())
		}
	})
}
