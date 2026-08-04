package handlers

import (
	"testing"
	"time"
)

func TestFreshTraveosSyncing_HidesAMarkerLeftBehindByADeadProcess(t *testing.T) {
	// The marker is cleared by a defer, so the only way one outlives its sync is a
	// process that died mid-pull. Past the sync's own timeout it cannot still be
	// running, and reporting it would lock the row on every screen with no way out.
	recent := time.Now().Add(-10 * time.Second)
	stale := time.Now().Add(-traveosSyncTimeout - time.Minute)

	if got := freshTraveosSyncing(nil); got != nil {
		t.Errorf("freshTraveosSyncing(nil) = %v, want nil", got)
	}
	if got := freshTraveosSyncing(&recent); got == nil {
		t.Error("a pull started 10s ago was reported as not running")
	}
	if got := freshTraveosSyncing(&stale); got != nil {
		t.Errorf("a marker older than the timeout was reported as running: %v", got)
	}
}
