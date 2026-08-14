-- Two facts about issuances that came from the pre-panel spreadsheet rather than from an
-- operator, and that the Traveos integration has to know about.
--
-- imported: the booking was made outside the system, months before the row existed. Asking
-- Traveos to issue a ticket for one of these would be issuing a real, paid ticket for a
-- flight that already happened - so an imported issuance is only ever *searched* for at the
-- vendor, never emitted (see internal/issuance.findOrEmit).
--
-- traveos_attempted_at: when the automatic lookup last ran on it. The panel's autocomplete
-- has always allowed one attempt per row per page load, held in browser memory, which for a
-- few hundred imported rows means the whole set is looked up again on every refresh. On the
-- row it becomes one attempt per row, full stop.
ALTER TABLE issuances ADD COLUMN imported boolean NOT NULL DEFAULT false;
ALTER TABLE issuances ADD COLUMN traveos_attempted_at timestamptz;
