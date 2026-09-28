-- The city-to-IATA table the Azul statement page reads to show a redemption's route as codes. It
-- was a hardcoded map in Go; moving it here lets the operator assign a code when a city the map
-- does not know shows up, instead of it going out with the next release. Seeded on boot from the
-- Go default (SeedAzulCityCodes); this migration only creates the table. Additive, so it changes
-- no existing number.
CREATE TABLE azul_city_codes (
    -- The normalised lookup key (lower-case, accents stripped), how the statement's place name is
    -- matched. One row per city.
    city_key TEXT PRIMARY KEY,
    -- The name as it was seen/typed, for showing in a management list; the key is what matches.
    city_name TEXT NOT NULL,
    iata_code TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
