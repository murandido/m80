package statement

import (
	"testing"
	"time"
)

// The period comes from the file, so a comparison can never run over a window the statement
// does not describe.
func TestPeriodOfSpansTheMovements(t *testing.T) {
	t.Parallel()

	statement := Statement{Movements: []Movement{
		{OccurredOn: day("2026-08-15")},
		{OccurredOn: day("2026-07-22")},
		{OccurredOn: day("2026-09-07")},
	}}

	from, to, ok := PeriodOf(statement)
	if !ok {
		t.Fatal("PeriodOf reported no period for a statement with movements")
	}
	if !from.Equal(day("2026-07-22")) || !to.Equal(day("2026-09-07")) {
		t.Errorf("period = %s..%s, want 2026-07-22..2026-09-07",
			from.Format(time.DateOnly), to.Format(time.DateOnly))
	}
}

func TestPeriodOfReportsNothingForAnEmptyStatement(t *testing.T) {
	t.Parallel()

	if _, _, ok := PeriodOf(Statement{}); ok {
		t.Error("PeriodOf claimed a period for an empty statement")
	}
}
