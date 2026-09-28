package handlers

import (
	"net/http"
	"time"

	"gorm.io/gorm"
)

// statementExportPeriod is the day range a statement CSV covers: the date filter the operator has
// on the statement page, sent along with the download. Both bounds are optional and inclusive, as
// Brazilian calendar days - the grain occurred_on is stored in - so no zone conversion happens
// here. With neither, the export is everything stored, as it always was.
type statementExportPeriod struct {
	from, to string
}

// parseStatementExportPeriod reads date_from/date_to (YYYY-MM-DD). An unparseable value is
// treated as absent rather than as an error, the same leniency parseDateRange gives the listings.
func parseStatementExportPeriod(r *http.Request) statementExportPeriod {
	day := func(value string) string {
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return ""
		}
		return value
	}
	q := r.URL.Query()
	return statementExportPeriod{from: day(q.Get("date_from")), to: day(q.Get("date_to"))}
}

// scope narrows a statement-lines query to the period.
func (p statementExportPeriod) scope(db *gorm.DB) *gorm.DB {
	if p.from != "" {
		db = db.Where("occurred_on >= ?", p.from)
	}
	if p.to != "" {
		db = db.Where("occurred_on <= ?", p.to)
	}
	return db
}

// filenameSuffix names the period in the download ("-2026-09-01_2026-09-26"), so two exports of
// one account do not overwrite each other in the operator's downloads. An open side reads as
// "inicio" or "hoje".
func (p statementExportPeriod) filenameSuffix() string {
	if p.from == "" && p.to == "" {
		return ""
	}
	from, to := p.from, p.to
	if from == "" {
		from = "inicio"
	}
	if to == "" {
		to = "hoje"
	}
	return "-" + from + "_" + to
}
