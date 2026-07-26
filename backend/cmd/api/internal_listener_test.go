package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/events"
)

func TestInternalRouter_BotStatusChanged_Broadcasts(t *testing.T) {
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	r := newInternalRouter(broker)

	req := httptest.NewRequest(http.MethodPost, "/internal/bot/status-changed", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNoContent)
	}

	select {
	case evt := <-ch:
		if evt.Type != "bot.status" {
			t.Fatalf("event type: got %q, want %q", evt.Type, "bot.status")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}

func TestInternalRouter_ChatRequested_Broadcasts(t *testing.T) {
	broker := events.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	r := newInternalRouter(broker)

	req := httptest.NewRequest(http.MethodPost, "/internal/bot/chat-requested", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNoContent)
	}

	select {
	case evt := <-ch:
		if evt.Type != "listened_chat.requested" {
			t.Fatalf("event type: got %q, want %q", evt.Type, "listened_chat.requested")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}
