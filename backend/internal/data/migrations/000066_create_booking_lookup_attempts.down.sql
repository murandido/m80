-- Reverses 000066. The table is diagnostic only: nothing reads it to decide anything, so
-- dropping it loses the record of past lookups and changes no behaviour.
DROP TABLE IF EXISTS booking_lookup_attempts;
