package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls until cond holds or the deadline passes. The monitor's probes
// run on their own goroutine, so every assertion here is about something that
// becomes true shortly rather than something that is true immediately.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}

func TestMonitorStartsUnknown(t *testing.T) {
	monitor := NewMonitor(func() {})

	snapshot := monitor.Get()
	if snapshot.Bot.State != StateUnknown || snapshot.Traveos.State != StateUnknown {
		t.Fatalf("expected both dependencies unknown, got bot=%q traveos=%q",
			snapshot.Bot.State, snapshot.Traveos.State)
	}
	if snapshot.Bot.CheckedAt.IsZero() {
		t.Error("expected an initial timestamp, so the panel can say how stale the answer is")
	}
}

func TestMonitorReportsAFailingProbeAsDown(t *testing.T) {
	var notifications atomic.Int64
	monitor := NewMonitor(func() { notifications.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitor.Run(ctx, DependencyTraveos, time.Hour, func(context.Context) (string, error) {
		return "", errors.New("vendor is on fire")
	})

	waitFor(t, func() bool { return monitor.Get().Traveos.State == StateDown })

	if got := notifications.Load(); got != 1 {
		t.Errorf("expected exactly one notification for the first transition, got %d", got)
	}
}

func TestMonitorDoesNotNotifyWhenNothingChanged(t *testing.T) {
	var notifications atomic.Int64
	monitor := NewMonitor(func() { notifications.Add(1) })

	var probes atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A tiny interval so several ticks land inside the test, all of them
	// answering the same thing.
	go monitor.Run(ctx, DependencyBot, time.Millisecond, func(context.Context) (string, error) {
		probes.Add(1)
		return "connected", nil
	})

	waitFor(t, func() bool { return probes.Load() >= 10 })

	// One notification total: the unknown -> up transition. The other nine
	// ticks are news to nobody, and broadcasting them would push an SSE event
	// to every open browser for nothing.
	if got := notifications.Load(); got != 1 {
		t.Errorf("expected 1 notification across %d probes, got %d", probes.Load(), got)
	}
}

func TestMonitorNotifiesWhenOnlyTheDetailChanged(t *testing.T) {
	var notifications atomic.Int64
	monitor := NewMonitor(func() { notifications.Add(1) })

	// The bot process stays up throughout; only its WhatsApp session changes.
	// That is exactly the case the panel must not miss - a logged-out bot
	// answers its status endpoint perfectly well.
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitor.Run(ctx, DependencyBot, time.Millisecond, func(context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "connected", nil
		}
		return "disconnected", nil
	})

	waitFor(t, func() bool { return monitor.Get().Bot.Detail == "disconnected" })

	if monitor.Get().Bot.State != StateUp {
		t.Error("the bot process answered, so it is up regardless of the session state")
	}
	if got := notifications.Load(); got != 2 {
		t.Errorf("expected 2 notifications (unknown->connected, connected->disconnected), got %d", got)
	}
}

func TestMonitorSeparatesNotConfiguredFromDown(t *testing.T) {
	monitor := NewMonitor(func() {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitor.Run(ctx, DependencyTraveos, time.Hour, func(context.Context) (string, error) {
		return "", ErrNotConfigured
	})

	waitFor(t, func() bool { return monitor.Get().Traveos.State != StateUnknown })

	// A server that was never given vendor credentials is not a server with a
	// broken vendor. Painting it red teaches everyone to ignore the light.
	if got := monitor.Get().Traveos.State; got != StateNotConfigured {
		t.Errorf("expected %q, got %q", StateNotConfigured, got)
	}
}

func TestMonitorGetIsSafeWhileProbing(t *testing.T) {
	monitor := NewMonitor(func() {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitor.Run(ctx, DependencyBot, time.Microsecond, func(context.Context) (string, error) {
		return "connected", nil
	})

	// Reading while the ticker writes is the ordinary case in production: one
	// goroutine per dependency writing, every HTTP request reading. This test
	// only means something under -race, which is how CI runs it.
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 200 {
				_ = monitor.Get()
			}
		}()
	}
	readers.Wait()
}

func TestMonitorStopsWithItsContext(t *testing.T) {
	monitor := NewMonitor(func() {})

	ctx, cancel := context.WithCancel(context.Background())

	var probes atomic.Int64
	done := make(chan struct{})
	go func() {
		monitor.Run(ctx, DependencyBot, time.Millisecond, func(context.Context) (string, error) {
			probes.Add(1)
			return "", nil
		})
		close(done)
	}()

	waitFor(t, func() bool { return probes.Load() >= 2 })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

func TestTraveosProbeWithoutAClientIsNotConfigured(t *testing.T) {
	// cmd/api explicitly supports booting without vendor credentials, and
	// leaves the client nil when it does.
	if _, err := TraveosProbe(nil)(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestBotProbeWithoutAURLIsNotConfigured(t *testing.T) {
	if _, err := BotProbe("")(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestBotProbeReportsTheSessionStateAsDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Errorf("expected the probe to call /status, got %q", r.URL.Path)
		}
		// The real bot answers the standard envelope, and it carries jid and
		// pair_code alongside the state. The probe must pick out the state
		// and leave the rest behind.
		_, _ = w.Write([]byte(`{"success":true,"message":"status","data":{"state":"connected","jid":"5511999999999@s.whatsapp.net","pair_code":"ABCD-1234"}}`))
	}))
	defer server.Close()

	detail, err := BotProbe(server.URL)(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail != "connected" {
		t.Errorf("expected detail %q, got %q", "connected", detail)
	}
}

func TestBotProbeFailsWhenTheProcessIsGone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	if _, err := BotProbe(server.URL)(context.Background()); err == nil {
		t.Error("expected an error when the bot does not answer 200")
	}
}
