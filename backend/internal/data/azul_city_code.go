package data

import (
	"time"

	"github.com/murandido/m-80-backend/internal/azul"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AzulCityCode maps an Azul statement place name to its airport IATA code. Keyed by the normalised
// city name (azul.NormalizeCityKey) so the lookup is accent- and case-insensitive. Seeded from the
// Go default and extended by the operator when a new city appears. See migration 61.
type AzulCityCode struct {
	CityKey   string    `gorm:"column:city_key;primaryKey" json:"city_key"`
	CityName  string    `gorm:"column:city_name;not null" json:"city_name"`
	IATACode  string    `gorm:"column:iata_code;not null" json:"iata_code"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (AzulCityCode) TableName() string {
	return "azul_city_codes"
}

// SeedAzulCityCodes fills the table from the Go default the map used to live in, inserting only
// the rows that are not there yet - so it is safe to run every boot and never overwrites a code
// the operator changed by hand.
func SeedAzulCityCodes(db *gorm.DB) error {
	seed := azul.DefaultCityCodes()
	rows := make([]AzulCityCode, 0, len(seed))
	for _, c := range seed {
		rows = append(rows, AzulCityCode{CityKey: c.Key, CityName: c.Key, IATACode: c.IATA})
	}
	if len(rows) == 0 {
		return nil
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// LoadAzulCityCodes reads the whole table into the lookup ResolveCity takes (normalised key ->
// IATA). Small and read once per statement request.
func LoadAzulCityCodes(db *gorm.DB) (map[string]string, error) {
	var rows []AzulCityCode
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	codes := make(map[string]string, len(rows))
	for _, row := range rows {
		codes[row.CityKey] = row.IATACode
	}
	return codes, nil
}

// UpsertAzulCityCode assigns (or reassigns) a city's IATA code. The key is normalised so the same
// city typed with different accents/case lands on one row; cityName keeps what was typed.
func UpsertAzulCityCode(db *gorm.DB, cityName, iata string) error {
	row := AzulCityCode{
		CityKey:  azul.NormalizeCityKey(cityName),
		CityName: cityName,
		IATACode: iata,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "city_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"city_name":  cityName,
			"iata_code":  iata,
			"updated_at": gorm.Expr("now()"),
		}),
	}).Create(&row).Error
}
