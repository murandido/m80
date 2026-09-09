package data

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"

	"gorm.io/gorm/clause"
)

//go:embed seeds/point_batch_origins.json
var pointBatchOriginsSeedJSON []byte

type pointBatchOriginSeedRecord struct {
	Name string `json:"name"`
}

// SeedPointBatchOrigins populates the origins catalogue the first time it runs.
//
// It is the "only when empty" kind of seed, not a reconciler: this is starting volume the
// operator owns from then on. A partner they renamed, retired or added by hand is their
// decision about their own business, and a seeder that corrected it on every boot would be
// undoing that work in silence. Which is the opposite of seed_airlines.go, where the
// catalogue mirrors something external and the database is the one that can be wrong.
func SeedPointBatchOrigins() error {
	var count int64
	if err := DB.Model(&PointBatchOrigin{}).Count(&count).Error; err != nil {
		return fmt.Errorf("counting point batch origins: %w", err)
	}
	if count > 0 {
		return nil
	}

	var records []pointBatchOriginSeedRecord
	if err := json.Unmarshal(pointBatchOriginsSeedJSON, &records); err != nil {
		return fmt.Errorf("unmarshalling point batch origins seed: %w", err)
	}

	origins := make([]PointBatchOrigin, 0, len(records))
	for _, r := range records {
		origins = append(origins, PointBatchOrigin{Name: r.Name, Active: true})
	}

	// Second layer over the count check, the same one SeedAirports uses: a duplicate name
	// in the source file should be skipped, not fail the boot.
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true,
	}).Create(&origins)
	if result.Error != nil {
		return fmt.Errorf("seeding point batch origins: %w", result.Error)
	}

	log.Printf("🌱 Seeded %d point batch origins\n", result.RowsAffected)
	return nil
}
