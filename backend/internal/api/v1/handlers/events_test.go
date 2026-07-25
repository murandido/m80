package handlers

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/events"
)

// These tests drive the stream over a real httptest server rather than a
// ResponseRecorder: the handler writes from its own goroutine while the test
// reads, and ResponseRecorder is not safe for that under -race.

// openStream connects to a test server running Stream and returns a reader
// positioned right after the initial ": connected" frame, so the caller knows
// the client is already registered in the broker.
func openStream(t *testing.T, broker *events.Broker) (*bufio.Reader, context.CancelFunc) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(NewEventHandler(broker).Stream))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to open stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if got, want := resp.Header.Get("Content-Type"), "text/event-stream"; got != want {
		t.Errorf("Content-Type: got %q, want %q", got, want)
	}

	reader := bufio.NewReader(resp.Body)
	// ": connected\n" followed by the blank line that terminates the frame.
	readLine(t, reader)
	readLine(t, reader)

	return reader, cancel
}

// readLine reads one line, failing the test instead of hanging forever if the
// handler never writes.
func readLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)

	go func() {
		line, err := reader.ReadString('\n')
		done <- result{line: line, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("failed to read from stream: %v", got.err)
		}
		return got.line
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a line from the stream")
		return ""
	}
}

func TestStream_WritesBroadcastEventInSSEFormat(t *testing.T) {
	broker := events.NewBroker()
	reader, _ := openStream(t, broker)

	// A deliberately made-up type: the stream is domain-agnostic and must frame
	// whatever any module publishes, not just issuance events.
	broker.Broadcast(events.Event{
		Type:    "resource.changed",
		Payload: map[string]string{"id": "11111111-1111-1111-1111-111111111111"},
	})

	if got, want := readLine(t, reader), "event: resource.changed\n"; got != want {
		t.Errorf("event line: got %q, want %q", got, want)
	}
	if got, want := readLine(t, reader), `data: {"id":"11111111-1111-1111-1111-111111111111"}`+"\n"; got != want {
		t.Errorf("data line: got %q, want %q", got, want)
	}
	if got, want := readLine(t, reader), "\n"; got != want {
		t.Errorf("frame terminator: got %q, want %q", got, want)
	}
}

func TestStream_UnsubscribesWhenClientDisconnects(t *testing.T) {
	broker := events.NewBroker()
	_, cancel := openStream(t, broker)

	if got := broker.ClientCount(); got != 1 {
		t.Fatalf("client count while connected: got %d, want 1", got)
	}

	// Same thing the browser does when the tab is closed: the request context is
	// cancelled and the handler must take itself out of the hub.
	cancel()

	deadline := time.Now().Add(5 * time.Second)
	for broker.ClientCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("client count after disconnect: got %d, want 0", broker.ClientCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
