-- One row per booking provider asked during an itinerary sync, whichever answered.
--
-- It exists to decide when each airline's own integration can carry its bookings alone.
-- issuances.booking_source (migration 65) says who answered in the end, but not whether
-- the airline was asked first and failed, nor why - an outage and a "no such booking"
-- mean opposite things (see internal/booking), and only this table tells them apart.
--
-- TEMPORARY. It is scaffolding for the move off Traveos and goes away with it: once every
-- carrier reads from its own endpoint there is no fallback left to measure. Nothing but
-- the sync writes here and nothing but the direct-lookups report reads it, so dropping it
-- is the whole of the removal on the database side.
CREATE TABLE booking_lookup_attempts (
    id           UUID NOT NULL DEFAULT gen_random_uuid(),
    -- One value per call to the sync, shared by every provider it asked. It is what ties
    -- the airline's failure to the Traveos answer that followed it - a "fell back to the
    -- vendor" is a sync with both.
    sync_id      UUID NOT NULL,
    issuance_id  UUID NOT NULL,
    airline_id   UUID NOT NULL,
    -- The provider's own name ('traveos', 'copa', ...), as booking_source records it.
    provider     VARCHAR(32) NOT NULL,
    -- found: it returned the booking. not_found: the application answered that it has no
    -- such booking. unavailable: it could not be reached. refused: the sync turned the
    -- lookup away before or after asking (a missing field, an imported row).
    outcome      VARCHAR(16) NOT NULL,
    -- The error as the log would print it, cut short; NULL when found.
    error        TEXT NULL,
    duration_ms  INTEGER NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT booking_lookup_attempts_pkey PRIMARY KEY (id),
    -- CASCADE both ways: a diagnostic row has no meaning without its issuance or airline,
    -- and it must never be the thing that stops a hard delete of either.
    CONSTRAINT booking_lookup_attempts_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE CASCADE,
    CONSTRAINT booking_lookup_attempts_airline_fk
        FOREIGN KEY (airline_id) REFERENCES airlines(id) ON DELETE CASCADE,
    CONSTRAINT booking_lookup_attempts_outcome
        CHECK (outcome IN ('found', 'not_found', 'unavailable', 'refused'))
);

-- The one read there is: an airline's attempts over the last N days.
CREATE INDEX idx_booking_lookup_attempts_airline_created
    ON booking_lookup_attempts (airline_id, created_at DESC);
