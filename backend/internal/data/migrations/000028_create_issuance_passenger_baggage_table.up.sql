-- Baggage is not per leg, and it is not one number. The vendor reports it as a
-- list of entitlement classes for a whole direction - AVIANCA sends
-- [{"peso":"10kg","quantidade":1},{"peso":"23kg","quantidade":0}] for a single
-- leg - and issuance_passenger_flights.baggage_qty could only hold the sum,
-- which threw the weights away and merged two different entitlements into one
-- integer. The ticket has to show them side by side, so the storage has to keep
-- them apart.
CREATE TABLE issuance_passenger_baggage (
    id           UUID             NOT NULL DEFAULT gen_random_uuid(),
    passenger_id UUID             NOT NULL,
    -- direction, not flight_id, because that is how the entitlement exists at
    -- the source: one list per direction, covering every leg of it.
    direction    flight_direction NOT NULL,
    -- position is the index in the vendor's own array. It is what decides which
    -- icon the ticket draws - the first entry is the carry-on - and inferring
    -- that from the weight instead would be a guess.
    position     INTEGER          NOT NULL,
    -- weight is NULL when the vendor sends a bare count (GOL's `[1, 0]`), which
    -- is a real shape and not missing data.
    weight       VARCHAR          NULL,
    quantity     INTEGER          NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    FOREIGN KEY (passenger_id) REFERENCES issuance_passengers (id) ON DELETE CASCADE,
    UNIQUE (passenger_id, direction, position)
);

CREATE INDEX idx_issuance_passenger_baggage_passenger ON issuance_passenger_baggage (passenger_id);

ALTER TABLE issuance_passenger_flights DROP COLUMN baggage_qty;
