-- issuances.destination_iata_code used to be filled with the arrival of the LAST leg
-- of the itinerary. On a round trip that leg lands back where the passenger started,
-- so the column claimed every round trip was a trip to the passenger's own city.
--
-- The mapper now reads the last leg of the OUTBOUND direction instead. This backfills
-- what was already written, so the fix does not depend on somebody re-pulling every
-- issuance from Traveos one by one.
UPDATE issuances i
SET destination_iata_code = f.destination_iata_code
FROM (
    SELECT DISTINCT ON (issuance_id) issuance_id, destination_iata_code
    FROM issuance_flights
    WHERE direction = 'outbound'
    ORDER BY issuance_id, leg_order DESC
) f
WHERE f.issuance_id = i.id
  AND f.destination_iata_code IS NOT NULL
  AND i.destination_iata_code IS DISTINCT FROM f.destination_iata_code;
