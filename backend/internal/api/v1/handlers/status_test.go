package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/health"
)

// Unlike almost every other test in this package, these run everywhere: the
// status handler has no *gorm.DB, so there is nothing to skip when Postgres is
// missing.

func decodeStatusBody(t *testing.T, monitor *health.Monitor) (string, map[string]any) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/status", nil)
	NewStatusHandler(monitor).GetSystemStatus(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	raw := recorder.Body.String()

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("response is not the standard envelope: %v", err)
	}

	return raw, envelope.Data
}

func TestGetSystemStatusReportsBothDependencies(t *testing.T) {
	monitor := health.NewMonitor(func() {})

	_, data := decodeStatusBody(t, monitor)

	for _, dependency := range []string{"bot", "traveos"} {
		check, ok := data[dependency].(map[string]any)
		if !ok {
			t.Fatalf("expected a %q object in the response, got %v", dependency, data[dependency])
		}
		// Before the first probe answers, both are unknown rather than absent:
		// a missing light and a light that says "up" look the same to nobody.
		if check["state"] != string(health.StateUnknown) {
			t.Errorf("%s: expected state %q, got %v", dependency, health.StateUnknown, check["state"])
		}
		if check["checked_at"] == "" || check["checked_at"] == nil {
			t.Errorf("%s: expected a checked_at, so the panel can say how stale this is", dependency)
		}
	}
}

func TestGetSystemStatusNeverLeaksThePairingCode(t *testing.T) {
	// The bot's own /status carries pair_code and jid alongside the state, and
	// this route is open to every authenticated user - not just admins. The
	// pairing code is a credential: whoever reads it pairs the company's
	// WhatsApp to their own phone. It must not survive the projection.
	monitor := health.NewMonitor(func() {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Run(ctx, health.DependencyBot, time.Hour, func(context.Context) (string, error) {
		return "connected", nil
	})

	deadline := time.Now().Add(2 * time.Second)
	for monitor.Get().Bot.State == health.StateUnknown {
		if time.Now().After(deadline) {
			t.Fatal("the probe never ran")
		}
		time.Sleep(time.Millisecond)
	}

	raw, data := decodeStatusBody(t, monitor)

	for _, forbidden := range []string{"pair_code", "pairCode", "jid"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("the response must never carry %q: %s", forbidden, raw)
		}
	}

	bot, _ := data["bot"].(map[string]any)
	if bot["session"] != "connected" {
		t.Errorf("expected the session state to survive, got %v", bot["session"])
	}
}
