ALTER TABLE issuance_passenger_flights ADD COLUMN baggage_qty INTEGER NULL DEFAULT 0;

-- Going back loses the weights - there is nowhere to put them - but it does not
-- have to lose the count. Every leg of a direction carried the same total under
-- the old shape, so the sum of that direction's entitlements is exactly what the
-- column held before this migration ran.
UPDATE issuance_passenger_flights ipf
SET baggage_qty = COALESCE((
    SELECT SUM(b.quantity)
    FROM issuance_passenger_baggage b
    JOIN issuance_flights f ON f.id = ipf.flight_id
    WHERE b.passenger_id = ipf.passenger_id
      AND b.direction = f.direction
), 0);

DROP TABLE issuance_passenger_baggage;
