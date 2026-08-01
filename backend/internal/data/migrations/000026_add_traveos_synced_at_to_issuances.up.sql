-- Separates "complete because Traveos confirmed the itinerary" from "complete because
-- someone ticked it on the grid". Both set complete = true, but only the first one has
-- passengers, flights and seats that came from the airline - and that is the difference
-- that decides whether a ticket can be rendered, whether the bot may attach a PDF, and
-- what the refetch button is for.
--
-- Deliberately a timestamp rather than a boolean: the panel needs to show *when* the data
-- was pulled, because an itinerary can move after the fact and the operator's question is
-- always "how stale is this?".
ALTER TABLE issuances ADD COLUMN traveos_synced_at TIMESTAMP WITH TIME ZONE NULL;
