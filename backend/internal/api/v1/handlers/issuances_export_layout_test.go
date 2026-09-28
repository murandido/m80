package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// An unknown layout is refused before the database is touched, so the handler needs no DB
// here: reaching the query would panic on the nil connection.
func TestGetIssuancesExportRejectsUnknownLayout(t *testing.T) {
	handler := &IssuanceHandler{}
	request := httptest.NewRequest(http.MethodGet, "/issuances/export?layout=spreadsheet", nil)
	recorder := httptest.NewRecorder()

	handler.GetIssuancesExport(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
