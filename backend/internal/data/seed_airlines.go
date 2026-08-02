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

//go:embed seeds/airlines.json
var airlinesSeedJSON []byte

type airlineSeedRecord struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	TraveosAirlineID int    `json:"traveos_airline_id"`
	// BrandColor is the carrier's own primaryColor, read from Traveos'
	// GET /airlines/active rather than picked by eye. Seeding it means a ticket
	// prints in the airline's colour before that carrier has ever been synced.
	BrandColor             string `json:"brand_color"`
	TraveosSupported       bool   `json:"traveos_supported"`
	RequiresLastName       bool   `json:"requires_last_name"`
	RequiresPurchaseNumber bool   `json:"requires_purchase_number"`
	// Aliases are the names and codes this carrier is already known by in the
	// database - old IATA codes, longer legal names, and the typos that got typed
	// in by hand. Listing them is what lets the seeder recognise an existing row
	// instead of creating a second one beside it, and it keeps every match
	// auditable: a clever normaliser would guess, this file states.
	Aliases []string `json:"aliases"`
}

// SeedAirlines brings the airlines table in line with the embedded catalog.
//
// Unlike SeedAirports it does *not* stop at a non-empty table. Airlines are an
// external catalog we mirror, and the copy in the database was typed by hand: it
// carries codes that no longer match the ticket logos, a misspelt name, a missing
// carrier, and - the reason this exists - no traveos_airline_id at all, which
// makes every sync refuse before it reaches the vendor.
//
// So a row that already exists is corrected rather than skipped - including
// requires_last_name and requires_purchase_number, which decide what the bot
// demands on line 2 of an EMISSAO and what the sync demands before spending a
// real emission. Those are part of what the catalog is authoritative about: which
// carriers need a surname is a fact about the airline, not a per-installation
// preference, and the four carriers already in the database would be reachable no
// other way.
//
// What it will not touch:
//
//   - traveos_supported is only ever promoted to true, never back to false.
//     Turning Traveos off for a carrier is a deliberate choice in the CRUD.
//   - brand_color once it has a value: the vendor is the source (see
//     issuance/traveos_map.go), and it already answered once.
//   - any airline the seed file does not mention.
//
// The row's uuid is preserved in every case, so no issuance loses its airline.
func SeedAirlines(db *gorm.DB) error {
	var records []airlineSeedRecord
	if err := json.Unmarshal(airlinesSeedJSON, &records); err != nil {
		return fmt.Errorf("unmarshalling airlines seed: %w", err)
	}

	// One transaction for the whole catalog. Renaming codes can collide with the
	// unique index (say another row already holds the code this one is moving to),
	// and a half-applied catalog is worse than none: the API refuses to boot and
	// somebody looks at it, instead of the panel coming up with two carriers
	// fighting over one code.
	return db.Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			existing, err := findSeededAirline(tx, record)
			if err != nil {
				return err
			}

			if existing == nil {
				if err := createSeededAirline(tx, record); err != nil {
					return err
				}
				continue
			}

			if err := reconcileSeededAirline(tx, record, existing); err != nil {
				return err
			}
		}
		return nil
	})
}

// findSeededAirline looks for the row this record already has in the database,
// matching its code or name against the record's own code, name and aliases.
func findSeededAirline(tx *gorm.DB, record airlineSeedRecord) (*Airline, error) {
	candidates := append([]string{record.Code, record.Name}, record.Aliases...)

	lowered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate != "" {
			lowered = append(lowered, candidate)
		}
	}

	var airline Airline
	err := tx.Where("LOWER(code) IN ? OR LOWER(name) IN ?", lowered, lowered).First(&airline).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("looking up airline %s: %w", record.Code, err)
	}
	return &airline, nil
}

func createSeededAirline(tx *gorm.DB, record airlineSeedRecord) error {
	traveosID := record.TraveosAirlineID
	brandColor := record.BrandColor
	airline := Airline{
		Code:                   record.Code,
		Name:                   record.Name,
		TraveosAirlineID:       &traveosID,
		BrandColor:             &brandColor,
		TraveosSupported:       record.TraveosSupported,
		RequiresLastName:       record.RequiresLastName,
		RequiresPurchaseNumber: record.RequiresPurchaseNumber,
	}
	if err := tx.Create(&airline).Error; err != nil {
		return fmt.Errorf("creating airline %s: %w", record.Code, err)
	}

	log.Printf("🌱 airline %s created (traveos_airline_id=%d)\n", record.Code, traveosID)
	return nil
}

// reconcileSeededAirline updates only the columns the catalog is authoritative
// about, and logs each one. A seeder that repairs data in silence is impossible
// to audit afterwards - "why is this carrier's code different today" has to have
// an answer in the boot log.
func reconcileSeededAirline(tx *gorm.DB, record airlineSeedRecord, existing *Airline) error {
	changes := map[string]any{}
	var described []string

	if existing.Code != record.Code {
		changes["code"] = record.Code
		described = append(described, fmt.Sprintf("code %s → %s", existing.Code, record.Code))
	}
	if existing.Name != record.Name {
		changes["name"] = record.Name
		described = append(described, fmt.Sprintf("name %q → %q", existing.Name, record.Name))
	}
	if existing.TraveosAirlineID == nil || *existing.TraveosAirlineID != record.TraveosAirlineID {
		changes["traveos_airline_id"] = record.TraveosAirlineID
		described = append(described, fmt.Sprintf("traveos_airline_id → %d", record.TraveosAirlineID))
	}
	// Fill only, never replace: an existing colour came either from a previous run
	// of this seeder or from a real ticket the vendor already answered with, and
	// both beat overwriting it on every boot.
	if existing.BrandColor == nil || strings.TrimSpace(*existing.BrandColor) == "" {
		changes["brand_color"] = record.BrandColor
		described = append(described, "brand_color → "+record.BrandColor)
	}
	// Promotion only. A carrier the catalog knows emits for real should be reachable;
	// one that somebody turned off stays off.
	if record.TraveosSupported && !existing.TraveosSupported {
		changes["traveos_supported"] = true
		described = append(described, "traveos_supported → true")
	}
	// Both directions, unlike traveos_supported: which carriers need a surname or a
	// purchase number is a fact about the airline, and getting it wrong is expensive
	// in both directions - too strict refuses an EMISSAO that was fine, too loose
	// spends a real non-idempotent emission on a request the vendor cannot answer.
	if existing.RequiresLastName != record.RequiresLastName {
		changes["requires_last_name"] = record.RequiresLastName
		described = append(described, fmt.Sprintf("requires_last_name → %t", record.RequiresLastName))
	}
	if existing.RequiresPurchaseNumber != record.RequiresPurchaseNumber {
		changes["requires_purchase_number"] = record.RequiresPurchaseNumber
		described = append(described,
			fmt.Sprintf("requires_purchase_number → %t", record.RequiresPurchaseNumber))
	}

	if len(changes) == 0 {
		return nil
	}

	if err := tx.Model(&Airline{}).Where("id = ?", existing.ID).Updates(changes).Error; err != nil {
		return fmt.Errorf("reconciling airline %s: %w", record.Code, err)
	}

	log.Printf("🌱 airline %s: %s\n", record.Code, strings.Join(described, ", "))
	return nil
}
