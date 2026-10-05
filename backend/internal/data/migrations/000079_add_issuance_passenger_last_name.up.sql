-- The passenger's surname as the operator types it for the airline's lookup, kept apart
-- from main_passenger_name. That column carries the surname until the first sync and the
-- full name after it, and a surname typed for the lookup used to be overwritten there -
-- the next refetch then asked the airline with the full name instead.
--
-- Nullable and unused by everything written before it: a null falls back to
-- main_passenger_name wherever a surname is needed (data.Issuance.BookingLastName).
ALTER TABLE issuances ADD COLUMN passenger_last_name VARCHAR(255);
