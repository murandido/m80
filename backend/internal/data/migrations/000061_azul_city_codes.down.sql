-- Reverses 000061. Drops the city-to-IATA table. The statement page falls back to showing the
-- raw city name without it; nothing settled derives from these codes.
DROP TABLE IF EXISTS azul_city_codes;
