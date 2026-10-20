package data

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

//go:embed seeds/loyalty_programs.json
var loyaltyProgramsSeedJSON []byte

type loyaltyProgramSeedRecord struct {
	Code string `json:"code"`
	Name string `json:"name"`
	// DefaultAirlineCode, not an id: uuids are generated per database, so the seed
	// file refers to the carrier the same way an operator does and the code is
	// resolved at seed time.
	DefaultAirlineCode string `json:"default_airline_code"`
	Color              string `json:"color"`
	// The program's beneficiary cap, for the environments where the seeder creates the
	// program from scratch. Migration 37 sets the same values on databases that already
	// had the programs - the seeder runs after the migrations, so it cannot be the only
	// source. Nil for a program whose rule nobody has researched.
	CPFLimit       *int     `json:"cpf_limit"`
	CPFLimitWindow *string  `json:"cpf_limit_window"`
	CPFLimitMode   *string  `json:"cpf_limit_mode"`
	Aliases        []string `json:"aliases"`
}

// SeedLoyaltyPrograms creates the loyalty programs that are missing, and stops
// there.
//
// It is deliberately more timid than SeedAirlines. Nothing about an existing
// program is known to be wrong the way the airline codes were, and accounts point
// at programs: a duplicate would scatter one program's accounts across two rows,
// which is a mess to unpick by hand. So a program that matches by code, name or
// alias is left exactly as it is - its colour and its default airline are
// whoever-registered-it's call.
//
// Must run after SeedAirlines: default_airline_id is NOT NULL and is resolved
// from the airline code here.
func SeedLoyaltyPrograms(db *gorm.DB) error {
	var records []loyaltyProgramSeedRecord
	if err := json.Unmarshal(loyaltyProgramsSeedJSON, &records); err != nil {
		return fmt.Errorf("unmarshalling loyalty programs seed: %w", err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			found, err := seededLoyaltyProgramExists(tx, record)
			if err != nil {
				return err
			}
			if found {
				continue
			}

			var airline Airline
			if err := tx.Where("LOWER(code) = LOWER(?)", record.DefaultAirlineCode).
				First(&airline).Error; err != nil {
				// Only reachable if the airline seed did not run first, or somebody
				// edited one seed file without the other. Naming both sides beats a
				// foreign key violation from three layers down.
				return fmt.Errorf(
					"seeding loyalty program %s: its default airline %s is not in the database: %w",
					record.Code, record.DefaultAirlineCode, err,
				)
			}

			program := LoyaltyProgram{
				Code:             record.Code,
				Name:             record.Name,
				Color:            record.Color,
				DefaultAirlineID: airline.ID,
				CPFLimit:         record.CPFLimit,
				CPFLimitWindow:   record.CPFLimitWindow,
				CPFLimitMode:     record.CPFLimitMode,
			}
			if err := tx.Create(&program).Error; err != nil {
				return fmt.Errorf("creating loyalty program %s: %w", record.Code, err)
			}

			log.Printf("🌱 loyalty program %s created (default airline %s)\n", record.Code, airline.Code)
		}
		return nil
	})
}

func seededLoyaltyProgramExists(tx *gorm.DB, record loyaltyProgramSeedRecord) (bool, error) {
	candidates := append([]string{record.Code, record.Name}, record.Aliases...)

	lowered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate != "" {
			lowered = append(lowered, candidate)
		}
	}

	// Unscoped: an archived program still counts as existing. Programs are
	// soft-deleted (migration 29) and the seeder runs on every boot, so without this
	// the next `make dev` would re-create every program the operator just archived -
	// with a fresh uuid, leaving the old accounts pointing at the archived row.
	var program LoyaltyProgram
	err := tx.Unscoped().
		Where("LOWER(code) IN ? OR LOWER(name) IN ?", lowered, lowered).
		First(&program).Error
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("looking up loyalty program %s: %w", record.Code, err)
}
