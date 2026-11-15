package testdb

import "testing"

func TestRequireTestDatabaseAcceptsATestName(t *testing.T) {
	t.Setenv("DB_NAME", "m80_test")
	RequireTestDatabase(t)
}

// The refusing branches call Skip and Fatalf, which end the calling test. They run inside a
// subtest whose outcome is then read back, so the parent can assert on it.
func TestRequireTestDatabaseRefusesOtherNames(t *testing.T) {
	tests := []struct {
		name       string
		dbName     string
		wantFailed bool
		wantSkip   bool
	}{
		{name: "dev database", dbName: "m80", wantFailed: true},
		{name: "test in the middle", dbName: "m80_test_copy", wantFailed: true},
		{name: "unset", dbName: "", wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DB_NAME", tt.dbName)
			probe := &recordingTB{TB: t}
			func() {
				defer func() { _ = recover() }()
				RequireTestDatabase(probe)
			}()
			if probe.failed != tt.wantFailed || probe.skipped != tt.wantSkip {
				t.Errorf("DB_NAME=%q: failed=%v skipped=%v, want failed=%v skipped=%v",
					tt.dbName, probe.failed, probe.skipped, tt.wantFailed, tt.wantSkip)
			}
		})
	}
}

// recordingTB records Skip and Fatalf instead of ending the test, and panics to stop
// RequireTestDatabase the way runtime.Goexit would.
type recordingTB struct {
	testing.TB
	failed  bool
	skipped bool
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Skip(...any) {
	r.skipped = true
	panic("skip")
}

func (r *recordingTB) Fatalf(string, ...any) {
	r.failed = true
	panic("fatal")
}
