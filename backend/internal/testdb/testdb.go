// Package testdb guards the test suites that write to Postgres.
//
// Every database test begins a transaction it rolls back, but several also commit fixtures
// or empty whole tables (the seed tests run on an emptied catalog). Pointed at the dev
// database by a stray DB_NAME, a plain `go test ./...` would wipe it. The rule is a naming
// one - the database a test may touch ends in "_test" - because a name is the only thing
// every environment (CI, the compose test profile, a disposable container) already sets.
package testdb

import (
	"os"
	"strings"
	"testing"
)

// RequireTestDatabase stops a test before it connects unless DB_NAME names a test database.
//
// An unset DB_NAME skips, the same as an unreachable database always has: there is no
// database configured, and libpq would otherwise fall back to a database named after the
// user. A name that is set but does not end in "_test" fails instead of skipping, loudly,
// because a skip there is exactly the silent pass that hides a misconfigured run.
func RequireTestDatabase(t testing.TB) {
	t.Helper()

	name := os.Getenv("DB_NAME")
	if name == "" {
		t.Skip("skipping: DB_NAME is not set, so there is no test database to use")
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatalf("refusing to run against database %q: database tests only run on a database whose name ends in _test (set DB_NAME, e.g. m80_test)", name)
	}
}
