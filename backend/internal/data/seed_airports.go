package data

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"

	"gorm.io/gorm/clause"
)

//go:embed seeds/airports.json
var airportsSeedJSON []byte

type airportSeedRecord struct {
	IATA    string `json:"iata"`
	Name    string `json:"name"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
}

// SeedAirports populates the airports table from the embedded dataset the
// first time it runs (skips if the table already has rows), so it's safe to
// call on every API startup.
func SeedAirports() error {
	var count int64
	if err := DB.Model(&Airport{}).Count(&count).Error; err != nil {
		return fmt.Errorf("counting airports: %w", err)
	}
	if count > 0 {
		return nil
	}

	var records []airportSeedRecord
	if err := json.Unmarshal(airportsSeedJSON, &records); err != nil {
		return fmt.Errorf("unmarshalling airports seed: %w", err)
	}

	airports := make([]Airport, 0, len(records))
	for _, r := range records {
		airport := Airport{
			IATACode: r.IATA,
			Name:     r.Name,
			Country:  r.Country,
		}
		if r.City != "" {
			city := r.City
			airport.City = &city
		}
		if r.State != "" {
			state := r.State
			airport.State = &state
		}
		airports = append(airports, airport)
	}

	// ON CONFLICT DO NOTHING guards against duplicate IATA codes in the
	// source dataset itself, on top of the count check above.
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "iata_code"}},
		DoNothing: true,
	}).CreateInBatches(&airports, 500)
	if result.Error != nil {
		return fmt.Errorf("seeding airports: %w", result.Error)
	}

	log.Printf("🌱 Seeded %d airports\n", result.RowsAffected)
	return nil
}
