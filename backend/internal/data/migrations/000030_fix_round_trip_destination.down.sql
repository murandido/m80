-- Deliberately empty. The previous value was the arrival of the last leg of the whole
-- itinerary, which is exactly the wrong answer this migration corrected - putting it
-- back would be restoring a defect, not reversing a change. Rolling the schema back
-- past this point leaves the destinations right.
SELECT 1;
