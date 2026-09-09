package statement

import (
	"encoding/json"
	"time"
)

// batchLineDateLayout is the date as the column stores it: a plain day, which is all the
// comparison matches on.
const batchLineDateLayout = "2006-01-02"

// BatchLine is one statement credit that went into a batch.
//
// Recorded on the batch itself (point_batches.statement_lines) because a batch that summed
// four credits no longer resembles any of them: date and amount alone cannot find it
// again, and the same file reconciled a second time would offer to record all four once
// more. Only the two fields the comparison needs are kept - nothing about the flight, the
// passenger or the airline's wording.
type BatchLine struct {
	OccurredOn time.Time
	Points     int
}

// The stored shape, short because a batch of twenty accruals carries twenty of these.
type batchLineJSON struct {
	Day    string `json:"d"`
	Points int    `json:"p"`
}

func (l BatchLine) MarshalJSON() ([]byte, error) {
	return json.Marshal(batchLineJSON{Day: l.OccurredOn.Format(batchLineDateLayout), Points: l.Points})
}

func (l *BatchLine) UnmarshalJSON(raw []byte) error {
	var stored batchLineJSON
	if err := json.Unmarshal(raw, &stored); err != nil {
		return err
	}
	day, err := time.Parse(batchLineDateLayout, stored.Day)
	if err != nil {
		return err
	}
	l.OccurredOn, l.Points = day, stored.Points
	return nil
}

// EncodeBatchLines is what the commit writes onto a batch. No lines means no column value:
// a batch entered by hand says so by having none, and the comparison then falls back to
// matching it by date and amount.
func EncodeBatchLines(lines []BatchLine) ([]byte, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	return json.Marshal(lines)
}

// DecodeBatchLines reads the column back.
//
// A value that does not parse is read as no provenance at all rather than as an error: the
// batch then compares the way a hand-entered one does, which is a worse answer than the
// stored one but a much better answer than refusing to reconcile the account.
func DecodeBatchLines(raw []byte) []BatchLine {
	if len(raw) == 0 {
		return nil
	}
	var lines []BatchLine
	if err := json.Unmarshal(raw, &lines); err != nil {
		return nil
	}
	return lines
}
