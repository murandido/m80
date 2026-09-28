-- Two columns for the move off Traveos, one carrier at a time.
--
-- direct_booking_enabled is the switch. The M-80 is growing an integration with each
-- airline's own endpoint, and the vendor stops being the only way to read an itinerary.
-- Per airline rather than global because the integrations arrive one at a time, and
-- separate from traveos_supported on purpose: that one says whether the vendor may be
-- used at all, this one says whether we try the airline first. An airline can have
-- both, and that is the normal state during the migration - the direct integration
-- answers, and Traveos catches the requests it could not.
--
-- Defaulting to FALSE is what makes deploying a new carrier a non-event: the code ships
-- dark and somebody turns it on from the airline screen, with no release in between and
-- no window where an untested integration is suddenly answering for real bookings.
ALTER TABLE airlines ADD COLUMN direct_booking_enabled BOOLEAN NOT NULL DEFAULT FALSE;

-- booking_source records which provider actually answered - 'traveos', 'azul', and so on.
--
-- It is not bookkeeping for its own sake: it is the only way to find out whether a direct
-- integration is really carrying its airline, or whether it has been quietly falling back
-- to the vendor on every lookup. Turning Traveos off for a carrier is a decision that has
-- to be made on that evidence, and without this column the evidence does not exist.
--
-- Nullable, with no backfill: every row written before this migration came from Traveos,
-- but writing that in would be asserting it about rows nobody checked. NULL reads as
-- "from before this was recorded", which is exactly what it is.
ALTER TABLE issuances ADD COLUMN booking_source VARCHAR(32);
