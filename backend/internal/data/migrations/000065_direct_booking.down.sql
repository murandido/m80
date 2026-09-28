-- Reverses 000065. Both columns are additive and nothing outside the booking provider
-- layer reads them, so dropping them returns every airline to Traveos-only.
--
-- What is lost is which provider answered for each issuance. The itinerary rows
-- themselves are untouched: this column never decided what was written, only who wrote it.
ALTER TABLE issuances DROP COLUMN IF EXISTS booking_source;
ALTER TABLE airlines DROP COLUMN IF EXISTS direct_booking_enabled;
