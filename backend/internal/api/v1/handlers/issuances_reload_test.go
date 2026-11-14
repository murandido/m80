package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// dbFailingFlightReads returns a connection over the same test transaction whose
// reads of issuance_flights fail. Only the post-commit reload preloads the flights
// on the cancel path, so this breaks the reload and nothing before it.
//
// It is a separate gorm instance on purpose: callbacks live in the gorm config, and
// registering one on the shared test connection would fail every parallel test that
// happens to read flights at the same time.
func dbFailingFlightReads(t *testing.T, tx *gorm.DB) *gorm.DB {
	t.Helper()

	isolated, err := gorm.Open(postgres.New(postgres.Config{Conn: tx.Statement.ConnPool}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open an isolated gorm instance: %v", err)
	}

	err = isolated.Callback().Query().Before("gorm:query").Register("test:fail_issuance_flights", func(db *gorm.DB) {
		if db.Statement.Table == "issuance_flights" {
			_ = db.AddError(errors.New("forced reload failure"))
		}
	})
	if err != nil {
		t.Fatalf("failed to register the failing callback: %v", err)
	}
	return isolated
}

// A cancel whose reload fails after the commit must still reach the other tabs
// and must not read as a plain failure, or the operator acts on it a second time.
func TestCancelIssuance_ReloadFailureStillBroadcastsAndSaysItWasSaved(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		broker := events.NewBroker()
		subscriber := broker.Subscribe()
		defer broker.Unsubscribe(subscriber)

		req := httptest.NewRequest(http.MethodPost, "/issuances/"+issuance.ID+"/cancel", strings.NewReader(""))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", issuance.ID)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		ctx = asUser(ctx, data.User{ID: user.ID, Role: "admin"})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		NewIssuanceHandler(dbFailingFlightReads(t, tx), broker, nil).CancelIssuance(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusInternalServerError, rec.Body.String())
		}

		var body struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !strings.Contains(body.Message, "foi cancelada") {
			t.Errorf("message should say the cancellation was saved, got %q", body.Message)
		}

		select {
		case event := <-subscriber:
			if event.Type != eventIssuanceCancelled {
				t.Errorf("event type: got %q, want %q", event.Type, eventIssuanceCancelled)
			}
			payload, _ := event.Payload.(map[string]string)
			if payload["id"] != issuance.ID {
				t.Errorf("event id: got %q, want %q", payload["id"], issuance.ID)
			}
		case <-time.After(time.Second):
			t.Fatal("no event was broadcast for the committed cancellation")
		}

		var stored data.Issuance
		if err := tx.First(&stored, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to read the issuance back: %v", err)
		}
		if stored.CancelledAt == nil {
			t.Error("the cancellation should be persisted despite the failed reload")
		}
	})
}
