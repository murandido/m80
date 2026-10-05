-- Whether this carrier identifies a booking by its origin as well as the locator. Only
-- GOL and Azul do, among the integrations built so far; every other one - and Traveos -
-- finds the booking without it. The panel used to demand the origin of every airline,
-- which held up the autocomplete for carriers that never read it.
--
-- Defaults to false and is set per carrier by the airlines seed, which reconciles the
-- requires_* flags on every boot (see seed_airlines.go).
ALTER TABLE airlines ADD COLUMN requires_origin BOOLEAN NOT NULL DEFAULT FALSE;
