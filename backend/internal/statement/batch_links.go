package statement

import (
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BatchLineRef is one point-entry statement line the resolver is asked about: its link key and
// the day/points that identify which batch credit it is.
type BatchLineRef struct {
	Key        string
	OccurredOn time.Time
	Points     int
}

// ResolvedBatch is the real point batch (lote) a statement entry maps to, and whether that
// mapping was pinned by hand rather than matched automatically.
type ResolvedBatch struct {
	Batch  data.PointBatch
	Manual bool
}

// ResolveBatchLinks maps each of the given point-entry lines to the batch it belongs to. For each
// line the resolution is, in order:
//
//  1. a saved manual pin (AzulBatchLink), if its batch still exists;
//  2. otherwise the batch whose recorded credits include a line of the same day and points - a
//     batch keeps the (day, points) of every credit it was summed from, so a purchase or accrual
//     lands on the batch it was recorded into. Most recent acquisition wins a tie.
//  3. otherwise the line is absent from the returned map - not linked (and, in edit mode, the
//     operator can pin a batch).
//
// The batch is loaded with its Origin so the page can name it. Matching is by day and points, not
// by locator: a batch has no PNR, and the point entries it costs mostly carry none either.
func ResolveBatchLinks(db *gorm.DB, accountID string, lines []BatchLineRef) (map[string]ResolvedBatch, error) {
	if len(lines) == 0 {
		return map[string]ResolvedBatch{}, nil
	}

	keys := make([]string, 0, len(lines))
	seenKey := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if line.Key == "" {
			continue
		}
		if _, ok := seenKey[line.Key]; ok {
			continue
		}
		seenKey[line.Key] = struct{}{}
		keys = append(keys, line.Key)
	}

	// The manual pins for these keys, and the batches they point at.
	var pins []data.AzulBatchLink
	if err := db.Where("account_id = ? AND line_key IN ?", accountID, keys).
		Find(&pins).Error; err != nil {
		return nil, fmt.Errorf("statement.ResolveBatchLinks: loading manual links: %w", err)
	}
	pinnedByKey := make(map[string]string, len(pins))
	pinnedIDs := make([]string, 0, len(pins))
	for _, pin := range pins {
		pinnedByKey[pin.LineKey] = pin.BatchID
		pinnedIDs = append(pinnedIDs, pin.BatchID)
	}
	pinnedBatches := make(map[string]data.PointBatch)
	if len(pinnedIDs) > 0 {
		var rows []data.PointBatch
		if err := db.Preload("Origin").Where("id IN ?", pinnedIDs).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("statement.ResolveBatchLinks: loading pinned batches: %w", err)
		}
		for _, row := range rows {
			pinnedBatches[row.ID] = row
		}
	}

	// The automatic match: index every batch's recorded credits by (day, points), most recent
	// acquisition winning a tie, so a line finds the batch it was summed into.
	var batches []data.PointBatch
	if err := db.Preload("Origin").
		Where("account_id = ? AND statement_lines IS NOT NULL", accountID).
		Order("acquired_on DESC").
		Find(&batches).Error; err != nil {
		return nil, fmt.Errorf("statement.ResolveBatchLinks: loading batches: %w", err)
	}
	autoByCredit := make(map[string]data.PointBatch)
	for _, batch := range batches {
		for _, credit := range DecodeBatchLines(batch.StatementLines) {
			creditKey := creditKey(credit.OccurredOn, credit.Points)
			if _, seen := autoByCredit[creditKey]; !seen {
				autoByCredit[creditKey] = batch
			}
		}
	}

	resolved := make(map[string]ResolvedBatch, len(keys))
	for _, line := range lines {
		if line.Key == "" {
			continue
		}
		if _, done := resolved[line.Key]; done {
			continue
		}
		if batchID, pinned := pinnedByKey[line.Key]; pinned {
			if batch, ok := pinnedBatches[batchID]; ok {
				resolved[line.Key] = ResolvedBatch{Batch: batch, Manual: true}
				continue
			}
			// The pinned batch is gone; fall through to the auto match.
		}
		if batch, ok := autoByCredit[creditKey(line.OccurredOn, line.Points)]; ok {
			resolved[line.Key] = ResolvedBatch{Batch: batch, Manual: false}
		}
	}
	return resolved, nil
}

// creditKey identifies one credit within a batch's provenance, and one point-entry line, by the
// two things they share: the day and the signed points.
func creditKey(day time.Time, points int) string {
	return fmt.Sprintf("%s|%d", day.Format(batchLineDateLayout), points)
}

// UpsertBatchLink pins a line on an account to a batch, replacing any existing pin for that
// (account, line key). createdBy records who made it, and may be nil.
func UpsertBatchLink(db *gorm.DB, accountID, lineKey, batchID string, createdBy *string) error {
	link := data.AzulBatchLink{
		AccountID:   accountID,
		LineKey:     lineKey,
		BatchID:     batchID,
		CreatedByID: createdBy,
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}, {Name: "line_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"batch_id":   batchID,
			"created_by": createdBy,
			"updated_at": gorm.Expr("now()"),
		}),
	}).Create(&link).Error
	if err != nil {
		return fmt.Errorf("statement.UpsertBatchLink: %w", err)
	}
	return nil
}

// DeleteBatchLink removes a manual pin, handing the line back to the automatic match. It is a
// no-op when there is no pin.
func DeleteBatchLink(db *gorm.DB, accountID, lineKey string) error {
	err := db.Where("account_id = ? AND line_key = ?", accountID, lineKey).
		Delete(&data.AzulBatchLink{}).Error
	if err != nil {
		return fmt.Errorf("statement.DeleteBatchLink: %w", err)
	}
	return nil
}
