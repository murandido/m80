package bot

import (
	"os"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
)

// TestMain switches the humanizer's sleeps off for this package.
//
// They are the whole point in production and pure wall clock in here: 65 of
// these tests were spending roughly 90% of the package's runtime doing nothing
// but waiting, which was also enough to trip go test's 10 minute default once
// the packages ran under load together.
//
// Nothing is lost by turning them off at this level. The ranges themselves are
// asserted by internal/bot/humanizer's own tests, which are a separate package
// and therefore a separate test binary that this flag cannot reach. The one
// test in here that needs the real timing turns them back on for itself - see
// humanizer_flow_test.go.
//
// It also shortens interruptedJobGrace for the same reason. The tests that reach
// it park a job that never finishes, so they always wait the grace out in full -
// ten seconds each, three times, for a timeout whose expiry is the whole point.
// The production value stays in deletar.go; only this binary sees the short one.
func TestMain(m *testing.M) {
	humanizer.SetDelaysEnabled(false)
	interruptedJobGrace = 50 * time.Millisecond
	os.Exit(m.Run())
}
