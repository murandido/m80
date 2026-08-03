// Package health polls the process's external dependencies in the background
// and keeps the last result of each one, so the panel can show whether the
// WhatsApp bot and the Traveos vendor are actually up.
//
// It is deliberately domain-agnostic, in the same spirit as internal/events: it
// knows nothing about Traveos or whatsmeow, and takes each dependency as a
// Probe function. That is what keeps its tests pure - no Postgres, no network -
// and what stops it from becoming a second home for business rules. The
// concrete probes live in probes.go, which is allowed to know about both.
//
// The shape is the one internal/bot/status.go already established for the
// whatsmeow session: a snapshot behind a lock plus a notify callback, so this
// package never learns what HTTP or SSE are.
package health

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrNotConfigured is what a probe returns when the dependency was never set up
// on this server - no vendor credentials, no bot URL. It is reported as its own
// state rather than as a failure: "nobody configured this" and "this is broken"
// call for different reactions, and painting the first one red trains everyone
// to ignore the light.
var ErrNotConfigured = errors.New("health: dependency is not configured")

// State is the coarse condition of one dependency, as shown in the panel.
type State string

const (
	// StateUnknown is the state before the first probe has answered. It only
	// exists for the first few seconds of the process's life.
	StateUnknown       State = "unknown"
	StateUp            State = "up"
	StateDown          State = "down"
	StateNotConfigured State = "not_configured"
)

// Probe checks one dependency. detail is an optional short label the probe may
// attach - the bot uses it to carry the whatsmeow session state - and this
// package never interprets it, it only notices when it changes.
type Probe func(ctx context.Context) (detail string, err error)

// Check is the last known result for one dependency.
type Check struct {
	State State `json:"state"`
	// Detail is probe-defined and may be empty. It is not an error message:
	// see probes.go for why a Traveos failure never carries one.
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// Snapshot is every dependency at once, which is what the status route returns.
type Snapshot struct {
	Bot     Check `json:"bot"`
	Traveos Check `json:"traveos"`
}

// Dependency names the field a probe writes into. They are constants rather
// than free strings so a typo in cmd/api is a compile error instead of a light
// that silently never turns on.
type Dependency string

const (
	DependencyBot     Dependency = "bot"
	DependencyTraveos Dependency = "traveos"
)

// Monitor holds the latest Snapshot and runs one goroutine per dependency to
// keep it fresh. It is safe for concurrent use: the probes write, every HTTP
// request reads.
type Monitor struct {
	mu       sync.RWMutex
	snapshot Snapshot
	notify   func()
}

// NewMonitor returns a monitor with everything unknown. notify is called after
// every observed *change*; pass a no-op if nothing needs to react.
func NewMonitor(notify func()) *Monitor {
	now := time.Now()
	return &Monitor{
		snapshot: Snapshot{
			Bot:     Check{State: StateUnknown, CheckedAt: now},
			Traveos: Check{State: StateUnknown, CheckedAt: now},
		},
		notify: notify,
	}
}

// Get returns the current snapshot. It never blocks on a probe: the HTTP
// handler reads whatever the last tick left behind, so a vendor taking twenty
// seconds to answer is never a request that takes twenty seconds.
func (m *Monitor) Get() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.snapshot
}

// Run probes one dependency immediately and then on every tick, until ctx is
// cancelled. It is meant to be started with `go`.
//
// The immediate first probe is the point: without it the panel would open on
// "unknown" for a full interval after every deploy, which reads exactly like
// the monitor itself being broken.
func (m *Monitor) Run(ctx context.Context, dependency Dependency, interval time.Duration, probe Probe) {
	m.probeOnce(ctx, dependency, probe)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.probeOnce(ctx, dependency, probe)
		}
	}
}

// probeTimeout caps a single check. A probe that hangs must not still be
// hanging when the next tick arrives, and none of these dependencies has any
// business taking ten seconds to say hello.
const probeTimeout = 10 * time.Second

func (m *Monitor) probeOnce(ctx context.Context, dependency Dependency, probe Probe) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	detail, err := probe(probeCtx)

	check := Check{State: StateUp, Detail: detail, CheckedAt: time.Now()}
	switch {
	case errors.Is(err, ErrNotConfigured):
		check.State = StateNotConfigured
	case err != nil:
		check.State = StateDown
		// The error is logged here and nowhere else. It never reaches the
		// panel: for Traveos, every failure answers with the same opaque 500
		// (see internal/traveos/errors.go), so any reason shown on screen
		// would be invented.
		slog.Warn("health probe failed", "dependency", dependency, "err", err)
	}

	m.record(dependency, check)
}

// record stores the check and notifies only when something actually moved.
//
// Notifying on every tick would broadcast an SSE event to every open browser
// hundreds of times a day carrying no news at all. CheckedAt advances on every
// tick by definition, so it is deliberately not part of the comparison - only
// State and Detail are.
func (m *Monitor) record(dependency Dependency, check Check) {
	m.mu.Lock()

	var previous Check
	switch dependency {
	case DependencyBot:
		previous = m.snapshot.Bot
		m.snapshot.Bot = check
	case DependencyTraveos:
		previous = m.snapshot.Traveos
		m.snapshot.Traveos = check
	}

	changed := previous.State != check.State || previous.Detail != check.Detail
	m.mu.Unlock()

	if !changed {
		return
	}

	// Both directions are worth a line: an outage that healed on its own at
	// 3am is only visible in the log afterwards.
	slog.Info("health state changed",
		"dependency", dependency,
		"from", previous.State,
		"to", check.State,
		"detail", check.Detail)

	m.notify()
}
